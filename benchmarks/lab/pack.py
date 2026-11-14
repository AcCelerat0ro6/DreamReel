"""Build a source-only archive for the Linux lab runner from a Git checkout."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]
BASE = "1cfc6fdf9f0df72ad934ace15db8b927f6064cd4"


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def pack(destination):
    manifest = {"baseline": BASE, "worktree_head": git("rev-parse", "HEAD").decode().strip(), "sha256": {}}
    with tarfile.open(destination, "x:gz") as bundle:
        def add(name, data):
            member = tarfile.TarInfo(name)
            member.size = len(data)
            bundle.addfile(member, io.BytesIO(data))
            manifest["sha256"][name] = hashlib.sha256(data).hexdigest()

        for version in ("before", "after"):
            if version == "before":
                raw = git("archive", BASE, "apps/api/internal", "apps/api/go.mod", "apps/api/go.sum")
                with tarfile.open(fileobj=io.BytesIO(raw)) as old:
                    for entry in old:
                        if entry.isfile() and entry.name.endswith((".go", "go.mod", "go.sum")):
                            add("before/" + entry.name.removeprefix("apps/api/"), old.extractfile(entry).read())
            else:
                api = ROOT / "apps/api"
                for file in sorted([api / "go.mod", api / "go.sum", *(api / "internal").rglob("*.go"),
                                    *(api / "test/performance").glob("*.go")]):
                    add("after/" + file.relative_to(api).as_posix(), file.read_bytes())
            add(version + "/cmd/lab-api/main.go", (ROOT / "benchmarks/lab/server.go.tmpl").read_bytes())
        for name in ("run.py", "analyze.py","assets.py","buffer.mjs"):
            add(name, (ROOT / "benchmarks/lab" / name).read_bytes())
        add("web/package.json", b'{"type":"module"}')
        for name in ("feedBuffer.js","recommendationContinuation.js"):
            add("web/"+name,(ROOT / "apps/web/src" / name).read_bytes())
        add("source.json", json.dumps(manifest, indent=2, sort_keys=True).encode())


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    pack(parser.parse_args().archive)
