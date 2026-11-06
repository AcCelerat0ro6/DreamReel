import test from "node:test";
import assert from "node:assert/strict";
import { buildAIGCPayload } from "./aigc.js";

test("ordinary videos omit all optional metadata", () => {
  assert.deepEqual(buildAIGCPayload({}), {});
  assert.deepEqual(buildAIGCPayload({ model_name: "  ", model_params: "\n", origin_video_id: " " }), {});
});
test("AIGC values are trimmed and origin ID is numeric", () => {
  assert.deepEqual(buildAIGCPayload({ model_name: " Model v2 ", model_params: " seed=42\n", ai_style_tag: " 水墨 ", origin_video_id: " 12 " }), {
    model_name: "Model v2", model_params: "seed=42", ai_style_tag: "水墨", origin_video_id: 12
  });
});
test("limits follow backend UTF-8 byte counts", () => {
  assert.equal(buildAIGCPayload({ model_name: "中".repeat(21) }).model_name.length, 21);
  assert.throws(() => buildAIGCPayload({ model_name: "中".repeat(22) }), /模型名称过长/);
  assert.throws(() => buildAIGCPayload({ model_params: "x".repeat(1025) }), /生成参数过长/);
  assert.throws(() => buildAIGCPayload({ ai_style_tag: "x".repeat(33) }), /风格标签过长/);
});
test("invalid or lossy origin IDs are rejected", () => {
  for (const origin_video_id of ["0", "-1", "1.5", "1e2", "abc", "9007199254740993"]) {
    assert.throws(() => buildAIGCPayload({ origin_video_id }), /正整数/);
  }
});
