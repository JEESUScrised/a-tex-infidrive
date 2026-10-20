const filesBody = document.querySelector("#files");
const jobsBody = document.querySelector("#jobs");
const health = document.querySelector("#health");
const youtube = document.querySelector("#youtube");
const youtubeChannel = document.querySelector("#youtube-channel");
const youtubeActions = document.querySelector("#youtube-actions");
const youtubeError = document.querySelector("#youtube-error");
const uploadError = document.querySelector("#upload-error");
const recoverError = document.querySelector("#recover-error");

let youtubeState = { enabled: false, connected: false };

async function api(path, options) {
  const response = await fetch(path, options);
  const text = await response.text();
  let body = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = { error: text };
    }
  }
  if (!response.ok) {
    throw new Error((body && body.error) || response.statusText);
  }
  return body;
}

function sizeText(n) {
  if (n < 1024) return n + " Б";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " КБ";
  return (n / 1024 / 1024).toFixed(1) + " МБ";
}

function jobKindLabel(kind) {
  if (kind === "encode") return "кодирование";
  if (kind === "decode") return "восстановление";
  if (kind === "youtube_upload") return "YouTube";
  return kind;
}

function canCancel(status) {
  return status === "queued" || status === "running" || status === "uploading" ||
    status === "uploaded" || status === "processing";
}

async function refreshHealth() {
  try {
    const data = await api("/api/health");
    if (data.ffmpeg) {
      health.textContent = "Сервер работает, FFmpeg найден" +
        (data.ffmpeg_source ? " (" + data.ffmpeg_source + ")." : ".");
      health.className = "ok";
    } else {
      health.textContent = "Сервер работает, но FFmpeg не найден. Кодирование завершится ошибкой.";
      health.className = "error";
    }
    await refreshYouTube();
  } catch (err) {
    health.textContent = "Нет связи с сервером: " + err.message;
    health.className = "error";
  }
}

async function refreshYouTube() {
  youtubeError.textContent = "";
  const yt = await api("/api/youtube");
  youtubeState = yt;
  youtube.textContent = yt.message || "";
  youtubeChannel.textContent = "";
  youtubeActions.replaceChildren();
  if (yt.channel && yt.channel.title) {
    youtubeChannel.textContent = "Канал: " + yt.channel.title;
  }
  if (!yt.enabled) {
    return;
  }
  if (!yt.connected) {
    const connect = document.createElement("button");
    connect.type = "button";
    connect.textContent = "Подключить Google";
    connect.addEventListener("click", async () => {
      connect.disabled = true;
      try {
        const data = await api("/api/youtube/auth/start");
        window.location.href = data.auth_url;
      } catch (err) {
        youtubeError.textContent = err.message;
        connect.disabled = false;
      }
    });
    youtubeActions.append(connect);
    return;
  }
  const disconnect = document.createElement("button");
  disconnect.type = "button";
  disconnect.textContent = "Отключить аккаунт";
  disconnect.addEventListener("click", async () => {
    disconnect.disabled = true;
    try {
      await api("/api/youtube/disconnect", { method: "POST" });
      await refreshYouTube();
      await refreshFiles();
    } catch (err) {
      youtubeError.textContent = err.message;
      disconnect.disabled = false;
    }
  });
  youtubeActions.append(disconnect);
}

async function refreshFiles() {
  const files = await api("/api/files");
  filesBody.replaceChildren();
  for (const file of files) {
    const tr = document.createElement("tr");
    const name = document.createElement("td");
    name.textContent = file.original_name;
    const size = document.createElement("td");
    size.textContent = sizeText(file.size);
    const hash = document.createElement("td");
    hash.className = "hash";
    hash.textContent = file.sha256;
    const actions = document.createElement("td");
    const encode = document.createElement("button");
    encode.type = "button";
    encode.textContent = "В видео";
    encode.addEventListener("click", async () => {
      encode.disabled = true;
      uploadError.textContent = "";
      try {
        await api("/api/files/" + file.id + "/encode", { method: "POST" });
        await refreshJobs();
      } catch (err) {
        uploadError.textContent = err.message;
      } finally {
        encode.disabled = false;
      }
    });
    const download = document.createElement("a");
    download.href = "/api/files/" + file.id + "/download";
    download.textContent = "файл";
    actions.append(encode, document.createTextNode(" "), download);
    if (file.has_video) {
      const video = document.createElement("a");
      video.href = "/api/files/" + file.id + "/video";
      video.textContent = "mp4";
      actions.append(document.createTextNode(" "), video);
      if (youtubeState.enabled && youtubeState.connected && !file.youtube_video_id) {
        const upload = document.createElement("button");
        upload.type = "button";
        upload.textContent = "Загрузить на YouTube";
        upload.addEventListener("click", async () => {
          upload.disabled = true;
          uploadError.textContent = "";
          try {
            await api("/api/files/" + file.id + "/youtube", { method: "POST" });
            await refreshJobs();
          } catch (err) {
            uploadError.textContent = err.message;
          } finally {
            upload.disabled = false;
          }
        });
        actions.append(document.createTextNode(" "), upload);
      }
      if (file.youtube_url) {
        const link = document.createElement("a");
        link.href = file.youtube_url;
        link.target = "_blank";
        link.rel = "noopener";
        link.textContent = "YouTube";
        actions.append(document.createTextNode(" "), link);
        if (file.youtube_processing_status || file.youtube_upload_status) {
          const st = document.createElement("span");
          st.textContent = " [" + (file.youtube_processing_status || file.youtube_upload_status) +
            (file.youtube_privacy_status ? ", " + file.youtube_privacy_status : "") + "]";
          actions.append(st);
        }
      }
      if (file.youtube_error) {
        const err = document.createElement("span");
        err.className = "error";
        err.textContent = " " + file.youtube_error;
        actions.append(err);
      }
    }
    tr.append(name, size, hash, actions);
    filesBody.append(tr);
  }
}

function progressCell(job) {
  const wrap = document.createElement("div");
  wrap.textContent = job.progress + "%";
  const bar = document.createElement("div");
  bar.className = "progress";
  const span = document.createElement("span");
  span.style.width = Math.max(0, Math.min(100, job.progress)) + "%";
  bar.append(span);
  wrap.append(bar);
  return wrap;
}

async function refreshJobs() {
  const jobs = await api("/api/jobs");
  jobsBody.replaceChildren();
  for (const job of jobs) {
    const tr = document.createElement("tr");
    const kind = document.createElement("td");
    kind.textContent = jobKindLabel(job.kind);
    const status = document.createElement("td");
    status.textContent = job.status;
    const progress = document.createElement("td");
    progress.append(progressCell(job));
    const message = document.createElement("td");
    message.textContent = job.error || job.message || "";
    if (job.error) message.className = "error";
    const actions = document.createElement("td");
    if (canCancel(job.status)) {
      const cancel = document.createElement("button");
      cancel.type = "button";
      cancel.textContent = "отмена";
      cancel.addEventListener("click", async () => {
        cancel.disabled = true;
        try {
          await api("/api/jobs/" + job.id + "/cancel", { method: "POST" });
          await refreshJobs();
        } catch (err) {
          uploadError.textContent = err.message;
          cancel.disabled = false;
        }
      });
      actions.append(cancel);
    }
    if (job.status === "succeeded") {
      const link = document.createElement("a");
      link.href = "/api/jobs/" + job.id + "/download";
      link.textContent = job.kind === "encode" ? "скачать mp4" : "скачать файл";
      actions.append(link);
    }
    tr.append(kind, status, progress, message, actions);
    jobsBody.append(tr);
  }
}

document.querySelector("#upload-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  uploadError.textContent = "";
  const input = document.querySelector("#upload-file");
  if (!input.files.length) return;
  const form = new FormData();
  form.append("file", input.files[0]);
  try {
    await api("/api/files", { method: "POST", body: form });
    input.value = "";
    await refreshFiles();
  } catch (err) {
    uploadError.textContent = err.message;
  }
});

document.querySelector("#recover-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  recoverError.textContent = "";
  const input = document.querySelector("#recover-file");
  if (!input.files.length) return;
  const form = new FormData();
  form.append("file", input.files[0]);
  try {
    await api("/api/recover", { method: "POST", body: form });
    input.value = "";
    await refreshJobs();
  } catch (err) {
    recoverError.textContent = err.message;
  }
});

async function tick() {
  try {
    await refreshFiles();
    await refreshJobs();
  } catch (err) {
    uploadError.textContent = err.message;
  }
}

const params = new URLSearchParams(window.location.search);
if (params.get("youtube_error")) {
  youtubeError.textContent = params.get("youtube_error");
}

refreshHealth();
tick();
setInterval(tick, 2000);
