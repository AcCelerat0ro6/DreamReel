"""在工作区内构建两个版本的独立副本，原项目仅用于读取。"""
import argparse
import hashlib
import json
import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent
CURRENT = ROOT.parents[1] / "apps" / "api"
parser = argparse.ArgumentParser()
parser.add_argument("--baseline", default="G:/GoCode/GCFeed/apps/api")
args = parser.parse_args()
BASELINE = Path(args.baseline)
WORK = ROOT / ".work"
WORK.mkdir(exist_ok=True)
manifest = {}
for label, source, module in (("baseline", BASELINE, "GCFeed"), ("current", CURRENT, "DreamReel")):
    dest = WORK / label
    dest.mkdir(exist_ok=True)
    for name in ("go.mod", "go.sum"):
        shutil.copyfile(source / name, dest / name)
    files = []
    for path in (source / "internal").rglob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        rel = path.relative_to(source)
        target = dest / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        data = path.read_bytes()
        target.write_bytes(data)
        files.append((rel.as_posix(), hashlib.sha256(data).hexdigest()))
    registration = "if err=router.Register(g,cfg,db);err!=nil{return err}" if label == "baseline" else "if err=router.Register(g,cfg,db,rdb,nil);err!=nil{return err}"
    runner = (ROOT / "harness.go.tmpl").read_text(encoding="utf-8-sig").replace("@MODULE@", module).replace("@REGISTER@", registration).replace("@ZAPIMPORT@", '"go.uber.org/zap"' if label == "current" else "").replace("@ZAPINIT@", "zap.ReplaceGlobals(zap.NewNop())" if label == "current" else "")
    target = dest / "cmd" / "k6bench"
    target.mkdir(parents=True, exist_ok=True)
    (target / "main.go").write_text(runner, encoding="utf-8")
    subprocess.run(["gofmt", "-w", str(target / "main.go")], check=True)
    subprocess.run(["go", "build", "-o", str(WORK / f"{label}.exe"), "./cmd/k6bench"], cwd=dest, check=True)
    manifest[label] = {"source": str(source), "go_mod_sha256": hashlib.sha256((source / "go.mod").read_bytes()).hexdigest(), "files": sorted(files)}
(ROOT / "source-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
