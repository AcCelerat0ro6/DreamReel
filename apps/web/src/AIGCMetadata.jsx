export default function AIGCMetadata({ video }) {
  const fields = [
    ["模型", video.model_name],
    ["风格", video.ai_style_tag],
    ["生成参数", video.model_params],
    ["原视频 ID", video.origin_video_id]
  ].filter(([, value]) => value !== undefined && value !== null && String(value).trim() !== "");
  if (!fields.length) return null;

  return (
    <details className="aigc-metadata" onWheel={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
      <summary>
        <span className="aigc-badge">AIGC</span>
        <span>{video.model_name || "AI 创作信息"}{video.ai_style_tag ? ` · ${video.ai_style_tag}` : ""}</span>
        <span className="aigc-expand">详情</span>
      </summary>
      <dl>{fields.map(([label, value]) => (
        <div key={label}><dt>{label}</dt><dd>{value}</dd></div>
      ))}</dl>
    </details>
  );
}
