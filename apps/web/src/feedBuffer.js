import { FEED_BUFFER_LIMIT, FEED_HISTORY_LIMIT } from "./recommendationContinuation.js";

export function appendFeedItems(currentItems, nextItems) {
  if (!nextItems.length) return currentItems;
  const seen = new Set(currentItems.map((item) => item.video_id));
  const merged = [...currentItems];
  for (const item of nextItems) {
    if (seen.has(item.video_id)) continue;
    seen.add(item.video_id);
    merged.push(item);
  }
  return merged;
}

export function feedTrimCount(length, index) {
  return Math.max(0, Math.min(index - FEED_HISTORY_LIMIT, length - FEED_BUFFER_LIMIT));
}
