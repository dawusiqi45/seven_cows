package secrets

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type Config struct {
	ASRProvider  string       `json:"asrProvider"`
	TencentCloud TencentCloud `json:"tencentCloud"`
	LLM          LLM          `json:"llm"`
}

type TencentCloud struct {
	SecretID  string `json:"secretId"`
	SecretKey string `json:"secretKey"`
	Region    string `json:"region"`
	Engine    string `json:"engine"`
	Hotwords  string `json:"hotwords"`
}

type LLM struct {
	Provider       string `json:"provider"`
	APIKey         string `json:"apiKey"`
	Model          string `json:"model"`
	BaseURL        string `json:"baseUrl"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func Load(path string) (Config, error) {
	var cfg Config
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func First(values ...string) string {
	for _, value := range values {
		cleaned := strings.TrimSpace(value)
		if cleaned != "" && cleaned != "****" {
			return cleaned
		}
	}
	return ""
}
