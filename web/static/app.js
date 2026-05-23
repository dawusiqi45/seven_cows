const state = {
  config: null,
  provider: "unknown",
  stream: null,
  audioContext: null,
  sourceNode: null,
  processorNode: null,
  sampleRate: 0,
  audioBuffers: [],
  recording: false,
  processing: false,
  recordingStartedAt: 0,
  timerHandle: null,
};

const statusEl = document.querySelector("#status");
const statusBadge = document.querySelector("#statusBadge");
const providerBadge = document.querySelector("#providerBadge");
const recordButton = document.querySelector("#recordButton");
const recordButtonText = document.querySelector("#recordButtonText");
const resultText = document.querySelector("#resultText");
const resultMeta = document.querySelector("#resultMeta");
const processButton = document.querySelector("#processButton");
const copyButton = document.querySelector("#copyButton");
const clearButton = document.querySelector("#clearButton");
const historyList = document.querySelector("#historyList");
const historyCount = document.querySelector("#historyCount");
const clearHistoryButton = document.querySelector("#clearHistoryButton");
const saveSettingsButton = document.querySelector("#saveSettingsButton");
const meterBar = document.querySelector("#meterBar");
const timerText = document.querySelector("#timerText");
const messageBar = document.querySelector("#messageBar");

const inputs = {
  autoPunctuation: document.querySelector("#autoPunctuation"),
  removeFillers: document.querySelector("#removeFillers"),
  enableCommands: document.querySelector("#enableCommands"),
};

function setStatus(text, mode = "ready") {
  statusEl.textContent = text;
  statusBadge.textContent = text;
  statusBadge.className = "badge badge-muted";
  if (mode === "recording") {
    statusBadge.classList.add("badge-recording");
  }
  if (mode === "error") {
    statusBadge.classList.add("badge-error");
  }
}

function showMessage(text, mode = "info") {
  messageBar.hidden = false;
  messageBar.textContent = text;
  messageBar.className = `message-bar ${mode === "error" ? "error" : ""}`;
}

function hideMessage() {
  messageBar.hidden = true;
  messageBar.textContent = "";
}

function setBusy(isBusy) {
  state.processing = isBusy;
  recordButton.disabled = isBusy;
  processButton.disabled = isBusy;
  copyButton.disabled = isBusy;
  clearButton.disabled = isBusy;
  saveSettingsButton.disabled = isBusy;
  clearHistoryButton.disabled = isBusy;
  recordButton.classList.toggle("processing", isBusy);
  if (isBusy) {
    recordButtonText.textContent = "识别中";
  } else if (!state.recording) {
    recordButtonText.textContent = "按住说话";
  }
}

async function api(path, options = {}) {
  const response = await fetch(path, options);
  if (!response.ok) {
    throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
  }
  return response.json();
}

async function loadHealth() {
  const health = await api("/api/health");
  state.provider = health.provider || "unknown";
  providerBadge.textContent = `识别服务：${state.provider}`;
}

async function loadConfig() {
  state.config = await api("/api/config");
  inputs.autoPunctuation.checked = state.config.text.autoPunctuation;
  inputs.removeFillers.checked = state.config.text.removeFillers;
  inputs.enableCommands.checked = state.config.text.enableCommands;
}

async function saveConfig() {
  try {
    state.config.text.autoPunctuation = inputs.autoPunctuation.checked;
    state.config.text.removeFillers = inputs.removeFillers.checked;
    state.config.text.enableCommands = inputs.enableCommands.checked;
    await api("/api/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(state.config),
    });
    setStatus("设置已保存");
    showMessage("文本处理设置已保存。");
  } catch (error) {
    setStatus("设置保存失败", "error");
    showMessage(`设置保存失败：${error.message}`, "error");
  }
}

async function loadHistory() {
  const entries = await api("/api/history");
  historyList.innerHTML = "";
  historyCount.textContent = `${entries.length} 条`;
  if (entries.length === 0) {
    historyList.innerHTML = '<div class="empty-state">暂无历史记录</div>';
    return;
  }
  for (const entry of entries) {
    const item = document.createElement("button");
    item.className = "history-item";
    item.type = "button";
    item.innerHTML = `
      <time>${new Date(entry.createdAt).toLocaleString()} · ${escapeHTML(entry.provider || "unknown")}</time>
      <div class="history-text">${escapeHTML(entry.finalText || "")}</div>
    `;
    item.addEventListener("click", () => {
      resultText.value = entry.finalText || "";
      updateResultMeta();
      showMessage("已从历史记录回填文本。");
    });
    historyList.appendChild(item);
  }
}

async function startRecording() {
  if (state.recording || state.processing) {
    return;
  }

  try {
    hideMessage();
    state.stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
      },
    });

    const AudioContext = window.AudioContext || window.webkitAudioContext;
    state.audioContext = new AudioContext();
    state.sampleRate = state.audioContext.sampleRate;
    state.audioBuffers = [];
    state.sourceNode = state.audioContext.createMediaStreamSource(state.stream);
    state.processorNode = state.audioContext.createScriptProcessor(4096, 1, 1);

    state.processorNode.onaudioprocess = (event) => {
      if (!state.recording) {
        return;
      }
      const input = event.inputBuffer.getChannelData(0);
      const chunk = new Float32Array(input);
      state.audioBuffers.push(chunk);
      updateMeter(chunk);
    };

    state.sourceNode.connect(state.processorNode);
    state.processorNode.connect(state.audioContext.destination);
    state.recording = true;
    state.recordingStartedAt = Date.now();
    state.timerHandle = window.setInterval(updateTimer, 250);
    updateTimer();

    recordButton.classList.add("recording");
    recordButtonText.textContent = "松开识别";
    setStatus("正在录音", "recording");
  } catch (error) {
    setStatus("无法录音", "error");
    showMessage(`无法访问麦克风：${error.message}`, "error");
    cleanupAudio();
  }
}

async function stopRecording() {
  if (!state.recording) {
    return;
  }

  state.recording = false;
  recordButton.classList.remove("recording");
  meterBar.style.width = "0";
  stopTimer();
  setStatus("正在识别");

  const samples = mergeBuffers(state.audioBuffers);
  await cleanupAudio();

  if (samples.length < state.sampleRate * 0.35) {
    setStatus("录音太短", "error");
    recordButtonText.textContent = "按住说话";
    showMessage("录音时间太短，请按住按钮说完整一句话。", "error");
    return;
  }

  const wavBlob = encodeWav(samples, state.sampleRate, 16000);
  await submitRecording(wavBlob);
}

async function submitRecording(blob) {
  setBusy(true);
  const startedAt = performance.now();
  try {
    const result = await api("/api/recognize", {
      method: "POST",
      headers: { "Content-Type": "audio/wav" },
      body: blob,
    });
    resultText.value = result.finalText || "";
    updateResultMeta(result);
    if (state.config.ui.autoCopy && result.finalText) {
      await navigator.clipboard.writeText(result.finalText);
    }
    const duration = Math.round(performance.now() - startedAt);
    setStatus("识别完成");
    showMessage(`识别完成，服务：${result.provider || state.provider}，耗时 ${duration}ms。`);
    await loadHistory();
  } catch (error) {
    setStatus("识别失败", "error");
    showMessage(`识别失败：${error.message}`, "error");
  } finally {
    setBusy(false);
    updateResultMeta();
  }
}

async function processCurrentText() {
  if (!resultText.value.trim()) {
    showMessage("当前没有可处理的文本。", "error");
    return;
  }
  try {
    const result = await api("/api/process", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: resultText.value }),
    });
    resultText.value = result.text;
    updateResultMeta();
    setStatus("文本已处理");
    showMessage("已按当前设置处理文本。");
  } catch (error) {
    setStatus("处理失败", "error");
    showMessage(`文本处理失败：${error.message}`, "error");
  }
}

async function copyCurrentText() {
  if (!resultText.value.trim()) {
    showMessage("当前没有可复制的文本。", "error");
    return;
  }
  await navigator.clipboard.writeText(resultText.value);
  setStatus("已复制");
  showMessage("文本已复制到剪贴板。");
}

async function clearHistory() {
  if (!window.confirm("确定清空全部历史记录吗？")) {
    return;
  }
  await api("/api/history", { method: "DELETE" });
  await loadHistory();
  showMessage("历史记录已清空。");
}

function clearResult() {
  resultText.value = "";
  updateResultMeta();
  hideMessage();
}

async function cleanupAudio() {
  if (state.processorNode) {
    state.processorNode.disconnect();
    state.processorNode.onaudioprocess = null;
  }
  if (state.sourceNode) {
    state.sourceNode.disconnect();
  }
  if (state.stream) {
    state.stream.getTracks().forEach((track) => track.stop());
  }
  if (state.audioContext && state.audioContext.state !== "closed") {
    await state.audioContext.close();
  }
  state.stream = null;
  state.audioContext = null;
  state.sourceNode = null;
  state.processorNode = null;
}

function updateTimer() {
  const elapsed = Math.max(0, Date.now() - state.recordingStartedAt);
  const seconds = Math.floor(elapsed / 1000);
  const minutesText = String(Math.floor(seconds / 60)).padStart(2, "0");
  const secondsText = String(seconds % 60).padStart(2, "0");
  timerText.textContent = `${minutesText}:${secondsText}`;
}

function stopTimer() {
  if (state.timerHandle) {
    window.clearInterval(state.timerHandle);
    state.timerHandle = null;
  }
  timerText.textContent = "00:00";
}

function updateResultMeta(result = null) {
  const text = resultText.value.trim();
  if (!text) {
    resultMeta.textContent = "暂无文本";
    return;
  }
  const chars = Array.from(text).length;
  const lines = text.split(/\n/).length;
  const provider = result?.provider ? ` · ${result.provider}` : "";
  resultMeta.textContent = `${chars} 字 · ${lines} 行${provider}`;
}

function mergeBuffers(buffers) {
  const totalLength = buffers.reduce((sum, buffer) => sum + buffer.length, 0);
  const merged = new Float32Array(totalLength);
  let offset = 0;
  for (const buffer of buffers) {
    merged.set(buffer, offset);
    offset += buffer.length;
  }
  return merged;
}

function downsampleBuffer(buffer, inputRate, outputRate) {
  if (outputRate === inputRate) {
    return buffer;
  }
  const ratio = inputRate / outputRate;
  const newLength = Math.round(buffer.length / ratio);
  const result = new Float32Array(newLength);
  let offsetResult = 0;
  let offsetBuffer = 0;

  while (offsetResult < result.length) {
    const nextOffsetBuffer = Math.round((offsetResult + 1) * ratio);
    let accumulator = 0;
    let count = 0;
    for (let i = offsetBuffer; i < nextOffsetBuffer && i < buffer.length; i += 1) {
      accumulator += buffer[i];
      count += 1;
    }
    result[offsetResult] = accumulator / Math.max(count, 1);
    offsetResult += 1;
    offsetBuffer = nextOffsetBuffer;
  }

  return result;
}

function encodeWav(floatSamples, inputRate, outputRate) {
  const samples = downsampleBuffer(floatSamples, inputRate, outputRate);
  const dataLength = samples.length * 2;
  const buffer = new ArrayBuffer(44 + dataLength);
  const view = new DataView(buffer);

  writeString(view, 0, "RIFF");
  view.setUint32(4, 36 + dataLength, true);
  writeString(view, 8, "WAVE");
  writeString(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, outputRate, true);
  view.setUint32(28, outputRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeString(view, 36, "data");
  view.setUint32(40, dataLength, true);

  let offset = 44;
  for (const sample of samples) {
    const clamped = Math.max(-1, Math.min(1, sample));
    view.setInt16(offset, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true);
    offset += 2;
  }

  return new Blob([view], { type: "audio/wav" });
}

function writeString(view, offset, value) {
  for (let i = 0; i < value.length; i += 1) {
    view.setUint8(offset + i, value.charCodeAt(i));
  }
}

function updateMeter(buffer) {
  let sum = 0;
  for (const sample of buffer) {
    sum += sample * sample;
  }
  const rms = Math.sqrt(sum / Math.max(buffer.length, 1));
  meterBar.style.width = `${Math.min(100, Math.round(rms * 460))}%`;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => {
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
recordButton.addEventListener("pointercancel", stopRecording);
recordButton.addEventListener("pointerleave", () => {
  if (state.recording) {
    stopRecording();
  }
});
processButton.addEventListener("click", processCurrentText);
copyButton.addEventListener("click", copyCurrentText);
clearButton.addEventListener("click", clearResult);
clearHistoryButton.addEventListener("click", clearHistory);
saveSettingsButton.addEventListener("click", saveConfig);
resultText.addEventListener("input", updateResultMeta);

Promise.all([loadHealth(), loadConfig(), loadHistory()])
  .then(() => {
    updateResultMeta();
    setStatus("准备就绪");
  })
  .catch((error) => {
    setStatus("初始化失败", "error");
    showMessage(`初始化失败：${error.message}`, "error");
  });
