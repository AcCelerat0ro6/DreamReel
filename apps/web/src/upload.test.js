import test from "node:test";
import assert from "node:assert/strict";
import { createUploadManager, uploadWithProgress, validateUploadFile } from "./upload.js";

const file = (name = "video.mp4", size = 1024) => ({ name, size });
function fixture() {
  const calls = [];
  const changes = [];
  const manager = createUploadManager({
    upload(selected, kind, options) {
      let resolve, reject;
      const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
      calls.push({ file: selected, kind, ...options, resolve, reject });
      return promise;
    },
    onChange: (kind, state) => changes.push({ kind, state })
  });
  return { manager, calls, changes };
}

test("选择文件立即开始上传，点击发布复用同一个进行中的任务", async () => {
  const f = fixture();
  const pending = f.manager.select("video", file());
  assert.equal(f.calls.length, 1);
  assert.equal(f.manager.ensure("video"), pending);
  f.calls[0].onProgress({ phase: "processing", percent: 100 });
  assert.equal(f.changes.at(-1).state.phase, "processing");
  f.calls[0].resolve({ url: "/uploads/video/ready.mp4" });
  await pending;
  assert.equal(f.changes.at(-1).state.phase, "ready");
  assert.deepEqual(await f.manager.ensure("video"), { url: "/uploads/video/ready.mp4" });
  assert.equal(f.calls.length, 1);
});

test("文件可以并行上传，失败重试仅传输失败的文件", async () => {
  const f = fixture();
  const video = f.manager.select("video", file());
  const cover = f.manager.select("cover", file("cover.png"));
  assert.equal(f.calls.length, 2);
  f.calls[0].resolve({ url: "/video.mp4" });
  f.calls[1].reject(new Error("network failed"));
  await video;
  await assert.rejects(cover, /network failed/);
  assert.equal(f.changes.at(-1).state.phase, "error");
  const retry = Promise.all([f.manager.ensure("video"), f.manager.ensure("cover")]);
  assert.equal(f.calls.length, 3);
  assert.equal(f.calls[2].kind, "cover");
  f.calls[2].resolve({ url: "/cover.png" });
  assert.deepEqual(await retry, [{ url: "/video.mp4" }, { url: "/cover.png" }]);
});

test("更换文件取消旧上传，迟到的进度和响应不会覆盖新文件", async () => {
  const f = fixture();
  const old = f.manager.select("video", file("old.mp4"));
  const next = f.manager.select("video", file("new.mp4"));
  assert.equal(f.calls[0].signal.aborted, true);
  const count = f.changes.length;
  f.calls[0].onProgress({ phase: "processing", percent: 100 });
  f.calls[0].resolve({ url: "/old.mp4" });
  await old;
  assert.equal(f.changes.length, count);
  f.calls[1].resolve({ url: "/new.mp4" });
  await next;
  assert.equal(f.changes.at(-1).state.url, "/new.mp4");
});

test("离开页面清理任务后不再接收旧响应，严格模式重新挂载后仍可选择文件", async () => {
  const f = fixture();
  const pending = f.manager.select("video", file());
  f.manager.cancelAll();
  assert.equal(f.calls[0].signal.aborted, true);
  const count = f.changes.length;
  f.calls[0].reject(new DOMException("aborted", "AbortError"));
  await assert.rejects(pending, { name: "AbortError" });
  assert.equal(f.changes.length, count);
  await assert.rejects(f.manager.ensure("video"), /请选择视频文件/);
  const next = f.manager.select("video", file("next.mp4"));
  f.calls[1].resolve({ url: "/next.mp4" });
  await next;
  assert.equal(f.changes.at(-1).state.phase, "ready");
});

test("按后端格式和大小限制提前校验，不发送明显无效的文件", async () => {
  validateUploadFile(file("VIDEO.MP4", 512 * 1024 * 1024), "video");
  validateUploadFile(file("cover.webp", 20 * 1024 * 1024), "cover");
  assert.throws(() => validateUploadFile(file("bad.avi"), "video"), /仅支持/);
  assert.throws(() => validateUploadFile(file("video.mp4", 512 * 1024 * 1024 + 1), "video"), /512 MB/);
  assert.throws(() => validateUploadFile(file("cover.png", 20 * 1024 * 1024 + 1), "cover"), /20 MB/);
  assert.throws(() => validateUploadFile(file("empty.mp4", 0), "video"), /非空/);
  const f = fixture();
  await assert.rejects(f.manager.select("video", file("bad.avi")), /仅支持/);
  assert.equal(f.calls.length, 0);
  assert.equal(f.changes.at(-1).state.phase, "error");
});

class FakeXHR {
  static requests = [];
  constructor() { this.upload = {}; this.headers = {}; FakeXHR.requests.push(this); }
  open(method, url) { this.method = method; this.url = url; }
  setRequestHeader(key, value) { this.headers[key] = value; }
  send(data) { this.data = data; }
  abort() { this.aborted = true; this.onabort?.(); }
  respond(status, responseText) { this.status = status; this.responseText = responseText; this.onload(); }
}
function withXHR(fn) {
  const previous = globalThis.XMLHttpRequest;
  globalThis.XMLHttpRequest = FakeXHR;
  FakeXHR.requests = [];
  return Promise.resolve().then(fn).finally(() => { globalThis.XMLHttpRequest = previous; });
}

test("真实传输适配器保留 multipart 协议，进度 100% 后仍等待服务端成功响应", () => withXHR(async () => {
  const updates = [];
  const selected = new File(["test video"], "video.mp4", { type: "video/mp4" });
  let complete = false;
  const result = uploadWithProgress(selected, "video", "token", { onProgress: (value) => updates.push(value) });
  result.then(() => { complete = true; });
  const xhr = FakeXHR.requests[0];
  assert.deepEqual([xhr.method, xhr.url], ["POST", "/api/uploads"]);
  assert.equal(xhr.headers.Authorization, "Bearer token");
  assert.equal(xhr.headers["Content-Type"], undefined);
  assert.equal(xhr.data.get("kind"), "video");
  assert.equal(xhr.data.get("file").name, "video.mp4");
  xhr.upload.onprogress({ lengthComputable: true, loaded: 50, total: 100 });
  xhr.upload.onprogress({ lengthComputable: true, loaded: 50, total: 100 });
  assert.equal(updates.length, 1);
  xhr.upload.onload();
  await Promise.resolve();
  assert.equal(updates.at(-1).phase, "processing");
  assert.equal(complete, false);
  xhr.respond(201, JSON.stringify({ url: "/ready.mp4" }));
  assert.deepEqual(await result, { url: "/ready.mp4" });
}));

test("传输支持主动中止，已取消的请求不会继续发送", () => withXHR(async () => {
  const controller = new AbortController();
  const selected = new File(["video"], "video.mp4");
  const pending = uploadWithProgress(selected, "video", "token", { signal: controller.signal });
  controller.abort();
  assert.equal(FakeXHR.requests[0].aborted, true);
  await assert.rejects(pending, { name: "AbortError" });
  const stopped = uploadWithProgress(selected, "video", "token", { signal: controller.signal });
  await assert.rejects(stopped, { name: "AbortError" });
  assert.equal(FakeXHR.requests[1].data, undefined);
}));

test("传输异常、鉴权失败和无效响应不会被视为上传成功", () => withXHR(async () => {
  const selected = new File(["video"], "video.mp4");
  for (const [status, response, expected] of [
    [500, '{"error":"processing failed"}', /processing failed/],
    [401, "unauthorized", /登录已过期/],
    [201, "invalid json", /响应无效/]
  ]) {
    const pending = uploadWithProgress(selected, "video", "token");
    FakeXHR.requests.at(-1).respond(status, response);
    await assert.rejects(pending, expected);
  }
  const pending = uploadWithProgress(selected, "video", "token");
  FakeXHR.requests.at(-1).onerror();
  await assert.rejects(pending, /网络连接失败/);
}));
