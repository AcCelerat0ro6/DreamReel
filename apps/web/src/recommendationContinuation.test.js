import test from "node:test";
import assert from "node:assert/strict";
import {
  createRecommendationState,
  fetchRecommendationBatch,
  createReplayBatch,
  RECOMMENDATION_REFRESH_MS
} from "./recommendationContinuation.js";

const video = (video_id, author_id = video_id) => ({ video_id, author_id, liked: false });
const page = (items, next_cursor = "", has_more = false) => ({ items, next_cursor, has_more });
const error = (status) => Object.assign(new Error(String(status)), { status });

function options(fetchPage, seenIDs = [], now = 1000) {
  let request = 0;
  return { fetchPage, seenIDs: new Set(seenIDs), now: () => now, createRequestID: (scene) => `${scene}-${++request}` };
}

test("推荐空列表时按顺序请求时间流，保留实际来源", async () => {
  const calls = [];
  const batch = await fetchRecommendationBatch(createRecommendationState(), options(async (scene, cursor) => {
    calls.push([scene, cursor]);
    return scene === "recommend" ? page([]) : page([video(2)], "timeline-2", true);
  }));
  assert.deepEqual(calls, [["recommend", ""], ["timeline", ""]]);
  assert.equal(batch.scene, "timeline");
  assert.equal(batch.state.timeline.cursor, "timeline-2");
  assert.equal(batch.items[0].video_id, 2);
});

test("正常推荐分页复用游标，不额外刷新或读取时间流", async () => {
  const previous = createRecommendationState();
  previous.recommend = { cursor: "next", hasMore: true, requestID: "existing" };
  const batch = await fetchRecommendationBatch(previous, options(async (scene, cursor, requestID) => {
    assert.deepEqual([scene, cursor, requestID], ["recommend", "next", "existing"]);
    return page([video(2)], "after", true);
  }));
  assert.equal(batch.state.recommend.cursor, "after");
  assert.equal(previous.recommend.cursor, "next");
});

test("会话过期后重建推荐会话，再过滤已加载的视频", async () => {
  const previous = createRecommendationState();
  previous.refreshAfter = 99_000;
  previous.recommend = { cursor: "expired", hasMore: true, requestID: "old" };
  const calls = [];
  const batch = await fetchRecommendationBatch(previous, options(async (scene, cursor) => {
    calls.push([scene, cursor]);
    if (cursor === "expired") throw error(410);
    return page([video(1), video(2), video(2)]);
  }, [1]));
  assert.deepEqual(calls, [["recommend", "expired"], ["recommend", ""]]);
  assert.deepEqual(batch.items.map((item) => item.video_id), [2]);
  assert.notEqual(batch.requestID, "old");
});

test("全重复页面最多扫描三页，并保存续读位置", async () => {
  const previous = createRecommendationState();
  previous.refreshAfter = 99_000;
  previous.recommend = { cursor: "p0", hasMore: true, requestID: "old" };
  let calls = 0;
  const batch = await fetchRecommendationBatch(previous, options(async (scene) => {
    calls++;
    return scene === "recommend" ? page([video(1)], `p${calls}`, true) : page([]);
  }, [1]));
  assert.equal(calls, 4);
  assert.equal(batch.state.recommend.cursor, "p3");
  assert.deepEqual(batch.items, []);
});

test("无新内容时在冷却期内停止网络请求，到期后可重新尝试", async () => {
  let calls = 0;
  const fetchPage = async () => { calls++; return page([]); };
  const first = await fetchRecommendationBatch(createRecommendationState(), options(fetchPage));
  assert.equal(calls, 2);
  assert.equal(first.state.retryAfter, 1000 + RECOMMENDATION_REFRESH_MS);
  await fetchRecommendationBatch(first.state, options(fetchPage, [], 2000));
  assert.equal(calls, 2);
  await fetchRecommendationBatch(first.state, options(fetchPage, [], first.state.retryAfter));
  assert.equal(calls, 4);
});

test("权限失败立即交给登录处理，网络失败仍可尝试时间流", async () => {
  let calls = 0;
  await assert.rejects(fetchRecommendationBatch(createRecommendationState(), options(async () => {
    calls++; throw error(401);
  })), { status: 401 });
  assert.equal(calls, 1);
  const batch = await fetchRecommendationBatch(createRecommendationState(), options(async (scene) => {
    if (scene === "recommend") throw error(503);
    return page([video(3)]);
  }));
  assert.equal(batch.scene, "timeline");
  assert.equal(batch.items[0].video_id, 3);
});

test("取消请求后停止继续尝试，返回结果也不修改旧进度", async () => {
  const previous = createRecommendationState();
  let calls = 0;
  const batch = await fetchRecommendationBatch(previous, options(async () => {
    calls++;
    throw Object.assign(new Error("aborted"), { name: "AbortError" });
  }));
  assert.equal(calls, 1);
  assert.equal(batch.error.name, "AbortError");
  assert.deepEqual(previous, createRecommendationState());
});

test("复播轮次更换条目标识，并尽量避开上一条视频和作者", () => {
  const items = [video(1, 1), video(2, 2), video(3, 1)];
  const original = structuredClone(items);
  const replay = createReplayBatch(items, "replay-1", () => 0.99);
  assert.equal(replay[0].video_id, 2);
  assert.equal(replay[0].author_id, 2);
  assert.deepEqual(new Set(replay.map((item) => item.video_id)), new Set([1, 2, 3]));
  assert.ok(replay.every((item) => item.entry_id === `replay-1:${item.video_id}`));
  assert.deepEqual(items, original);
  assert.notEqual(createReplayBatch(items, "replay-2")[0].entry_id, replay[0].entry_id);
});

test("单视频和空列表都能安全生成下一轮，重复播放记录不会扩大素材池", () => {
  assert.deepEqual(createReplayBatch([], "empty"), []);
  const replay = createReplayBatch([video(1), video(1)], "single");
  assert.equal(replay.length, 1);
  assert.equal(replay[0].entry_id, "single:1");
  assert.equal(createReplayBatch([...replay, ...replay], "again").length, 1);
});
