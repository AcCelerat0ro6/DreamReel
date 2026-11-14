function recentScore(item) {
  const count = value => Number.isFinite(Number(value)) ? Math.max(0, Number(value)) : 0;
  return count(item.like_count) * 3 + count(item.comment_count) * 5 + count(item.favorite_count) * 4;
}

// 精选仅对当前召回的近期作品排序，不代表全站或实时热榜。
export function rankRecentVideos(items = []) {
  const published = item => Date.parse(item.published_at) || 0;
  return [...items].sort((left, right) => recentScore(right) - recentScore(left)
    || published(right) - published(left) || Number(right.video_id) - Number(left.video_id));
}

// 空榜首屏使用近期精选兜底，后续分页保持实际来源与游标配对。
export async function fetchHotFeedPage({ fetchPage, cursor = "", source = "hot" }) {
  const page = await fetchPage(source, cursor);
  if (source === "hot" && !cursor && !(page.items || []).length && !page.has_more) {
    const recent = await fetchPage("timeline", "");
    return { ...recent, items: rankRecentVideos(recent.items), source: "timeline" };
  }
  return source === "timeline"
    ? { ...page, items: rankRecentVideos(page.items), source }
    : { ...page, source };
}
