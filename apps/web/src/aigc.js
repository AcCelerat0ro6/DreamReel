import { validateTextLength } from "./text.js";

export function buildAIGCPayload(form) {
  const payload = {};
  for (const [key, label, maxCharacters] of [
    ["model_name", "模型名称", 64],
    ["model_params", "生成参数", 1024],
    ["ai_style_tag", "风格标签", 32]
  ]) {
    const value = (form[key] || "").trim();
    validateTextLength(value, label, maxCharacters);
    if (value) payload[key] = value;
  }
  const origin = (form.origin_video_id || "").trim();
  if (origin) {
    if (!/^\d+$/.test(origin) || !Number.isSafeInteger(Number(origin)) || Number(origin) <= 0) {
      throw new Error("原视频 ID 需为有效的正整数");
    }
    payload.origin_video_id = Number(origin);
  }
  return payload;
}
