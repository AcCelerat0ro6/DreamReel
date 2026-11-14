import { characterCount } from "../text";

export default function CharacterCount({ value, limit, note = "" }) {
  const count = characterCount((value || "").trim());
  return <small className={`text-limit ${count > limit ? "is-over" : ""}`}>最多 {limit} 字 · 已输入 {count} 字{note && ` · ${note}`}</small>;
}
