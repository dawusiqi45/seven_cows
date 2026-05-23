# seven_cows

暑期训练营项目：语音输入法助手。

语音输入法助手。项目目标是在三天内完成一个可运行、可演示、可扩展的 Go 语言语音输入法 MVP。

## 功能

- 浏览器录音入口。
- Go 后端统一 ASR 接口。
- Mock 语音识别实现，便于无第三方服务时演示完整链路。
- 文本处理：清理语气词、口令转换、自动补句号。
- 历史记录：本地保存最近 100 条识别结果。
- 设置项：自动标点、清理语气词、启用口令。

## 运行

```bash
go run ./cmd/voiceinput
```

打开：

```text
http://127.0.0.1:8080
```

## 腾讯云 ASR 配置

默认使用 `mock` 识别器，便于无密钥时演示完整流程。接入腾讯云一句话识别时，通过环境变量启用：

```powershell
$env:VOICEINPUT_ASR_PROVIDER="tencent"
$env:TENCENTCLOUD_SECRET_ID="your-secret-id"
$env:TENCENTCLOUD_SECRET_KEY="your-secret-key"
$env:TENCENTCLOUD_REGION="ap-shanghai"
$env:TENCENT_ASR_ENGINE="16k_zh"
go run ./cmd/voiceinput
```

密钥不要写入代码、README 或提交记录。本项目只提交 `.env.example`，真实 `.env` 已在 `.gitignore` 中排除。

前端会把浏览器麦克风音频编码成 16kHz 单声道 WAV 后提交给 Go 后端，后端再调用腾讯云 ASR。

## 测试

```bash
go test ./...
```

## 项目结构

```text
cmd/voiceinput       应用入口
internal/asr         语音识别接口与 mock 实现
internal/config      配置读写
internal/history     历史记录存储
internal/server      HTTP API 与静态页面服务
internal/textproc    文本处理
web/static           前端页面
docs                 需求与计划文档
```

## 第三方依赖

当前版本仅使用 Go 标准库，没有引入第三方库或框架。腾讯云 ASR 通过 HTTP API 和 TC3-HMAC-SHA256 签名直接调用，未引入腾讯云 SDK。

## 原创功能部分

- Go 后端服务与 API 路由。
- ASR 抽象接口、mock 识别器、腾讯云 ASR 适配器。
- 文本清洗、口令转换、自动标点处理。
- 本地 JSON 配置与历史记录。
- Web 录音控制台和浏览器端 WAV 编码。

## 后续扩展

- 接入真实 ASR 服务，例如云端 HTTP API 或本地 Whisper/Vosk。
- 增加全局快捷键和托盘程序。
- 增加 Windows SendInput 上屏能力。
- 长期版本可研究 Windows TSF，实现真正系统级输入法。

## PR 拆分建议

1. `feat: initialize go voice input project scaffold`
2. `feat: add text processing pipeline`
3. `feat: add web recorder and recognition api`
4. `feat: add local history and settings`
5. `docs: add requirements and delivery plan`
