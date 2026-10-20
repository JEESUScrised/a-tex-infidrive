const filesBody = document.querySelector("#files");
const jobsBody = document.querySelector("#jobs");
const health = document.querySelector("#health");
const youtube = document.querySelector("#youtube");
const uploadError = document.querySelector("#upload-error");
const recoverError = document.querySelector("#recover-error");

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
    const yt = await api("/api/youtube");
    youtube.textContent = yt.message;
  } catch (err) {
    health.textContent = "Нет связи с сервером: " + err.message;
    health.className = "error";
  }
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
    kind.textContent = job.kind === "encode" ? "кодирование" : "восстановление";
    const status = document.createElement("td");
    status.textContent = job.status;
    const progress = document.createElement("td");
    progress.append(progressCell(job));
    const message = document.createElement("td");
    message.textContent = job.error || job.message || "";
    if (job.error) message.className = "error";
    const actions = document.createElement("td");
    if (job.status === "queued" || job.status === "running") {
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

refreshHealth();
tick();
setInterval(tick, 2000);
