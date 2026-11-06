export function buildAIGCPayload(form) {
  const payload = {};
  for (const [key, label, maxBytes] of [
    ["model_name", "模型名称", 64],
    ["model_params", "生成参数", 1024],
    ["ai_style_tag", "风格标签", 32]
  ]) {
    const value = (form[key] || "").trim();
    if (new TextEncoder().encode(value).length > maxBytes) {
      throw new Error(`${label}过长，请控制在 ${maxBytes} 字节以内（中文通常占 3 字节）`);
    }
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
