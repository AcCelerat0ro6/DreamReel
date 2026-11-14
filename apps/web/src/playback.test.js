import test from "node:test";
import assert from "node:assert/strict";
import { buildVideoShareURL, createPlaybackTracker, isPlaybackTap, toggleVideoPlayback, updatePlaybackGesture } from "./playback.js";
import { sharedVideoIndex } from "./discovery.js";

function playbackFixture() {
  let time = 0;
  let reportID = 0;
  const tracker = createPlaybackTracker({ now: () => time, makeReportID: () => String(++reportID) });
  const video = {
    paused: true,
    async play() { this.paused = false; tracker.playing(); },
    pause() { this.paused = true; }
  };
  return { tracker, video, advance: (elapsed) => { time += elapsed; } };
}

test("画面暂停后继续播放，QoS只累计实际播放片段且每次报告ID唯一", async () => {
  const { tracker, video, advance } = playbackFixture();
  advance(40);
  tracker.loaded();
  assert.equal(await toggleVideoPlayback(video), "playing");
  advance(1250);
  assert.equal(await toggleVideoPlayback(video), "paused");
  const first = tracker.flush();
  assert.deepEqual(first, { reportID: "1", firstFrameMs: 40, stutterCount: 0, watchMs: 1250 });
  advance(10000); // Ten seconds spent paused must not enter the next report.
  assert.equal(await toggleVideoPlayback(video), "playing");
  advance(750);
  await toggleVideoPlayback(video);
  const second = tracker.flush();
  assert.equal(second.watchMs, 750);
  assert.notEqual(second.reportID, first.reportID);
  assert.equal(second.firstFrameMs, undefined);
  assert.equal(tracker.flush(), null); // Unmount after pause must not double-report.
});

test("缓冲时间不计入观看时间，重复playing事件不重置开始时钟", () => {
  const { tracker, advance } = playbackFixture();
  tracker.playing(); // Starting at clock 0 is valid.
  advance(200);
  tracker.playing();
  advance(300);
  tracker.waiting();
  advance(5000);
  tracker.playing();
  advance(700);
  const report = tracker.flush();
  assert.equal(report.watchMs, 1200);
  assert.equal(report.stutterCount, 1);
});

test("浏览器拒绝播放时保留暂停状态，用户下次可再次尝试", async () => {
  const { tracker, video, advance } = playbackFixture();
  video.play = async () => { throw new Error("NotAllowedError"); };
  await assert.rejects(toggleVideoPlayback(video), /NotAllowedError/);
  advance(5000);
  assert.equal(video.paused, true);
  assert.equal(tracker.flush(), null);
  video.play = async () => { video.paused = false; tracker.playing(); };
  await toggleVideoPlayback(video);
  advance(300);
  await toggleVideoPlayback(video);
  assert.equal(tracker.flush().watchMs, 300);
});

test("轻触播放，超过拖动阈值、拖回起点、取消手势均不会误触暂停", () => {
  const start = { x: 120, y: 90, moved: false };
  assert.equal(isPlaybackTap(start, { clientX: 121, clientY: 92 }), true);
  assert.equal(isPlaybackTap(start, { clientX: 120, clientY: 98 }), false);
  let gesture = updatePlaybackGesture(start, { clientX: 120, clientY: 130 });
  gesture = updatePlaybackGesture(gesture, { clientX: 120, clientY: 90 });
  assert.equal(isPlaybackTap(gesture, { clientX: 120, clientY: 90 }), false);
  assert.equal(isPlaybackTap({ ...start, cancelled: true }, { clientX: 120, clientY: 90 }), false);
  assert.equal(isPlaybackTap(null, { clientX: 120, clientY: 90 }), false);
});

test("从登录推荐流复制的分享链接可由公开feed识别，清除旧筛选与hash", () => {
  const link = buildVideoShareURL("https://dreamreel.example/recommend?q=old&video=9#detail", 42);
  const url = new URL(link);
  assert.equal(url.origin, "https://dreamreel.example");
  assert.equal(url.pathname, "/timeline");
  assert.equal(url.search, "?video=42");
  assert.equal(url.hash, "");
  assert.equal(sharedVideoIndex([{ video_id: 1 }, { video_id: 42 }], url.search), 1);
  for (const id of [undefined, 0, -1, "bad", "2&injected=1", "9007199254740993"]) {
    assert.throws(() => buildVideoShareURL("https://dreamreel.example/following", id), /作品 ID 无效/);
  }
});
