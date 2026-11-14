import { useEffect, useState } from "react";
import Icon from "./components/Icon";

export default function AIGCMetadata({ video }) {
  const [copyStatus, setCopyStatus] = useState("");
  const [copying, setCopying] = useState(false);
  useEffect(() => { setCopyStatus(""); }, [video.video_id, video.id, video.model_params]);
  const fields = [
    ["模型", video.model_name],
    ["风格", video.ai_style_tag],
    ["生成参数", video.model_params],
    ["原视频 ID", video.origin_video_id]
  ].filter(([, value]) => value !== undefined && value !== null && String(value).trim() !== "");
  if (!fields.length) return null;

  async function copyParameters() {
    setCopying(true);
    try {
      if (!navigator.clipboard?.writeText) throw new Error("Clipboard unavailable");
      await navigator.clipboard.writeText(String(video.model_params));
      setCopyStatus("已复制，可以开始你的下一次创作了");
    } catch {
      setCopyStatus("复制失败，请选中上方参数手动复制");
    } finally {
      setCopying(false);
    }
  }

  return (
    <details className="aigc-metadata" onWheel={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()} onPointerDown={(event) => event.stopPropagation()}>
      <summary>
        <span className="aigc-badge">AIGC</span>
        <span>{video.model_name || "AI 创作信息"}{video.ai_style_tag ? ` · ${video.ai_style_tag}` : ""}</span>
        <span className="aigc-expand">创作幕后<Icon name="expand_more" size={14} /></span>
      </summary>
      <div className="aigc-details-content">
        <dl>{fields.map(([label, value]) => (
          <div key={label}><dt>{label}</dt><dd>{value}</dd></div>
        ))}</dl>
        {String(video.model_params || "").trim() && <div className="aigc-copy-row">
          <button type="button" onClick={copyParameters} disabled={copying}><Icon name={copyStatus.startsWith("已复制") ? "check" : "content_copy"} size={13} />{copying ? "复制中…" : "复制提示词 / 参数"}</button>
          {copyStatus && <span role="status">{copyStatus}</span>}
        </div>}
      </div>
    </details>
  );
}
