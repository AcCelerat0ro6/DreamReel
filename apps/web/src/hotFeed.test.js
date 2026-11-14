import test from "node:test";
import assert from "node:assert/strict";
import { fetchHotFeedPage, rankRecentVideos } from "./hotFeed.js";

const page = (items, next_cursor = "", has_more = false) => ({ items, next_cursor, has_more });

test("实时热门有数据时保留服务端顺序和游标，不请求兜底", async () => {
  const hot = page([{ video_id: 1 }, { video_id: 2, like_count: 100 }], "hot-next", true);
  const calls = [];
  const result = await fetchHotFeedPage({ fetchPage: async (...args) => { calls.push(args); return hot; } });
  assert.deepEqual(calls, [["hot", ""]]);
  assert.deepEqual(result.items, hot.items);
  assert.equal(result.source, "hot");
  assert.equal(result.next_cursor, "hot-next");
});

test("实时热门首屏为空时读取近期作品，保留来源和分页信息", async () => {
  const calls = [];
  const result = await fetchHotFeedPage({ fetchPage: async (scene, cursor) => {
    calls.push([scene, cursor]);
    return scene === "hot" ? page([]) : page([{ video_id: 1 }, { video_id: 2, favorite_count: 5 }], "timeline-next", true);
  } });
  assert.deepEqual(calls, [["hot", ""], ["timeline", ""]]);
  assert.deepEqual(result.items.map(item => item.video_id), [2, 1]);
  assert.equal(result.source, "timeline");
  assert.equal(result.next_cursor, "timeline-next");
  assert.equal(result.has_more, true);
});

test("精选后续分页沿用时间流游标，不传入热门接口", async () => {
  const calls = [];
  const result = await fetchHotFeedPage({ source: "timeline", cursor: "timeline-next", fetchPage: async (...args) => {
    calls.push(args); return page([{ video_id: 3 }], "timeline-after", true);
  } });
  assert.deepEqual(calls, [["timeline", "timeline-next"]]);
  assert.equal(result.next_cursor, "timeline-after");
  assert.equal(result.source, "timeline");
});

test("热门翻到末尾不混入其他来源；全库为空时保留空列表", async () => {
  const calls = [];
  await fetchHotFeedPage({ cursor: "hot-last", fetchPage: async (...args) => { calls.push(args); return page([]); } });
  assert.deepEqual(calls, [["hot", "hot-last"]]);
  const empty = await fetchHotFeedPage({ fetchPage: async () => page([]) });
  assert.deepEqual(empty.items, []);
  assert.equal(empty.has_more, false);
});

test("接口异常或取消直接返回错误，不把失败当作空热榜", async () => {
  for (const error of [Object.assign(new Error("failed"), { status: 500 }), new DOMException("cancelled", "AbortError")]) {
    let calls = 0;
    await assert.rejects(fetchHotFeedPage({ fetchPage: async () => { calls++; throw error; } }), expected => expected === error);
    assert.equal(calls, 1);
  }
});

test("近期作品按互动情况和发布时间稳定排序，不修改原始响应", () => {
  const items = [
    { video_id: 1, like_count: 2, published_at: "2026-11-05T00:00:00Z" },
    { video_id: 2, comment_count: 2 },
    { video_id: 3, like_count: 2, published_at: "2026-11-06T00:00:00Z" },
    { video_id: 4, like_count: 2, published_at: "2026-11-06T00:00:00Z" }
  ];
  const original = structuredClone(items);
  assert.deepEqual(rankRecentVideos(items).map(item => item.video_id), [2, 4, 3, 1]);
  assert.deepEqual(items, original);
});
