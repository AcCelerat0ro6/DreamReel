export const RECOMMENDATION_REFRESH_MS = 30_000;
export const FEED_BUFFER_LIMIT = 200;
export const FEED_HISTORY_LIMIT = 20;

export function createRecommendationState() {
  return {
    source: "recommend",
    recommend: { cursor: "", hasMore: false, requestID: "" },
    timeline: { cursor: "", hasMore: false, requestID: "" },
    refreshAfter: 0,
    retryAfter: 0
  };
}

// 每次复制续刷进度，过期请求的结果可以直接丢弃，避免污染新页面。
export async function fetchRecommendationBatch(previous, { fetchPage, seenIDs, createRequestID, now = Date.now }) {
  const state = {
    ...previous,
    recommend: { ...previous.recommend },
    timeline: { ...previous.timeline }
  };
  let lastError = null;
  let succeeded = false;

  function result(items = [], scene = state.source) {
    return { state, items, scene, requestID: state[scene].requestID, error: succeeded ? null : lastError };
  }

  if (now() < state.retryAfter) return result();

  async function read(scene, fresh = false) {
    let progress = state[scene];
    if (fresh) {
      progress = { cursor: "", hasMore: false, requestID: createRequestID(scene) };
      state[scene] = progress;
    }
    // 允许越过少量全重复页面，同时限制一次补充的请求量。
    for (let page = 0; page < 3; page++) {
      const cursor = progress.cursor;
      let data;
      try {
        data = await fetchPage(scene, cursor, progress.requestID);
        succeeded = true;
      } catch (error) {
        if (error.status === 401 || error.name === "AbortError") throw error;
        lastError = error;
        progress.hasMore = false;
        progress.cursor = "";
        if (scene === "recommend" && error.status === 410) state.refreshAfter = 0;
        return [];
      }
      progress.cursor = data.next_cursor || "";
      progress.hasMore = Boolean(data.has_more && progress.cursor && progress.cursor !== cursor);
      const added = new Set();
      const items = (data.items || []).filter((item) => {
        if (!item.video_id || seenIDs.has(item.video_id) || added.has(item.video_id)) return false;
        added.add(item.video_id);
        return true;
      });
      if (items.length) return items;
      if (!progress.hasMore) break;
    }
    return [];
  }

  try {
    if (state.source === "recommend" && state.recommend.hasMore) {
      const items = await read("recommend");
      if (items.length) return result(items, "recommend");
    }
    if (now() >= state.refreshAfter) {
      state.refreshAfter = now() + RECOMMENDATION_REFRESH_MS;
      const items = await read("recommend", true);
      if (items.length) {
        state.source = "recommend";
        return result(items, "recommend");
      }
    }
    const items = await read("timeline", !state.timeline.hasMore);
    state.source = "timeline";
    if (items.length) return result(items, "timeline");
  } catch (error) {
    if (error.status === 401) throw error;
    lastError = error;
    succeeded = false;
  }
  state.retryAfter = now() + RECOMMENDATION_REFRESH_MS;
  return result();
}

// 复播使用独立的前端条目标识，视频本身的点赞、收藏仍按 video_id 关联。
export function createReplayBatch(items, requestID, random = Math.random) {
  const unique = new Map();
  for (const item of items) unique.set(item.video_id, item);
  const pool = [...unique.values()];
  for (let i = pool.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }
  const result = [];
  let previous = items.at(-1);
  while (pool.length) {
    let index = pool.findIndex((item) => item.video_id !== previous?.video_id && item.author_id !== previous?.author_id);
    if (index < 0) index = pool.findIndex((item) => item.video_id !== previous?.video_id);
    if (index < 0) index = 0;
    const [item] = pool.splice(index, 1);
    const next = { ...item, request_id: requestID, entry_id: `${requestID}:${item.video_id}` };
    result.push(next);
    previous = next;
  }
  return result;
}
