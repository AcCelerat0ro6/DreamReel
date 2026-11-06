"""汇总逐轮数据，P95 使用各轮 P95 的中位数，并保留每轮原始测量。"""
import csv
import json
import statistics
import sys
from pathlib import Path

root = Path(sys.argv[1])
rows = []
for path in sorted(root.glob("*-r*.json")):
    data = json.loads(path.read_text(encoding="utf-8-sig"))
    metrics = data["metrics"]
    values = metrics["recommendation_latency"]["values"]
    server = data["benchmark"]["server"]
    if "elapsed_seconds" not in server:
        continue
    requests = metrics["recommendation_requests"]["values"]["count"]
    def counter(name):
        return metrics.get(name, {}).get("values", {}).get("count", 0)
    row = {
        "file": path.name,
        "variant": path.name.split("-")[0],
        "mode": data["benchmark"]["mode"],
        "requests": requests,
        "elapsed_seconds": server["elapsed_seconds"],
        "measured_rps": requests / server["elapsed_seconds"],
        "mean_ms": values["avg"],
        "median_ms": values["med"],
        "p95_ms": values["p(95)"],
        "p99_ms": values["p(99)"],
        "valid_rate": metrics["valid_page"]["values"]["rate"],
        "dropped_iterations": counter("dropped_iterations"),
        "duplicate_videos": counter("duplicate_videos"),
        "queries": server["queries"],
        "queries_per_request": server["queries"] / requests,
        "journeys": counter("completed_journeys"),
        "first_page_p95_ms": metrics.get("first_page_latency", {}).get("values", {}).get("p(95)"),
        "cached_page_p95_ms": metrics.get("cached_page_latency", {}).get("values", {}).get("p(95)"),
        "refill_page_p95_ms": metrics.get("refill_page_latency", {}).get("values", {}).get("p(95)"),
        "go_heap_end_bytes": server["go_heap_bytes"],
        "redis_keys_end": server["redis_keys"],
    }
    rows.append(row)
groups = {}
for row in rows:
    groups.setdefault(row["variant"] + "-" + row["mode"], []).append(row)
summary = {}
for key, group in groups.items():
    total = sum(row["requests"] for row in group)
    summary[key] = {
        "rounds": len(group),
        "requests": total,
        "median_round_rps": statistics.median(row["measured_rps"] for row in group),
        "median_round_mean_ms": statistics.median(row["mean_ms"] for row in group),
        "median_round_p50_ms": statistics.median(row["median_ms"] for row in group),
        "median_round_p95_ms": statistics.median(row["p95_ms"] for row in group),
        "median_round_p99_ms": statistics.median(row["p99_ms"] for row in group),
        "queries_per_request": sum(row["queries"] for row in group) / total,
        "min_valid_rate": min(row["valid_rate"] for row in group),
        "dropped_iterations": sum(row["dropped_iterations"] for row in group),
        "duplicate_videos": sum(row["duplicate_videos"] for row in group),
    }
    for metric in ("first_page_p95_ms", "cached_page_p95_ms", "refill_page_p95_ms"):
        samples = [row[metric] for row in group if row[metric] is not None]
        if samples:
            summary[key]["median_round_" + metric] = statistics.median(samples)
with (root / "rounds.csv").open("w", encoding="utf-8-sig", newline="") as out:
    writer = csv.DictWriter(out, fieldnames=list(rows[0]))
    writer.writeheader()
    writer.writerows(rows)
(root / "analysis.json").write_text(json.dumps({"summary": summary, "rounds": rows}, ensure_ascii=False, indent=2), encoding="utf-8")
print(json.dumps(summary, ensure_ascii=False, indent=2))
