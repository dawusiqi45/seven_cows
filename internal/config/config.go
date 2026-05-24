package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/dawusiqi45/seven_cows/internal/textproc"
)

type AppConfig struct {
	ASR  ASRConfig           `json:"asr"`
	Text textproc.TextConfig `json:"text"`
	LLM  LLMConfig           `json:"llm"`
	UI   UIConfig            `json:"ui"`
}

type ASRConfig struct {
	Provider string `json:"provider"`
}

type LLMConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

type UIConfig struct {
	AutoCopy bool `json:"autoCopy"`
	AutoSend bool `json:"autoSend"`
}

func Default() AppConfig {
	return AppConfig{
		ASR: ASRConfig{
			Provider: "mock",
		},
		Text: textproc.TextConfig{
			AutoPunctuation: true,
			RemoveFillers:   true,
			EnableCommands:  true,
			Hotwords:        []string{"七牛云", "Go语言", "语音输入法"},
		},
		LLM: LLMConfig{
			Enabled: false,
			Mode:    "conservative",
		},
		UI: UIConfig{
			AutoCopy: true,
			AutoSend: false,
		},
	}
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() (AppConfig, error) {
	cfg := Default()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, s.Save(cfg)
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (s *FileStore) Save(cfg AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
