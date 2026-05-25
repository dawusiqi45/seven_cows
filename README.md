# seven_cows

暑期训练营项目：语音输入法助手。

本项目基于 Go 开发语音输入法助手，提供录音、语音识别、文本处理、历史记录和配置管理能力，用于提升文本输入效率。

## 功能

- 浏览器录音入口。
- Go 后端统一 ASR 接口。
- Mock 语音识别实现，便于无第三方服务时验证完整链路。
- 腾讯云一句话识别适配器。
- GLM 智能文本优化按钮，用于对识别后的文本进行纠错和整理。
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

运行日志默认写入：

```text
data/logs/voiceinput.log
```

也可以通过参数指定日志文件：

```bash
go run ./cmd/voiceinput -log-file data/logs/dev.log
```

## 腾讯云 ASR

默认使用 `mock` 识别器，便于无密钥时验证完整流程。接入腾讯云一句话识别时，直接修改项目根目录下的配置文件：

```text
D:\prosoft\package\goproject\LanguageInput\seven_cows\voiceinput.local.json
```

配置文件随项目提交到 GitHub，但敏感字段使用 `****` 脱敏：

```json
{
  "asrProvider": "tencent",
  "tencentCloud": {
    "secretId": "****",
    "secretKey": "****",
    "region": "ap-shanghai",
    "engine": "16k_zh",
    "hotwords": ""
  },
  "llm": {
    "provider": "glm",
    "apiKey": "****",
    "model": "glm-5",
    "baseUrl": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
    "timeoutSeconds": 60
  }
}
```

实际运行时只需要把 `secretId`、`secretKey` 和 `llm.apiKey` 的 `****` 改成真实密钥。提交代码前请确认这些字段已经恢复为 `****`，避免泄露密钥。需要切回 mock 模式时，把 `asrProvider` 改为 `mock` 即可。

前端会把浏览器麦克风音频编码成 16kHz 单声道 WAV 后提交给 Go 后端，后端再调用腾讯云 ASR。

## 智能优化

智能优化是语音识别后的手动文本后处理。流程为：

```text
录音 -> 腾讯云 ASR -> 展示识别文本 -> 点击智能优化 -> GLM 文本优化 -> 展示优化结果
```

GLM 只用于修正错别字、同音字、标点、断句和口语化表达，不负责语音识别本身。录音识别完成后，页面会先展示腾讯云 ASR 和本地规则处理后的文本；用户点击“智能优化”按钮后，后端才会把当前文本提交给 GLM 优化。GLM 请求会关闭思考过程并限制输出长度，减少等待时间。如果未配置 `llm.apiKey`，则只执行本地规则处理；如果 GLM 调用超时或失败，页面会提示错误并保留当前文本。

## 测试

```bash
go test ./...
```

## 演示说明

演示步骤见 [docs/demo-guide.md](docs/demo-guide.md)。

演示视频：[Bilibili Demo](https://www.bilibili.com/video/BV1wiG462EMh/)。

## 项目结构

```text
cmd/voiceinput       应用入口
internal/asr         语音识别接口与 mock 实现
internal/config      配置读写
internal/history     历史记录存储
internal/llm         GLM 文本优化接口
internal/logging     zap 日志初始化
internal/server      HTTP API 与静态页面服务
internal/textproc    文本处理
web/static           前端页面
docs                 需求与计划文档
```

## 第三方依赖

- `go.uber.org/zap`：结构化日志，记录服务启动、HTTP 请求、识别调用、错误信息和耗时。

腾讯云 ASR 通过 HTTP API 和 TC3-HMAC-SHA256 签名直接调用，未引入腾讯云 SDK。GLM 文本优化通过 HTTP API 调用。

本项目演示阶段使用腾讯云 ASR 和智谱 GLM 服务的免费额度完成接口调用，不涉及额外付费部署。

## 原创功能部分

- Go 后端服务与 API 路由。
- ASR 抽象接口、mock 识别器、腾讯云 ASR 适配器。
- GLM 智能文本优化器。
- 文本清洗、口令转换、自动标点处理。
- 本地 JSON 配置与历史记录。
- 基于 zap 的结构化日志记录。
- Web 录音控制台和浏览器端 WAV 编码。
