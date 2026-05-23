const state = {
  config: null,
  mediaRecorder: null,
  audioChunks: [],
  recording: false,
};

const statusEl = document.querySelector("#status");
const recordButton = document.querySelector("#recordButton");
const resultText = document.querySelector("#resultText");
const processButton = document.querySelector("#processButton");
const copyButton = document.querySelector("#copyButton");
const clearButton = document.querySelector("#clearButton");
const historyList = document.querySelector("#historyList");
const clearHistoryButton = document.querySelector("#clearHistoryButton");
const saveSettingsButton = document.querySelector("#saveSettingsButton");
const meterBar = document.querySelector("#meterBar");

const inputs = {
  autoPunctuation: document.querySelector("#autoPunctuation"),
  removeFillers: document.querySelector("#removeFillers"),
  enableCommands: document.querySelector("#enableCommands"),
};

function setStatus(text) {
  statusEl.textContent = text;
}

async function api(path, options = {}) {
  const response = await fetch(path, options);
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

async function loadConfig() {
  state.config = await api("/api/config");
  inputs.autoPunctuation.checked = state.config.text.autoPunctuation;
  inputs.removeFillers.checked = state.config.text.removeFillers;
  inputs.enableCommands.checked = state.config.text.enableCommands;
}

async function saveConfig() {
  state.config.text.autoPunctuation = inputs.autoPunctuation.checked;
  state.config.text.removeFillers = inputs.removeFillers.checked;
  state.config.text.enableCommands = inputs.enableCommands.checked;
  await api("/api/config", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(state.config),
  });
  setStatus("设置已保存");
}

async function loadHistory() {
  const entries = await api("/api/history");
  historyList.innerHTML = "";
  if (entries.length === 0) {
    historyList.innerHTML = '<div class="history-item">暂无历史记录</div>';
    return;
  }
  for (const entry of entries) {
    const item = document.createElement("button");
    item.className = "history-item";
    item.type = "button";
    item.innerHTML = `<time>${new Date(entry.createdAt).toLocaleString()}</time><div>${escapeHTML(entry.finalText)}</div>`;
    item.addEventListener("click", () => {
      resultText.value = entry.finalText;
    });
    historyList.appendChild(item);
  }
}

async function startRecording() {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  state.audioChunks = [];
  state.mediaRecorder = new MediaRecorder(stream);
  state.mediaRecorder.addEventListener("dataavailable", (event) => {
    if (event.data.size > 0) {
      state.audioChunks.push(event.data);
    }
  });
  state.mediaRecorder.addEventListener("stop", submitRecording);
  state.mediaRecorder.start();
  state.recording = true;
  recordButton.classList.add("recording");
  recordButton.textContent = "松开识别";
  setStatus("正在录音");
  animateMeter();
}

function stopRecording() {
  if (!state.mediaRecorder || state.mediaRecorder.state === "inactive") {
    return;
  }
  state.mediaRecorder.stop();
  state.mediaRecorder.stream.getTracks().forEach((track) => track.stop());
  state.recording = false;
  recordButton.classList.remove("recording");
  recordButton.textContent = "按住录音";
  meterBar.style.width = "0";
  setStatus("正在识别");
}

async function submitRecording() {
  try {
    const blob = new Blob(state.audioChunks, { type: "audio/webm" });
    const result = await api("/api/recognize", {
      method: "POST",
      headers: { "Content-Type": blob.type },
      body: blob,
    });
    resultText.value = result.finalText;
    if (state.config.ui.autoCopy) {
      await navigator.clipboard.writeText(result.finalText);
    }
    setStatus(`识别完成：${result.provider}`);
    await loadHistory();
  } catch (error) {
    setStatus(`识别失败：${error.message}`);
  }
}

async function processCurrentText() {
  const result = await api("/api/process", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text: resultText.value }),
  });
  resultText.value = result.text;
}

async function copyCurrentText() {
  await navigator.clipboard.writeText(resultText.value);
  setStatus("已复制到剪贴板");
}

async function clearHistory() {
  await api("/api/history", { method: "DELETE" });
  await loadHistory();
}

function animateMeter() {
  if (!state.recording) {
    return;
  }
  meterBar.style.width = `${20 + Math.round(Math.random() * 75)}%`;
  window.setTimeout(animateMeter, 180);
}

function escapeHTML(value) {
  return value.replace(/[&<>"']/g, (char) => {
    return {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#039;",
    }[char];
  });
}

recordButton.addEventListener("pointerdown", startRecording);
recordButton.addEventListener("pointerup", stopRecording);
recordButton.addEventListener("pointerleave", () => {
  if (state.recording) {
    stopRecording();
  }
});
processButton.addEventListener("click", processCurrentText);
copyButton.addEventListener("click", copyCurrentText);
clearButton.addEventListener("click", () => {
  resultText.value = "";
});
clearHistoryButton.addEventListener("click", clearHistory);
saveSettingsButton.addEventListener("click", saveConfig);

loadConfig()
  .then(loadHistory)
  .then(() => setStatus("准备就绪"))
  .catch((error) => setStatus(`初始化失败：${error.message}`));
