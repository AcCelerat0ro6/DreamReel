"""Reduce paired rounds without pooling their latency distributions."""
import csv
import json
import math
from pathlib import Path
from statistics import mean, median
import sys


def analyze(folder):
    rows = []
    names=["components","http"]
    if (folder/"buffer.jsonl").exists(): names.append("buffer")
    if (folder/"assets.jsonl").exists(): names.append("assets")
    for name in names:
        for line in (folder / (name + ".jsonl")).read_text().splitlines():
            run = json.loads(line)
            samples = sorted(run.pop("sample_ms"))
            n = len(samples)
            row = {k: run[k] for k in ("case", "variant", "round")}
            row.update(n=n, mean_ms=mean(samples))
            for p in (50, 95, 99):
                row[f"p{p}_ms"] = samples[math.ceil(p*n/100)-1]
            if "wall_s" in run:
                row["ops_s"] = n / run["wall_s"]
            for key in ("duplicates","consumed","body_bytes","retained_items"):
                if key in run: row[key]=run[key]
            for key in ("sql", "page_queries"):
                if key in run:
                    row[key+"_per_op"] = run[key]/n
            if "drain_observed_s" in run:
                row["drain_ms"] = run["drain_observed_s"]*1000
                row["persisted_ops_s"] = n / (run["wall_s"] + run["drain_observed_s"])
            rows.append(row)
    def write(name, records):
        with (folder / name).open("w", newline="") as f:
            w = csv.DictWriter(f, dict.fromkeys(k for r in records for k in r))
            w.writeheader()
            w.writerows(records)
    write("rounds.csv",rows)
    summary=[]
    for case in dict.fromkeys(r["case"] for r in rows):
        for variant in ("A","B"):
            group=[r for r in rows if r["case"]==case and r["variant"]==variant]
            if sorted(r["round"] for r in group)!=[1,2,3,4]:
                raise ValueError(f"incomplete {case} {variant}")
            result=dict(case=case,variant=variant,total_n=sum(r["n"] for r in group))
            result.update({k:(sum if k in ("duplicates","consumed") else median)(r[k] for r in group)
                           for k in group[0] if k not in ("case","variant","round","n")})
            summary.append(result)
    write("summary.csv",summary)


if __name__=="__main__":
    analyze(Path(sys.argv[1]))
