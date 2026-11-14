// The clock is injected in tests so a pause or buffering period can never count as watch time.
export function createPlaybackTracker({ now = () => performance.now(), makeReportID = () => `${Date.now()}-${Math.random().toString(36).slice(2, 8)}` } = {}) {
  let state;
  function reset() {
    state = { reportID: makeReportID(), loadStartedAt: now(), playingStartedAt: null, watchMs: 0, firstFrameMs: undefined, stutterCount: 0 };
  }
  function stopClock() {
    if (state.playingStartedAt !== null) {
      state.watchMs += Math.max(0, now() - state.playingStartedAt);
      state.playingStartedAt = null;
    }
  }
  reset();
  return {
    loaded() {
      if (state.firstFrameMs === undefined) state.firstFrameMs = Math.max(0, Math.round(now() - state.loadStartedAt));
    },
    playing() {
      if (state.playingStartedAt === null) state.playingStartedAt = now();
    },
    waiting() {
      stopClock();
      state.stutterCount += 1;
    },
    flush() {
      stopClock();
      const { reportID, firstFrameMs, stutterCount } = state;
      const watchMs = Math.round(state.watchMs);
      reset();
      return watchMs > 0 || stutterCount > 0 || firstFrameMs !== undefined
        ? { reportID, firstFrameMs, stutterCount, watchMs }
        : null;
    }
  };
}

export async function toggleVideoPlayback(video) {
  if (!video) return "unavailable";
  if (video.paused) {
    await video.play();
    return "playing";
  }
  video.pause();
  return "paused";
}

export function updatePlaybackGesture(start, point, threshold = 8) {
  if (!start) return null;
  const moved = Math.hypot(point.clientX - start.x, point.clientY - start.y) >= threshold;
  return moved ? { ...start, moved: true } : start;
}

export function isPlaybackTap(start, point) {
  return Boolean(start && !start.cancelled && !updatePlaybackGesture(start, point).moved);
}

// Shared works always open in the public feed, even when copied from a signed-in recommendation.
export function buildVideoShareURL(href, videoID) {
  if (!/^\d+$/.test(String(videoID)) || !Number.isSafeInteger(Number(videoID)) || Number(videoID) <= 0) {
    throw new Error("作品 ID 无效");
  }
  const url = new URL(href);
  url.pathname = "/timeline";
  url.search = "";
  url.hash = "";
  url.searchParams.set("video", String(videoID));
  return url.toString();
}
