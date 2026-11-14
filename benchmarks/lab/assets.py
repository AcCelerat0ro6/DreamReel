"""Read-only validation of the deployed Vite bundle's HTTP cache validator."""
import hashlib
from html.parser import HTMLParser
import http.client
import json
from pathlib import Path
import sys
import time


class Scripts(HTMLParser):
    src = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "script" and attrs.get("type") == "module" and attrs.get("src", "").startswith("/assets/"):
            self.src = attrs["src"]


def run(path):
    conn = http.client.HTTPConnection("127.0.0.1", 80, timeout=5)
    def get(url, headers):
        conn.request("GET", url, headers=headers)
        response = conn.getresponse()
        return response, response.read()
    html = Scripts()
    response, body = get("/", {})
    assert response.status == 200
    html.feed(body.decode())
    assert html.src and not html.src.startswith("//")
    original, script = get(html.src, {})
    etag = original.getheader("ETag")
    assert original.status == 200 and etag and script
    with path.open("x") as out:
        for rd in range(1,5):
            for v in ("AB" if rd%2 else "BA"):
                samples, sizes = [], []
                for _ in range(20):
                    start=time.perf_counter_ns()
                    response, body=get(html.src, {} if v=="A" else {"If-None-Match":etag})
                    samples.append((time.perf_counter_ns()-start)/1e6)
                    assert (response.status,body)==((200,script) if v=="A" else (304,b""))
                    sizes.append(len(body))
                out.write(json.dumps(dict(case="static_revalidate",variant=v,round=rd,sample_ms=samples,
                    body_bytes=sizes[0],status=response.status,path=html.src,sha256=hashlib.sha256(script).hexdigest(),
                    cache_control=original.getheader("Cache-Control")))+"\n")
    conn.close()


if __name__=="__main__":
    run(Path(sys.argv[1]))
