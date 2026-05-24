package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const defaultGLMBaseURL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"

type GLMConfig struct {
	APIKey         string
	Model          string
	BaseURL        string
	TimeoutSeconds int
}

type GLMOptimizer struct {
	config GLMConfig
	client *http.Client
	logger *zap.Logger
}

func NewGLMOptimizer(config GLMConfig, logger *zap.Logger) (*GLMOptimizer, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.APIKey == "" {
		return nil, fmt.Errorf("glm optimizer requires api key")
	}
	if strings.TrimSpace(config.Model) == "" {
		config.Model = "glm-5"
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultGLMBaseURL
	}
	if config.TimeoutSeconds < 30 {
		config.TimeoutSeconds = 30
	}
	return &GLMOptimizer{
		config: config,
		client: &http.Client{Timeout: time.Duration(config.TimeoutSeconds) * time.Second},
		logger: logger.Named("glm_optimizer"),
	}, nil
}

func (o *GLMOptimizer) Provider() string {
	return "glm"
}

func (o *GLMOptimizer) Optimize(ctx context.Context, input Input) (Result, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return Result{Provider: o.Provider(), Model: o.config.Model, Skipped: true}, nil
	}
	if len([]rune(text)) < 5 {
		return Result{Text: text, Provider: o.Provider(), Model: o.config.Model, Skipped: true}, nil
	}

	payload := glmRequest{
		Model: o.config.Model,
		Messages: []glmMessage{
			{Role: "system", Content: systemPrompt(input.Mode)},
			{Role: "user", Content: text},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+o.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := o.client.Do(req)
	if err != nil {
		o.logger.Warn("glm optimize request failed", zap.Duration("duration", time.Since(started)), zap.Error(err))
		return Result{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		o.logger.Warn("glm optimize http error", zap.Int("status", resp.StatusCode), zap.Duration("duration", time.Since(started)))
		return Result{}, fmt.Errorf("glm http %d", resp.StatusCode)
	}

	var parsed glmResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		o.logger.Warn("glm optimize decode failed", zap.Duration("duration", time.Since(started)), zap.Error(err))
		return Result{}, err
	}
	if len(parsed.Choices) == 0 {
		return Result{}, fmt.Errorf("glm returned empty choices")
	}
	optimized := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if optimized == "" {
		return Result{}, fmt.Errorf("glm returned empty content")
	}
	o.logger.Info("glm optimize completed",
		zap.String("model", o.config.Model),
		zap.Duration("duration", time.Since(started)),
		zap.Int("input_chars", len([]rune(text))),
		zap.Int("output_chars", len([]rune(optimized))),
	)
	return Result{
		Text:     optimized,
		Provider: o.Provider(),
		Model:    o.config.Model,
	}, nil
}

func systemPrompt(mode string) string {
	switch strings.TrimSpace(mode) {
	case "formal":
		return "你是语音输入文本优化器。请在不改变核心事实、不编造新信息的前提下，把用户口述识别文本整理为正式、清晰、通顺的中文。可以删除语气词和重复表达，补全明显缺失的连接词、标点和断句，修正同音字、错别字和不自然表达。只输出优化后的文本，不要解释。"
	default:
		return "你是语音输入文本优化器。请把用户口述的语音识别文本优化成更清晰、连贯、自然的中文。要求：删除“嗯、啊、呃、然后”等无意义口头语和重复词；修正同音字、错别字、明显识别错误；补全标点、断句和必要连接词；在不改变原意、不编造事实的前提下，让表达更完整、更适合直接作为输入文本。只输出优化后的文本，不要解释。"
	}
}

type glmRequest struct {
	Model       string       `json:"model"`
	Messages    []glmMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
}

type glmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type glmResponse struct {
	Choices []struct {
		Message glmMessage `json:"message"`
	} `json:"choices"`
}
