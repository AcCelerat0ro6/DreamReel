"""构建两个 DreamReel 版本的独立压测副本。"""
import argparse
import hashlib
import json
import shutil
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent
CURRENT = ROOT.parents[1] / "apps" / "api"
parser = argparse.ArgumentParser()
parser.add_argument("--baseline", required=True, type=Path, help="基线工作区的 apps/api 路径")
args = parser.parse_args()
WORK = ROOT / ".work"
WORK.mkdir(exist_ok=True)
runner = (ROOT / "harness.go.tmpl").read_text(encoding="utf-8-sig")
runner = (runner.replace("@MODULE@", "DreamReel")
          .replace("@REGISTER@", "if err=router.Register(g,cfg,db,rdb,nil);err!=nil{return err}")
          .replace("@ZAPIMPORT@", '"go.uber.org/zap"')
          .replace("@ZAPINIT@", "zap.ReplaceGlobals(zap.NewNop())"))
manifest = {}
for label, source in (("baseline", args.baseline.resolve()), ("current", CURRENT)):
    module_file = source / "go.mod"
    if not module_file.is_file() or "module DreamReel" not in module_file.read_text(encoding="utf-8").splitlines():
        parser.error(f"{source} 不是 DreamReel 的 apps/api 目录")
    with tempfile.TemporaryDirectory(prefix=f"{label}-", dir=WORK) as temp_dir:
        dest = Path(temp_dir)
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
        target = dest / "cmd" / "k6bench" / "main.go"
        target.parent.mkdir(parents=True)
        target.write_text(runner, encoding="utf-8")
        subprocess.run(["gofmt", "-w", str(target)], check=True)
        subprocess.run(["go", "build", "-buildvcs=false", "-o", str(WORK / f"{label}.exe"), "./cmd/k6bench"], cwd=dest, check=True)
        manifest[label] = {
            "source": str(source),
            "go_mod_sha256": hashlib.sha256((dest / "go.mod").read_bytes()).hexdigest(),
            "files": sorted(files),
        }
(WORK / "source-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
