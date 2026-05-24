package asr

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	tencentAction   = "SentenceRecognition"
	tencentEndpoint = "https://asr.tencentcloudapi.com"
	tencentHost     = "asr.tencentcloudapi.com"
	tencentService  = "asr"
	tencentVersion  = "2019-06-14"
)

type TencentConfig struct {
	SecretID       string
	SecretKey      string
	Region         string
	Engine         string
	VoiceFormat    string
	FilterModal    int
	ConvertNumMode int
	HotwordList    string
}

type TencentRecognizer struct {
	config TencentConfig
	client *http.Client
	logger *zap.Logger
}

func NewTencentRecognizer(config TencentConfig, logger *zap.Logger) (*TencentRecognizer, error) {
	config.SecretID = strings.TrimSpace(config.SecretID)
	config.SecretKey = strings.TrimSpace(config.SecretKey)
	if config.SecretID == "" || config.SecretKey == "" {
		return nil, fmt.Errorf("tencent asr requires both secret id and secret key")
	}
	if config.Region == "" {
		config.Region = "ap-shanghai"
	}
	if config.Engine == "" {
		config.Engine = "16k_zh"
	}
	if config.VoiceFormat == "" {
		config.VoiceFormat = "wav"
	}
	if config.ConvertNumMode == 0 {
		config.ConvertNumMode = 1
	}

	return &TencentRecognizer{
		config: config,
		client: &http.Client{Timeout: 20 * time.Second},
		logger: logger.Named("tencent_asr"),
	}, nil
}

func (r *TencentRecognizer) Recognize(ctx context.Context, audio []byte, contentType string) (Result, error) {
	if len(audio) == 0 {
		return Result{}, fmt.Errorf("audio payload is empty")
	}

	payload := map[string]any{
		"SubServiceType": 2,
		"ProjectId":      0,
		"EngSerViceType": r.config.Engine,
		"SourceType":     1,
		"VoiceFormat":    r.config.VoiceFormat,
		"Data":           base64.StdEncoding.EncodeToString(audio),
		"DataLen":        len(audio),
		"FilterModal":    r.config.FilterModal,
		"ConvertNumMode": r.config.ConvertNumMode,
		"FilterPunc":     0,
		"WordInfo":       0,
	}
	if strings.TrimSpace(r.config.HotwordList) != "" {
		payload["HotwordList"] = r.config.HotwordList
	}
	r.logger.Info("tencent asr request prepared",
		zap.Int("audio_bytes", len(audio)),
		zap.String("content_type", contentType),
		zap.String("region", r.config.Region),
		zap.String("engine", r.config.Engine),
		zap.String("voice_format", r.config.VoiceFormat),
		zap.Bool("has_hotwords", strings.TrimSpace(r.config.HotwordList) != ""),
	)

	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tencentEndpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	r.sign(req, body, time.Now().UTC())

	started := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Warn("tencent asr request failed", zap.Duration("duration", time.Since(started)), zap.Error(err))
		return Result{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.logger.Warn("tencent asr http error", zap.Int("status", resp.StatusCode), zap.Duration("duration", time.Since(started)))
		return Result{}, fmt.Errorf("tencent asr http %d", resp.StatusCode)
	}

	var parsed tencentResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		r.logger.Warn("tencent asr decode failed", zap.Duration("duration", time.Since(started)), zap.Error(err))
		return Result{}, err
	}
	if parsed.Response.Error != nil {
		r.logger.Warn("tencent asr api error",
			zap.String("code", parsed.Response.Error.Code),
			zap.String("request_id", parsed.Response.RequestID),
			zap.Duration("duration", time.Since(started)),
		)
		return Result{}, fmt.Errorf("tencent asr %s: %s", parsed.Response.Error.Code, parsed.Response.Error.Message)
	}
	if strings.TrimSpace(parsed.Response.Result) == "" {
		r.logger.Warn("tencent asr returned empty result",
			zap.String("request_id", parsed.Response.RequestID),
			zap.Int("audio_duration_ms", parsed.Response.AudioDuration),
			zap.Duration("duration", time.Since(started)),
		)
		return Result{}, fmt.Errorf("tencent asr returned empty result, request id: %s", parsed.Response.RequestID)
	}
	r.logger.Info("tencent asr request completed",
		zap.String("request_id", parsed.Response.RequestID),
		zap.Int("audio_duration_ms", parsed.Response.AudioDuration),
		zap.Duration("duration", time.Since(started)),
	)

	return Result{
		Text:       parsed.Response.Result,
		Provider:   "tencent",
		Confidence: 0,
		RequestID:  parsed.Response.RequestID,
	}, nil
}

func (r *TencentRecognizer) sign(req *http.Request, payload []byte, now time.Time) {
	contentType := "application/json; charset=utf-8"
	timestamp := fmt.Sprintf("%d", now.Unix())
	date := now.Format("2006-01-02")

	hashedPayload := sha256Hex(payload)
	canonicalHeaders := "content-type:" + contentType + "\n" + "host:" + tencentHost + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	credentialScope := date + "/" + tencentService + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		timestamp,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+r.config.SecretKey), date)
	secretService := hmacSHA256(secretDate, tencentService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		r.config.SecretID,
		credentialScope,
		signedHeaders,
		signature,
	)

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", tencentHost)
	req.Header.Set("X-TC-Action", tencentAction)
	req.Header.Set("X-TC-Version", tencentVersion)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Region", r.config.Region)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

type tencentResponse struct {
	Response struct {
		Result        string        `json:"Result"`
		AudioDuration int           `json:"AudioDuration"`
		RequestID     string        `json:"RequestId"`
		Error         *tencentError `json:"Error"`
	} `json:"Response"`
}

type tencentError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}
