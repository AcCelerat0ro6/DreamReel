export const TEXT_LIMITS = { title: 128, description: 512, model_name: 64, ai_style_tag: 32, model_params: 1024 };

// 与后端的字符计数保持一致，中文与英文均按一个字符计数。
export function characterCount(value = "") { return Array.from(String(value)).length; }

export function validateTextLength(value, label, limit) {
  if (characterCount(String(value || "").trim()) > limit) throw new Error(`${label}最多 ${limit} 字`);
}
