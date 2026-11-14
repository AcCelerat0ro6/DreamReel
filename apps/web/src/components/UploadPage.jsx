import { useEffect, useMemo, useRef, useState } from "react";
import AIGCMetadata from "../AIGCMetadata";
import CreatorWelcome from "./CreatorWelcome";
import CharacterCount from "./CharacterCount";
import { validateTextLength } from "../text";
import { buildAIGCPayload } from "../aigc";
import { createUploadManager, uploadWithProgress } from "../upload";
import Icon from "./Icon";
import "./UploadPage.css";

const EMPTY_FORM = { title: "", description: "", model_name: "", model_params: "", ai_style_tag: "", origin_video_id: "" };
const MODELS = ["Sora", "可灵", "Runway", "Pika", "Veo"];
const STYLES = ["电影感", "写实", "动画", "超现实", "水墨"];

function readDraft(key) {
  try {
    const stored = JSON.parse(localStorage.getItem(key));
    if (!stored?.form || typeof stored.form !== "object") return { form: { ...EMPTY_FORM }, restored: false };
    const form = Object.fromEntries(Object.keys(EMPTY_FORM).map((field) => [field, typeof stored.form[field] === "string" ? stored.form[field] : ""]));
    return { form, restored: Object.values(form).some((value) => value.trim()) };
  } catch {
    return { form: { ...EMPTY_FORM }, restored: false };
  }
}

function useFilePreview(file) {
  const [url, setURL] = useState("");
  useEffect(() => {
    if (!file) { setURL(""); return; }
    const next = URL.createObjectURL(file);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return url;
}

function UploadProgress({ label, upload, disabled, onRetry }) {
  if (!upload) return null;
  const { phase, percent, error } = upload;
  const message = phase === "ready" ? "上传完成，已就绪"
    : phase === "processing" ? "传输完成，正在处理文件…"
    : phase === "error" ? error : `正在上传 ${percent}%`;
  return (
    <div className={`upload-progress ${phase}`}>
      <div className="upload-progress-copy">
        <span role="status">{label}：{message}</span>
        {phase === "error" && <button className="ghost-button compact" type="button" disabled={disabled} onClick={onRetry}>重试上传</button>}
      </div>
      <progress aria-label={`${label}上传进度`} value={percent} max="100" />
    </div>
  );
}

export default function UploadPage(props) {
  // A new account gets a separate component and cannot inherit another creator's draft or files.
  const draftKey = `dreamreel.uploadDraft.${props.session.user?.id || "guest"}`;
  return <CreatorStudio key={draftKey} {...props} draftKey={draftKey} />;
}

function CreatorStudio({ session, onNavigate, request, draftKey }) {
  const [initialDraft] = useState(() => readDraft(draftKey));
  const [form, setForm] = useState(initialDraft.form);
  const [draftStatus, setDraftStatus] = useState(initialDraft.restored ? "已恢复文字草稿，请重新选择视频和封面" : "文字草稿自动保存在本机");
  const [uploads, setUploads] = useState({ video: null, cover: null });
  const [status, setStatus] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const submittingRef = useRef(false);
  const publishAbortRef = useRef(null);
  const publishAttemptRef = useRef(null);
  const publishedRef = useRef(false);
  const creationFormRef = useRef(null);
  const videoFile = uploads.video?.file || null;
  const coverFile = uploads.cover?.file || null;
  const coverPreview = useFilePreview(coverFile);
  const videoPreview = useFilePreview(videoFile);
  const uploadManager = useMemo(() => createUploadManager({
    upload: (file, kind, options) => uploadWithProgress(file, kind, session.token, options),
    onChange: (kind, state) => setUploads((previous) => ({ ...previous, [kind]: state }))
  }), [session.token]);

  useEffect(() => {
    setUploads({ video: null, cover: null });
    return () => {
      uploadManager.cancelAll();
      publishAbortRef.current?.abort();
      publishAbortRef.current = null;
      submittingRef.current = false;
    };
  }, [uploadManager]);

  useEffect(() => {
    if (!session.token || publishedRef.current) return;
    try {
      if (Object.values(form).some((value) => value.trim())) {
        localStorage.setItem(draftKey, JSON.stringify({ form, updatedAt: Date.now() }));
        setDraftStatus(initialDraft.restored && form === initialDraft.form ? "已恢复文字草稿，请重新选择视频和封面" : "文字草稿已自动保存到本机");
      } else {
        localStorage.removeItem(draftKey);
        setDraftStatus("文字草稿自动保存在本机");
      }
    } catch {
      setDraftStatus("当前浏览器无法保存草稿，请保留此页面");
    }
  }, [form, draftKey, session.token, initialDraft]);

  function update(field, value) { setForm((previous) => ({ ...previous, [field]: value })); }

  function clearDraft() {
    setForm({ ...EMPTY_FORM });
    setStatus("");
    // The saving effect removes the storage record; selected files remain available.
    setDraftStatus("文字草稿已清空");
  }

  async function handleSubmit(event) {
    event.preventDefault();
    if (submittingRef.current) return;
    submittingRef.current = true;
    const controller = new AbortController();
    publishAbortRef.current = controller;
    setStatus("");
    try {
      if (!form.title.trim()) throw new Error("请输入视频标题");
      validateTextLength(form.title, "标题", 128);
      validateTextLength(form.description, "简介", 512);
      if (!videoFile) throw new Error("请选择视频文件");
      if (!coverFile) throw new Error("请选择封面文件");
      const aigc = buildAIGCPayload(form);
      setSubmitting(true);
      setStatus("正在等待文件上传和处理完成…");
      const [videoUpload, coverUpload] = await Promise.all([uploadManager.ensure("video"), uploadManager.ensure("cover")]);
      if (controller.signal.aborted) return;
      const payload = { title: form.title.trim(), description: form.description.trim(), media_url: videoUpload.url, cover_url: coverUpload.url, ...aigc };
      const signature = JSON.stringify(payload);
      if (publishAttemptRef.current?.signature !== signature) {
        publishAttemptRef.current = { signature, key: `web-upload-${Date.now()}-${Math.random().toString(36).slice(2, 10)}` };
      }
      setPublishing(true);
      setStatus("文件已就绪，正在发布作品…");
      await request("/api/videos", {
        method: "POST", token: session.token, signal: controller.signal,
        headers: { "Idempotency-Key": publishAttemptRef.current.key }, body: payload
      });
      if (controller.signal.aborted) return;
      publishedRef.current = true;
      try { localStorage.removeItem(draftKey); } catch { /* Publication must succeed even if storage is unavailable. */ }
      setStatus("发布成功");
      onNavigate("/profile");
    } catch (error) {
      if (controller.signal.aborted) return;
      if (error.status === 401) { session.clearAuth(); onNavigate("/auth"); return; }
      setStatus(error.message || "发布失败，请重试");
    } finally {
      if (publishAbortRef.current === controller) {
        publishAbortRef.current = null;
        submittingRef.current = false;
        setSubmitting(false);
        setPublishing(false);
      }
    }
  }

  if (!session.token) return (
    <main className="upload-page studio-page"><section className="upload-card"><div className="upload-empty">
      <Icon name="lock" size={32} /><h1>登录，开始你的下一部作品</h1><p>分享 AI 生成的视频，连接同样充满想象力的创作者。</p>
      <button className="primary-button" onClick={() => onNavigate("/auth")}><Icon name="login" />登录 / 注册</button>
    </div></section></main>
  );

  return (
    <main className="upload-page studio-page">
      <CreatorWelcome onNavigate={onNavigate} onCreate={() => creationFormRef.current?.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "start" })}/>
      <div className="studio-draft-bar"><span role="status"><Icon name="check" size={16} />{draftStatus}</span><button type="button" disabled={submitting || !Object.values(form).some(Boolean)} onClick={clearDraft}>清空文字草稿</button></div>
      <div className="studio-layout">
        <form ref={creationFormRef} aria-label="创作表单" className="upload-form studio-form" onSubmit={handleSubmit} aria-busy={submitting}>
          <section className="studio-section">
            <header><span className="studio-step">01</span><div><h2>上传影像</h2><p>选择文件后自动上传，填写完成后再发布。</p></div></header>
            <div className="studio-file-grid">
              {[
                { kind: "video", label: "视频", file: videoFile, icon: "movie", accept: ".mp4,.mov,.webm", hint: "MP4 / MOV / WebM · 最大 512 MB" },
                { kind: "cover", label: "封面", file: coverFile, icon: "image", accept: ".jpg,.jpeg,.png,.webp", hint: "JPG / PNG / WebP · 最大 20 MB" }
              ].map(({ kind, label, file, icon, accept, hint }) => <div className="upload-file-field" key={kind}>
                <label className={`studio-file-picker ${file ? "has-file" : ""}`}>
                  <span className="studio-file-icon"><Icon name={icon} size={25} /></span>
                  <strong>{file ? file.name : `选择${label}文件`}</strong><small>{hint}</small><span className="studio-select-label">{file ? "更换文件" : "浏览文件"}<Icon name="arrow_forward" size={14} /></span>
                  <input type="file" aria-label={`选择${label}文件`} accept={accept} disabled={submitting} onChange={(event) => { uploadManager.select(kind, event.target.files?.[0] || null); event.target.value = ""; }} />
                </label>
                <UploadProgress label={label} upload={uploads[kind]} disabled={submitting} onRetry={() => uploadManager.ensure(kind).catch(() => {})} />
              </div>)}
            </div>
          </section>
          <section className="studio-section">
            <header><span className="studio-step">02</span><div><h2>关于这部作品</h2><p>一个好名字，让你的想象更容易被发现。</p></div></header>
            <label><span>作品标题 <em>*</em></span><input value={form.title} disabled={submitting} required onChange={(event) => update("title", event.target.value)} placeholder="为你的下一帧取个名字" /><CharacterCount value={form.title} limit={128}/></label>
            <label><span>作品简介</span><textarea value={form.description} disabled={submitting} onChange={(event) => update("description", event.target.value)} placeholder="讲讲作品背后的故事、灵感或创作心得…" rows={3} /><CharacterCount value={form.description} limit={512}/></label>
          </section>
          <fieldset className="studio-section studio-aigc" disabled={submitting}>
            <legend className="sr-only">AIGC 创作信息，选填</legend>
            <header><span className="studio-step">03</span><div><h2>解锁创作幕后 <span className="studio-optional">选填</span></h2><p>填写模型名称，作品将计入社区模型榜单。</p></div></header>
            <div className="studio-field"><span>生成模型</span><div className="studio-chip-group" role="group" aria-label="快捷选择模型">{MODELS.map((model) => <button key={model} type="button" aria-pressed={form.model_name === model} className={form.model_name === model ? "active" : ""} onClick={() => update("model_name", model)}>{model}</button>)}</div><input aria-label="生成模型" value={form.model_name} onChange={(event) => update("model_name", event.target.value)} placeholder="选择模型，或输入模型名称及版本" /><CharacterCount value={form.model_name} limit={64}/></div>
            <div className="studio-field"><span>影像风格</span><div className="studio-chip-group" role="group" aria-label="快捷选择风格">{STYLES.map((style) => <button key={style} type="button" aria-pressed={form.ai_style_tag === style} className={form.ai_style_tag === style ? "active" : ""} onClick={() => update("ai_style_tag", style)}>{style}</button>)}</div><input aria-label="影像风格" value={form.ai_style_tag} onChange={(event) => update("ai_style_tag", event.target.value)} placeholder="选择或自定义风格" /><CharacterCount value={form.ai_style_tag} limit={32}/></div>
            <label><span>提示词 / 生成参数</span><textarea value={form.model_params} onChange={(event) => update("model_params", event.target.value)} rows={4} placeholder="分享 Prompt、分辨率、种子、镜头运动等，让灵感可以延续…" /><CharacterCount value={form.model_params} limit={1024} note="发布后公开展示"/></label>
            <label><span>原视频 ID</span><input value={form.origin_video_id} onChange={(event) => update("origin_video_id", event.target.value)} inputMode="numeric" pattern="[0-9]+" placeholder="基于已有作品创作？在这里注明原视频 ID" /></label>
          </fieldset>
          <div className="studio-publish-area">
            {status && <p className={`form-message ${status === "发布成功" || submitting ? "success" : ""}`} role="status">{status}</p>}
            <button className="primary-button studio-publish" disabled={submitting}><Icon name="publish" />{submitting ? publishing ? "正在发布…" : "等待文件就绪…" : "发布作品"}<Icon name="arrow_forward" size={17} /></button>
            <p>发布后可在「我的作品」查看作品状态和互动反馈。</p>
          </div>
        </form>
        <aside className="studio-preview-column">
          <section className="studio-preview-card">
            <header><span><Icon name="visibility" size={17} />作品预览</span><span>本地预览</span></header>
            <div className="studio-preview-frame">{videoPreview ? <video key={videoPreview} src={videoPreview} poster={coverPreview || undefined} controls playsInline preload="metadata" /> : coverPreview ? <img src={coverPreview} alt="作品封面预览" /> : <div className="studio-preview-empty"><Icon name="movie" size={36} /><span>你的想象，即将在此放映</span><small>选择视频和封面，查看实时预览</small></div>}</div>
            <div className="studio-preview-copy"><span className="studio-model-tag"><Icon name="auto_awesome" size={14} />{form.model_name || "AI GENERATED"}</span><h2>{form.title || "下一部精彩，来自你"}</h2><p>{form.description || "关于作品的灵感、故事与创作过程。"}</p><AIGCMetadata video={form} /></div>
          </section>
          <section className="studio-tips"><Icon name="auto_awesome" size={22} /><h3>让灵感被看见</h3><p>清晰的封面、准确的模型版本，还有一段有趣的创作幕后，都能帮助观众更好地理解你的作品。</p><div><Icon name="bookmark" size={16} /><span>草稿仅保存文字，不保存视频与封面文件。再次打开时，请重新选择文件。</span></div></section>
        </aside>
      </div>
    </main>
  );
}
