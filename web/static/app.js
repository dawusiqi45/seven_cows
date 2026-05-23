const state = {
  config: null,
  stream: null,
  audioContext: null,
  sourceNode: null,
  processorNode: null,
  sampleRate: 0,
  audioBuffers: [],
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
  if (state.recording) {
    return;
  }

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

  recordButton.classList.add("recording");
  recordButton.textContent = "松开识别";
  setStatus("正在录音");
}

async function stopRecording() {
  if (!state.recording) {
    return;
  }

  state.recording = false;
  recordButton.classList.remove("recording");
  recordButton.textContent = "按住录音";
  meterBar.style.width = "0";
  setStatus("正在识别");

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
  if (state.audioContext) {
    await state.audioContext.close();
  }

  const samples = mergeBuffers(state.audioBuffers);
  const wavBlob = encodeWav(samples, state.sampleRate, 16000);
  await submitRecording(wavBlob);
}

async function submitRecording(blob) {
  try {
    const result = await api("/api/recognize", {
      method: "POST",
      headers: { "Content-Type": "audio/wav" },
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
  meterBar.style.width = `${Math.min(100, Math.round(rms * 400))}%`;
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
