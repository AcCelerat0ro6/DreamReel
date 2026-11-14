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
test("字数限制与后端一致，中文和英文均按字符计数", () => {
  for (const unit of ["中", "x", "🎬"]) {
    assert.equal(Array.from(buildAIGCPayload({ model_name: unit.repeat(64) }).model_name).length, 64);
    assert.throws(() => buildAIGCPayload({ model_name: unit.repeat(65) }), /模型名称最多 64 字/);
    assert.equal(Array.from(buildAIGCPayload({ model_params: unit.repeat(1024) }).model_params).length, 1024);
    assert.throws(() => buildAIGCPayload({ model_params: unit.repeat(1025) }), /生成参数最多 1024 字/);
    assert.equal(Array.from(buildAIGCPayload({ ai_style_tag: unit.repeat(32) }).ai_style_tag).length, 32);
    assert.throws(() => buildAIGCPayload({ ai_style_tag: unit.repeat(33) }), /风格标签最多 32 字/);
  }
});
test("invalid or lossy origin IDs are rejected", () => {
  for (const origin_video_id of ["0", "-1", "1.5", "1e2", "abc", "9007199254740993"]) {
    assert.throws(() => buildAIGCPayload({ origin_video_id }), /正整数/);
  }
});
