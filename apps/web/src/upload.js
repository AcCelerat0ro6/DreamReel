const MiB = 1024 * 1024;

export function validateUploadFile(file, kind) {
  const video = kind === "video";
  const label = video ? "视频" : "封面";
  const allowed = video ? /\.(mp4|mov|webm)$/i : /\.(jpg|jpeg|png|webp)$/i;
  if (!file || file.size <= 0) throw new Error(`请选择非空的${label}文件`);
  if (!allowed.test(file.name)) throw new Error(video ? "视频仅支持 MP4、MOV、WebM 格式" : "封面仅支持 JPG、PNG、WebP 格式");
  if (file.size > (video ? 512 : 20) * MiB) throw new Error(`${label}不能超过 ${video ? 512 : 20} MB`);
}

// 传输进度与服务端处理分开显示，100% 不代表视频已经可以发布。
export function uploadWithProgress(file, kind, token, { signal, onProgress = () => {} } = {}) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    let settled = false;
    let lastPercent = -1;
    const abortError = () => new DOMException("上传已取消", "AbortError");
    const abort = () => { xhr.abort(); finish(abortError()); };
    function finish(error, value) {
      if (settled) return;
      settled = true;
      signal?.removeEventListener("abort", abort);
      if (error) reject(error);
      else resolve(value);
    }
    if (signal?.aborted) { finish(abortError()); return; }
    xhr.open("POST", "/api/uploads");
    xhr.setRequestHeader("Accept", "application/json");
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    xhr.upload.onprogress = (event) => {
      if (settled || !event.lengthComputable) return;
      const percent = Math.min(100, Math.floor(event.loaded / event.total * 100));
      if (percent === lastPercent) return;
      lastPercent = percent;
      onProgress({ phase: percent === 100 ? "processing" : "uploading", percent });
    };
    xhr.upload.onload = () => {
      if (!settled) onProgress({ phase: "processing", percent: 100 });
    };
    xhr.onload = () => {
      let payload;
      try { payload = JSON.parse(xhr.responseText); } catch {}
      if (xhr.status < 200 || xhr.status >= 300) {
        const message = xhr.status === 401 ? "登录已过期，请重新登录" : payload?.message || payload?.error || "上传失败，请重试";
        finish(Object.assign(new Error(message), { status: xhr.status }));
      } else if (!payload?.url) {
        finish(new Error("上传响应无效，请重试"));
      } else {
        finish(null, payload);
      }
    };
    xhr.onerror = () => finish(new Error("网络连接失败，请重试上传"));
    xhr.ontimeout = () => finish(new Error("上传超时，请重试"));
    xhr.onabort = () => finish(abortError());
    signal?.addEventListener("abort", abort, { once: true });
    const data = new FormData();
    data.append("file", file);
    data.append("kind", kind);
    try { xhr.send(data); } catch (error) { finish(error); }
  });
}

// 选择文件后提前上传；发布和重试共用任务，成功文件无需再次传输。
export function createUploadManager({ upload, onChange }) {
  const tasks = new Map();

  function select(kind, file) {
    const previous = tasks.get(kind);
    tasks.delete(kind);
    previous?.controller.abort();
    if (!file) { onChange(kind, null); return; }
    const task = { controller: new AbortController(), state: { file, phase: "uploading", percent: 0, error: "" } };
    tasks.set(kind, task);
    function update(patch) {
      if (tasks.get(kind) !== task) return;
      task.state = { ...task.state, ...patch };
      onChange(kind, task.state);
    }
    update({});
    try {
      validateUploadFile(file, kind);
      task.promise = Promise.resolve(upload(file, kind, {
        signal: task.controller.signal,
        onProgress: update
      }));
    } catch (error) {
      task.promise = Promise.reject(error);
    }
    task.promise = task.promise.then((value) => {
      update({ phase: "ready", percent: 100, error: "", url: value.url });
      return value;
    }, (error) => {
      update({ phase: "error", error: error.message || "上传失败，请重试" });
      throw error;
    });
    // 预上传可能早于发布按钮，失败先展示在文件旁，避免未处理的 Promise。
    task.promise.catch(() => {});
    return task.promise;
  }

  function ensure(kind) {
    const task = tasks.get(kind);
    if (!task) return Promise.reject(new Error(kind === "video" ? "请选择视频文件" : "请选择封面文件"));
    return task.state.phase === "error" ? select(kind, task.state.file) : task.promise;
  }

  return {
    select,
    ensure,
    cancelAll() {
      const previous = [...tasks.values()];
      tasks.clear();
      for (const task of previous) task.controller.abort();
    }
  };
}
