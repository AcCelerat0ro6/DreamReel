import { useCallback, useEffect, useRef, useState } from "react";
import AIGCMetadata from "../AIGCMetadata";
import { createPlaybackTracker, isPlaybackTap, toggleVideoPlayback, updatePlaybackGesture } from "../playback";
import Icon from "./Icon";
import "./VideoStage.css";

const fallbackCover = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720"><defs><linearGradient id="a" x2="1" y2="1"><stop stop-color="#27253c"/><stop offset="1" stop-color="#555071"/></linearGradient></defs><path fill="url(#a)" d="M0 0h1280v720H0z"/><circle cx="640" cy="330" r="90" fill="#ffffff" opacity=".05"/><path d="m623 295 55 35-55 35z" fill="#fff" opacity=".4"/></svg>')}`;
const isVideoSource = (source) => /\.(mp4|webm|ogg|mov|m4v)(\?|#|$)/i.test(source || "") || /^(blob:|data:video\/)/.test(source || "");
const isInteractive = (target) => Boolean(target?.closest?.("button, a, input, textarea, select, details, [contenteditable='true']"));

function clock(seconds) {
  const value = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0;
  return `${Math.floor(value / 60).toString().padStart(2, "0")}:${(value % 60).toString().padStart(2, "0")}`;
}

function metric(value) {
  const amount = Math.max(0, Number(value) || 0);
  return amount >= 10000 ? `${(amount / 10000).toFixed(amount >= 100000 ? 0 : 1).replace(/\.0$/, "")}万` : String(amount);
}

/** Shared by the feed; the existing feed, social actions and QoS API remain the source of truth. */
export default function VideoStage({ item, active, liked, favorited, following, followBusy, ownVideo, followError,
  onLike, onComment, onFavorite, onFollow, onPlaybackQoS, onOpenAuthor }) {
  const media = item.media_url || "";
  const cover = item.cover_url || fallbackCover;
  const showVideo = isVideoSource(media);
  const videoRef = useRef(null);
  const surfaceRef = useRef(null);
  const pointerRef = useRef(null);
  const itemRef = useRef(item);
  const reportRef = useRef(onPlaybackQoS);
  const qosRef = useRef(null);
  if (!qosRef.current) qosRef.current = createPlaybackTracker();
  const feedbackTimer = useRef(null);
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(true);
  const [fullscreen, setFullscreen] = useState(false);
  const [buffering, setBuffering] = useState(false);
  const [failedMedia, setFailedMedia] = useState("");
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [feedback, setFeedback] = useState("");

  itemRef.current = item;
  reportRef.current = onPlaybackQoS;

  const flushQoS = useCallback(() => {
    const metrics = qosRef.current.flush();
    if (metrics) reportRef.current?.(itemRef.current, metrics);
  }, []);

  useEffect(() => {
    qosRef.current = createPlaybackTracker();
    return flushQoS;
  }, [active, item.video_id, flushQoS]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !showVideo) return;
    if (active) {
      video.play()?.catch(() => { setPlaying(false); setBuffering(false); });
    } else {
      video.pause();
    }
  }, [active, media, showVideo]);

  useEffect(() => () => window.clearTimeout(feedbackTimer.current), []);

  useEffect(() => {
    const syncFullscreen = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', syncFullscreen);
    return () => document.removeEventListener('fullscreenchange', syncFullscreen);
  }, []);

  const notify = useCallback((message) => {
    window.clearTimeout(feedbackTimer.current);
    setFeedback(message);
    feedbackTimer.current = window.setTimeout(() => setFeedback(""), 2800);
  }, []);

  const togglePlayback = useCallback(() => {
    const video = videoRef.current;
    if (!active || !video || !showVideo) return;
    toggleVideoPlayback(video).catch(() => notify("暂时无法播放，请重试或切换作品"));
  }, [active, notify, showVideo]);

  useEffect(() => {
    if (!active) return undefined;
    function handleSpace(event) {
      if (event.code !== "Space" || event.repeat || event.altKey || event.ctrlKey || event.metaKey) return;
      if (isInteractive(event.target) || event.target?.closest?.(".comment-panel, .modal-backdrop")) return;
      event.preventDefault();
      togglePlayback();
    }
    window.addEventListener("keydown", handleSpace);
    return () => window.removeEventListener("keydown", handleSpace);
  }, [active, togglePlayback]);

  function handleLoadedData() {
    setBuffering(false);
    if (active) qosRef.current.loaded();
  }

  function handlePlaying() {
    setPlaying(true);
    setBuffering(false);
    if (!active) return;
    qosRef.current.playing();
  }

  function handleWaiting() {
    if (!active) return;
    setBuffering(true);
    qosRef.current.waiting();
  }

  function handleSurfaceClick(event) {
    if (isInteractive(event.target)) return;
    if (!isPlaybackTap(pointerRef.current, event)) return;
    togglePlayback();
  }

  async function enterFullscreen() {
    try {
      if (document.fullscreenElement || fullscreen) { await document.exitFullscreen(); setFullscreen(false); }
      else if (surfaceRef.current?.requestFullscreen) { await surfaceRef.current.requestFullscreen(); setFullscreen(true); }
      else if (videoRef.current?.webkitEnterFullscreen) videoRef.current.webkitEnterFullscreen();
      else notify("当前浏览器暂不支持全屏");
    } catch {
      notify("无法进入全屏，请稍后重试");
    }
  }

  function seek(event) {
    const video = videoRef.current;
    const nextTime = Number(event.target.value);
    if (video && Number.isFinite(duration) && duration > 0) {
      video.currentTime = nextTime;
      setCurrentTime(nextTime);
    }
  }

  const failed = failedMedia === media && Boolean(media);
  const creator = item.author || "DreamReel 创作者";

  return (
    <article className="video-stage dr-player">
      <div className={`dr-player-surface ${playing ? "is-playing" : "is-paused"}`} ref={surfaceRef}
        onPointerDown={(event) => { if (!isInteractive(event.target)) pointerRef.current = { x: event.clientX, y: event.clientY, moved: false }; }}
        onPointerMoveCapture={(event) => { pointerRef.current = updatePlaybackGesture(pointerRef.current, event); }}
        onPointerCancel={() => { if (pointerRef.current) pointerRef.current.cancelled = true; }}
        onClick={handleSurfaceClick}>
        <img className="dr-player-backdrop" src={cover} alt="" onError={(event) => { event.currentTarget.src = fallbackCover; }} />
        {showVideo ? (
          <video ref={videoRef} className="dr-player-media" src={media} poster={cover} autoPlay={active} muted={muted} loop playsInline
            preload={active ? "metadata" : "none"} aria-label={item.title || "AI 创作视频"}
            onLoadedData={handleLoadedData}
            onLoadedMetadata={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
            onDurationChange={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
            onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
            onPlaying={handlePlaying} onWaiting={handleWaiting}
            onPause={() => { setPlaying(false); setBuffering(false); flushQoS(); }}
            onEnded={flushQoS}
            onError={() => { setFailedMedia(media); setBuffering(false); setPlaying(false); flushQoS(); }} />
        ) : (
          <img className="dr-player-media" src={media || cover} alt={item.title || "AI 创作作品"}
            onError={(event) => { event.currentTarget.src = fallbackCover; }} />
        )}
        {failed ? (
          <div className="dr-player-error" role="status">
            <Icon name="videocam_off" size={30} /><strong>这个灵感暂时无法播放</strong>
            <span>视频地址可能已失效，试试重新加载或切换作品。</span>
            <button type="button" onClick={() => { setFailedMedia(""); videoRef.current?.load(); if (active) videoRef.current?.play()?.catch(() => {}); }}>
              <Icon name="refresh" size={16} />重新加载
            </button>
          </div>
        ) : showVideo && !playing && !buffering ? (
          <button className="dr-player-bigplay" type="button" aria-label="播放视频" onClick={togglePlayback}><Icon name="play_arrow" size={30} /></button>
        ) : null}
        {buffering && !failed && <span className="dr-player-buffer" role="status" aria-label="视频缓冲中" />}
        {showVideo && (
          <div className="dr-player-controls" onClick={(event) => event.stopPropagation()}>
            <input className="dr-player-seek" type="range" aria-label="播放进度" min="0" max={duration || 1} step="0.1"
              value={Math.min(currentTime, duration || 1)} disabled={!duration || failed} onChange={seek}
              style={{ "--progress": `${duration ? (currentTime / duration) * 100 : 0}%` }} />
            <div className="dr-player-controlrow">
              <button type="button" aria-label={playing ? "暂停视频" : "播放视频"} title="空格：播放 / 暂停" onClick={togglePlayback} disabled={failed}>
                <Icon name={playing ? "pause" : "play_arrow"} size={20} />
              </button>
              <span className="dr-player-clock">{clock(currentTime)}<i>/</i>{clock(duration)}</span>
              <button type="button" aria-label={muted ? "开启声音" : "静音"} onClick={() => setMuted((value) => !value)}>
                <Icon name={muted ? "volume_off" : "volume_up"} size={18} />
              </button>
              <button type="button" aria-label={fullscreen ? "退出全屏" : "全屏播放"} onClick={enterFullscreen}><Icon name={fullscreen ? "fullscreen_exit" : "fullscreen"} size={19} /></button>
            </div>
          </div>
        )}
      </div>
      <div className="dr-player-info">
        <div className="dr-player-creatorline">
          <button className="dr-player-creator" type="button" onClick={() => onOpenAuthor?.({ id: item.author_id, nickname: creator, avatar_url: item.avatar_url, bio: item.author_bio || "" })}>
            <img src={item.avatar_url || "/avatar.svg"} alt="" onError={(event) => { event.currentTarget.src = "/avatar.svg"; }} />
            <strong>@{creator}</strong>
          </button>
          <button className={`dr-player-follow ${following ? "is-following" : ""}`} type="button" disabled={followBusy || ownVideo} onClick={onFollow}>
            {ownVideo ? "本人" : followBusy ? "处理中" : following ? "已关注" : "关注"}
          </button>
        </div>
        <h2 className="dr-player-title">{item.title || "未命名的作品"}</h2>
        {(item.description || item.model_name || item.model_params || item.ai_style_tag || item.origin_video_id) && (
          <details className="dr-player-details">
            <summary>作品详情<Icon name="expand_more" size={13}/></summary>
            <div className="dr-player-details-content">
              {item.description && <p className="dr-player-description">{item.description}</p>}
              <AIGCMetadata key={item.video_id} video={item}/>
            </div>
          </details>
        )}
        {followError && <p className="dr-player-socialerror" role="status">{followError}</p>}
      </div>
      <div className="dr-player-actions" aria-label="作品互动">
        <button type="button" aria-label={liked ? "取消点赞" : "点赞"} aria-pressed={Boolean(liked)} className={liked ? "is-liked" : ""} onClick={onLike}>
          <Icon name="favorite" size={27} fill={liked ? "currentColor" : "none"}/><span>{metric(item.like_count)}</span>
        </button>
        <button type="button" aria-label={favorited ? "取消收藏" : "收藏作品"} aria-pressed={Boolean(favorited)} className={favorited ? "is-saved" : ""} onClick={onFavorite}>
          <Icon name="bookmark" size={27} fill={favorited ? "currentColor" : "none"}/><span>{metric(item.favorite_count)}</span>
        </button>
        <button type="button" aria-label="查看评论" onClick={onComment}><Icon name="chat_bubble" size={27}/><span>{metric(item.comment_count)}</span></button>
      </div>
      {feedback && <div className="dr-player-feedback" role="status"><Icon name="check_circle" size={16} />{feedback}</div>}
    </article>
  );
}
