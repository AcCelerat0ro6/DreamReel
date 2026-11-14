"""Run inside the extracted pack.py archive on an Ubuntu lab host."""
import argparse
import base64
import concurrent.futures
import contextlib
import datetime
import functools
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import platform
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urlencode

ROOT = Path(__file__).resolve().parent
VIEWER = 20000


def command(*args, **kw):
    return subprocess.run(args, text=True, check=True, **kw)


def until(test, timeout=60):
    end = time.monotonic() + timeout
    while not test():
        if time.monotonic() >= end:
            raise TimeoutError(str(test))
        time.sleep(.02)


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=20)
        self.path = str(path)

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


@functools.cache
def token(uid):
    def encode(data):
        return base64.urlsafe_b64encode(data).rstrip(b"=")
    payload = {"uid": uid, "role": "user", "token_type": "access", "jti": str(uid),
               "iat": int(time.time()), "exp": int(time.time()) + 7200}
    parts = [encode(json.dumps(v).encode()) for v in ({"alg": "HS256", "typ": "JWT"}, payload)]
    parts.append(encode(hmac.digest(os.environ["LAB_JWT"].encode(), b".".join(parts), "sha256")))
    return "Bearer " + b".".join(parts).decode()


class Lab:
    def __init__(self, work, out, stack):
        self.work, self.out, self.stack = work, out, stack
        self.local = threading.local()

    def start(self, name, *args, ready, env=None, timeout=60):
        log = self.stack.enter_context((self.work / (name + ".log")).open("w+"))
        proc = subprocess.Popen(args, stdout=log, stderr=subprocess.STDOUT, env=env, start_new_session=True)

        def stop():
            if proc.poll() is None:
                os.killpg(proc.pid, signal.SIGTERM)
                try:
                    proc.wait(15)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.wait()
        self.stack.callback(stop)

        def check():
            if proc.poll() is not None:
                log.seek(0)
                raise RuntimeError(name + " exited: " + log.read()[-3500:])
            return ready()
        try:
            until(check,timeout)
        except TimeoutError:
            log.seek(0)
            raise RuntimeError(name + " startup timeout: " + log.read()[-2500:])

    def sql(self, query, db=True):
        args = ["mysql", "--no-defaults", "-uroot", "-S", str(self.work / "mysql.sock"), "-BN"]
        if db:
            args.append("bench")
        return command(*args, input=query, capture_output=True).stdout.strip()

    def redis(self, *args, db=0, data=None):
        return command("redis-cli", "-s", str(self.work / "redis.sock"), "-n", str(db),
                       *args, input=data, capture_output=True).stdout.strip()

    def services(self):
        w = self.work
        (w / "lab-owner").write_text("dreamreel-perf-v1")
        data = w / "db"
        command("mysqld", "--no-defaults", "--initialize-insecure", "--datadir=" + str(data),
                "--log-error=" + str(w / "mysql-init.log"))
        self.start("mysqld", "mysqld", "--no-defaults", "--datadir=" + str(data), "--skip-networking",
                   "--mysqlx=OFF", "--pid-file=" + str(w / "mysql.pid"), "--socket=" + str(w / "mysql.sock"),
                   "--secure-file-priv=NULL", "--innodb-buffer-pool-size=256M",
                   ready=lambda: subprocess.run(["mysqladmin", "--no-defaults", "-uroot", "-S", str(w / "mysql.sock"),
                                                 "ping"], capture_output=True).returncode == 0)
        self.sql("CREATE DATABASE bench CHARACTER SET utf8mb4", False)
        self.start("redis", "redis-server", "--port", "0", "--unixsocket", str(w / "redis.sock"),
                   "--dir", str(w), "--save", "", "--appendonly", "no",
                   ready=lambda: (w / "redis.sock").exists())
        # Dedicated virtual host on the existing local lab broker; no business queues are bound.
        import urllib.request
        from urllib.parse import quote, unquote, urlsplit, urlunsplit
        import yaml
        config = yaml.safe_load(Path(os.environ["LAB_CONFIG"]).read_text())
        url = urlsplit(config["rabbitmq"]["url"])
        if url.hostname not in ("localhost", "127.0.0.1", "::1"):
            raise ValueError("expected the lab broker on loopback")
        user, password = unquote(url.username), unquote(url.password)
        auth = base64.b64encode(f"{user}:{password}".encode()).decode()
        def admin(method, path, payload=None):
            body = None if payload is None else json.dumps(payload).encode()
            req = urllib.request.Request("http://127.0.0.1:15672/api/"+path, method=method, data=body,
                    headers={"Authorization":"Basic "+auth,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=10) as response:
                return response.read()
        name = "dreamreel_lab_" + secrets.token_hex(12)
        endpoint = "vhosts/" + name
        if any(v["name"] == name for v in json.loads(admin("GET","vhosts"))):
            raise ValueError("test vhost already exists")
        admin("PUT",endpoint,{})
        self.stack.callback(admin,"DELETE",endpoint)
        admin("PUT","permissions/"+name+"/"+quote(user,safe=""),{"configure":".*","write":".*","read":".*"})
        os.environ["LAB_AMQP"] = urlunsplit((url.scheme,url.netloc,"/"+name,url.query,""))

    def server(self, mode):
        binary = "before" if mode == "before" else "after"
        self.start(mode, str(self.work / (binary + "-api")), mode, ready=lambda: (self.work / (mode + ".sock")).exists())

    def seed(self):
        # n=1..20000 generated in one SQL session. Ten videos share every publication second.
        self.sql("""
            SET SESSION cte_max_recursion_depth=20001;
            CREATE TABLE seq (n INT PRIMARY KEY);
            INSERT INTO seq WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM s WHERE n<20000) SELECT n FROM s;
            SET @epoch=UTC_TIMESTAMP();
            INSERT INTO account(id,account,password,nickname,status,role,created_at,updated_at)
              SELECT n,CONCAT('bench',n),'fixture',CONCAT('author',n),1,'user',@epoch,@epoch FROM seq;
            INSERT INTO video(id,author_id,title,description,media_url,cover_url,status,published_at,model_name,created_at,updated_at)
              SELECT n,1+MOD(n-1,64),CONCAT('work',n),'fixture','/lab.mp4','/lab.jpg',2,
              TIMESTAMPADD(SECOND,FLOOR((n-1)/10)-1000,@epoch),CONCAT('model',MOD(n,16)),@epoch,@epoch FROM seq WHERE n<=10000;
            INSERT INTO video_stat(video_id,like_count,comment_count,favorite_count,created_at,updated_at)
              SELECT id,MOD(id*23,1031),MOD(id,11),MOD(id,7),@epoch,@epoch FROM video;
            INSERT INTO user_follow(user_id,target_user_id,status,created_at,updated_at)
              SELECT n,1,1,@epoch,@epoch FROM seq WHERE n>10000;
            INSERT INTO user_follow(user_id,target_user_id,status,created_at,updated_at)
              SELECT 20000,n,1,@epoch,@epoch FROM seq WHERE n BETWEEN 2 AND 16;
            INSERT INTO user_relation_stat(user_id,follower_count,following_count,created_at,updated_at)
              SELECT n,IF(n=1,10000,1),0,@epoch,@epoch FROM seq WHERE n<=16;
            INSERT INTO interaction_action(user_id,video_id,action_type,status,created_at,updated_at)
              SELECT 20000,n,'LIKE',1,@epoch,@epoch FROM seq WHERE MOD(n,3)=0 AND n<=10000;
            INSERT INTO video_view_events(user_id,video_id,scene,event_type,watch_ms,completed,created_at)
              SELECT 20000,n,'recommend','complete',5000,1,@epoch FROM seq WHERE n<=200;
            CREATE TABLE vectors(topic INT PRIMARY KEY, vec JSON);
        """)
        # 64 deterministic nonzero unit vectors. Dense JSON with a mixture of signed coordinates.
        import math
        import random
        rng = random.Random(20261113)
        values = []
        for topic in range(64):
            vec = [rng.uniform(-1, 1) for _ in range(128)]
            norm = math.sqrt(sum(x*x for x in vec))
            values.append(f"({topic},'{json.dumps([round(x/norm,7) for x in vec])}')")
        self.sql("INSERT INTO vectors VALUES " + ",".join(values))
        self.sql("""
            INSERT INTO video_embedding(video_id,model,dimension,embedding_json,text_hash,created_at,updated_at)
              SELECT v.id,'hash-ngram-v1',128,vec,'lab',UTC_TIMESTAMP(),UTC_TIMESTAMP()
              FROM video v JOIN vectors t ON t.topic=MOD(v.id,64);
            ANALYZE TABLE video,video_stat,video_embedding,user_follow,interaction_action;
        """)
        # FDW/Redis indices normally filled by publication workers. Use the same score/member format.
        rows = self.sql("SELECT id,author_id,UNIX_TIMESTAMP(published_at) DIV 1 FROM video WHERE author_id<=16 ORDER BY id")
        commands = []
        for row in rows.splitlines():
            vid, author, ts = map(int, row.split())
            key = "feed:following:author:v1:1" if author == 1 else "feed:following:inbox:v1:20000"
            iso = datetime.datetime.fromtimestamp(ts, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            commands.append(f"ZADD {key} {ts*1000000+vid} {vid}:{author}:{iso}")
        commands.append("ZREMRANGEBYRANK feed:following:inbox:v1:20000 0 -1001")
        response = self.redis(data="\n".join(commands)+"\n")
        assert len(response.splitlines()) == len(commands) and all(x.isdigit() for x in response.splitlines())

    def request(self, mode, method, path, uid=VIEWER):
        if not hasattr(self.local, "connections"):
            self.local.connections = {}
        if mode not in self.local.connections:
            self.local.connections[mode] = UnixHTTP(self.work / (mode + ".sock"))
        conn = self.local.connections[mode]
        conn.request(method, path, headers={"Authorization": token(uid)})
        res = conn.getresponse()
        body = res.read()
        if res.status != 200:
            raise ValueError((mode,path,res.status,body[:180]))
        return json.loads(body)

    def page(self, mode, scene, cursor="", require_next=True):
        res = self.request(mode,"GET","/api/feed-items?"+urlencode(dict(scene=scene, limit=10, cursor=cursor)))
        ids = [r["video_id"] for r in res["items"]]
        assert res["scene"] == scene and len(ids)==len(set(ids))==10, (mode,scene,ids)
        if require_next:
            assert res["has_more"] and res["next_cursor"], (mode,scene,"no next page")
        return res

    def measure(self, fn, count, workers):
        def timed(i):
            start = time.perf_counter_ns()
            fn(i)
            return (time.perf_counter_ns()-start)/1e6
        with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
            start = time.perf_counter()
            samples = list(pool.map(timed, range(count)))
            wall = time.perf_counter()-start
        return {"sample_ms":samples, "wall_s":wall, "clients":workers}

    def http(self):
        with (self.out / "http.jsonl").open("x") as out:
            def save(case, variant, rd, sample):
                out.write(json.dumps(dict(case=case,variant=variant,round=rd,**sample))+"\n")
                out.flush()
            for scene in ("timeline","following","recommend_first","recommend_page","recommend_scroll","favorite","like"):
                print("Running", scene, flush=True)
                for rd in range(1,5):
                    for v in ("AB" if rd%2 else "BA"):
                        if scene in ("favorite","like"):
                            mode = "after" if v=="A" else "async"
                            vid = (3000 if scene=="favorite" else 4000) + rd*2 + int(v=="B")
                            n, workers = 192, 12
                            column = scene+"_count"
                            base = int(self.sql(f"SELECT {column} FROM video_stat WHERE video_id={vid}"))
                            def act(i):
                                res = self.request(mode,"PUT",f"/api/videos/{vid}/{scene}",10001+i)
                                assert res["active"] and res["video_id"]==vid and res["action_type"]==scene.upper()
                            for i in range(n): token(10001+i)
                            # Separate target for untimed connection/route warm-up.
                            prior=self.request(mode,"GET","/__lab/consumed")
                            self.request(mode,"PUT",f"/api/videos/{vid+100}/{scene}",10001)
                            if mode=="async":
                                until(lambda:self.request(mode,"GET","/__lab/consumed")==prior+1)
                            sample = self.measure(act,n,workers)
                            front_end = time.perf_counter()
                            def persisted():
                                vals=self.sql(f"""SELECT {column},(SELECT COUNT(*) FROM interaction_action
                                WHERE video_id={vid} AND user_id BETWEEN 10001 AND {10000+n} AND action_type='{scene.upper()}' AND status=1)
                                FROM video_stat WHERE video_id={vid}""")
                                return list(map(int,vals.split())) == [base+n,n]
                            until(persisted)
                            sample["drain_observed_s"] = time.perf_counter()-front_end
                            if mode=="async":
                                until(lambda:self.request(mode,"GET","/__lab/consumed")==prior+1+n)
                                sample["consumed"]=n
                            if scene=="like":
                                assert int(self.sql(f"SELECT COUNT(*) FROM user_message WHERE type='LIKE' AND event_id LIKE 'interaction:like:{vid}:%'"))==n
                        elif scene=="recommend_scroll":
                            mode="before" if v=="A" else "after"
                            duplicate_counts=[]
                            def scroll(i):
                                cursor=""
                                seen=set()
                                for page in range(8):
                                    result=self.page(mode,"recommend",cursor,require_next=page<7)
                                    seen.update(r["video_id"] for r in result["items"])
                                    cursor=result["next_cursor"]
                                duplicate_counts.append(80-len(seen))
                            scroll(-1)
                            duplicate_counts.clear()
                            sample=self.measure(scroll,30,4)
                            sample["duplicates"]=sum(duplicate_counts)
                            if v=="B": assert sample["duplicates"]==0
                        else:
                            feed = scene.split("_")[0]
                            mode = ("before" if feed=="recommend" else "sql") if v=="A" else "after"
                            cursor = ""
                            if scene=="recommend_page":
                                cursor=self.page(mode,feed)["next_cursor"]
                            expected=self.page(mode,feed,cursor)["items"]
                            if feed in ("timeline","following"):
                                assert expected==self.page("sql",feed)["items"]
                            def read(i):
                                result=self.page(mode,feed,cursor)
                                assert result["items"]==expected
                            n = 100 if feed=="recommend" else 480
                            sample=self.measure(read,n,4 if feed=="recommend" else 8)
                        save(scene,v,rd,sample)


def main(out):
    if not __debug__:
        raise RuntimeError("run with Python assertions enabled")
    out.mkdir(exist_ok=False)
    # Short private socket paths, ext4-backed MySQL / RabbitMQ storage (not tmpfs).
    with tempfile.TemporaryDirectory(prefix=".drbench-",dir=Path.home()) as tmp, contextlib.ExitStack() as stack:
        work=Path(tmp)
        os.environ.update(DREAMREEL_LAB_DIR=tmp, LAB_JWT=secrets.token_hex(32), GOMAXPROCS="4", TZ="UTC")
        lab=Lab(work,out,stack)
        for version in ("before","after"):
            command("go","build","-o",str(work/(version+"-api")),"./cmd/lab-api",cwd=ROOT/version)
        lab.services()
        for mode in ("before","sql","after","async"):
            lab.server(mode)
        lab.seed()
        anchor=lab.sql("SELECT CONCAT(published_at),id FROM video ORDER BY published_at DESC,id DESC LIMIT 1 OFFSET 8999").split("\t")
        query="SELECT id AS video_id,author_id,published_at FROM video WHERE status=2 AND published_at IS NOT NULL "
        order=" ORDER BY published_at DESC,id DESC LIMIT 11"
        cursor=f" AND (published_at<'{anchor[0]}' OR (published_at='{anchor[0]}' AND id<{int(anchor[1])}))"
        (out/"plans.txt").write_text("\n".join(q+"\n"+lab.sql("EXPLAIN ANALYZE "+q).replace("\\n","\n") for q in (query+order+" OFFSET 9000",query+cursor+order)))
        environment = dict(date=datetime.datetime.now(datetime.timezone.utc).isoformat(), kernel=platform.release(),
                           go=command("go","version",capture_output=True).stdout.strip(), cpu=command("lscpu",capture_output=True).stdout,
                           mysql=lab.sql("SELECT VERSION(),@@innodb_flush_log_at_trx_commit,@@sync_binlog,@@innodb_buffer_pool_size"),
                           redis=command("redis-server","--version",capture_output=True).stdout.strip(),
                           disk=command("df","-Th",tmp,capture_output=True).stdout,
                           rabbitmq=command("dpkg-query","-W","rabbitmq-server",capture_output=True).stdout.strip(),
                           fixture=lab.sql("SELECT (SELECT COUNT(*) FROM video),(SELECT COUNT(*) FROM account),(SELECT COUNT(*) FROM user_follow),(SELECT COUNT(*) FROM video_embedding)"))
        (out/"environment.json").write_text(json.dumps(environment,indent=2))
        shutil.copyfile(ROOT/"source.json",out/"source.json")
        os.environ["LAB_COMPONENT_RESULTS"]=str(out/"components.jsonl")
        command("go","test","-tags=labperf","-v","-run","^TestLab$","-count=1","-timeout=15m","./test/performance",cwd=ROOT/"after")
        lab.http()
    command("node",str(ROOT/"buffer.mjs"),str(out/"buffer.jsonl"))
    command(sys.executable,str(ROOT/"analyze.py"),str(out))


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output",type=Path)
    main(parser.parse_args().output.resolve())
