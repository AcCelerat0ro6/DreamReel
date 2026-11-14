import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { appendFeedItems, feedTrimCount } from "./web/feedBuffer.js";

const pages = Array.from({ length: 1000 }, (_, page) => Array.from({ length: 10 }, (_, i) => ({
  video_id: page * 10 + i + 1, title: `video-${page * 10 + i + 1}`
})));

function scroll(bounded) {
  let items = [];
  for (const page of pages) {
    items = appendFeedItems(items, page);
    // Model continuous forward consumption, currently watching the last item of this page.
    const remove = bounded ? feedTrimCount(items.length, items.length - 1) : 0;
    if (remove) items = items.slice(remove);
  }
  assert.equal(items.at(-1).video_id, 10000);
  assert.equal(items.at(-21).video_id, 9980);
  assert.equal(items.length, bounded ? 200 : 10000);
  return items.length;
}

let lines = "";
for (let round = 1; round <= 4; round++) {
  for (const variant of round % 2 ? "AB" : "BA") {
    const bounded = variant === "B";
    const retained_items = scroll(bounded);
    const sample_ms = [];
    for (let i = 0; i < 5; i++) {
      const start = performance.now();
      scroll(bounded);
      sample_ms.push(performance.now() - start);
    }
    lines += JSON.stringify({ case: "feed_buffer", variant, round, sample_ms, retained_items, node: process.version }) + "\n";
  }
}
writeFileSync(process.argv[2], lines, { flag: "wx" });
