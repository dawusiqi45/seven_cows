package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dawusiqi45/seven_cows/internal/asr"
	"github.com/dawusiqi45/seven_cows/internal/config"
	"github.com/dawusiqi45/seven_cows/internal/history"
	"github.com/dawusiqi45/seven_cows/internal/server"
	"github.com/dawusiqi45/seven_cows/internal/textproc"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dataDir := flag.String("data-dir", "data", "runtime data directory")
	staticDir := flag.String("static-dir", "web/static", "web static assets directory")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}

	configStore := config.NewFileStore(*dataDir + string(os.PathSeparator) + "config.json")
	appConfig, err := configStore.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	historyStore := history.NewJSONStore(*dataDir + string(os.PathSeparator) + "history.json")
	processor := textproc.NewProcessor(appConfig.Text)
	recognizer, provider, err := buildRecognizer(appConfig.ASR.Provider, logger)
	if err != nil {
		logger.Error("create recognizer", "error", err)
		os.Exit(1)
	}
	appConfig.ASR.Provider = provider

	app := server.New(server.Dependencies{
		Config:      appConfig,
		ConfigStore: configStore,
		History:     historyStore,
		Processor:   processor,
		Recognizer:  recognizer,
		StaticDir:   *staticDir,
		Logger:      logger,
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("voice input server started", "url", "http://"+*addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func buildRecognizer(configProvider string, logger *slog.Logger) (asr.Recognizer, string, error) {
	provider := strings.TrimSpace(os.Getenv("VOICEINPUT_ASR_PROVIDER"))
	if provider == "" {
		provider = strings.TrimSpace(configProvider)
	}
	if provider == "" || strings.EqualFold(provider, "mock") {
		if hasTencentCredentials() {
			provider = "tencent"
		} else {
			logger.Info("using mock asr provider")
			return asr.NewMockRecognizer(), "mock", nil
		}
	}

	switch strings.ToLower(provider) {
	case "tencent":
		recognizer, err := asr.NewTencentRecognizer(asr.TencentConfig{
			SecretID:       os.Getenv("TENCENTCLOUD_SECRET_ID"),
			SecretKey:      os.Getenv("TENCENTCLOUD_SECRET_KEY"),
			Region:         getenvDefault("TENCENTCLOUD_REGION", "ap-shanghai"),
			Engine:         getenvDefault("TENCENT_ASR_ENGINE", "16k_zh"),
			VoiceFormat:    "wav",
			FilterModal:    0,
			ConvertNumMode: 1,
			HotwordList:    os.Getenv("TENCENT_ASR_HOTWORDS"),
		})
		if err != nil {
			return nil, "", err
		}
		logger.Info("using tencent asr provider", "endpoint", "asr.tencentcloudapi.com")
		return recognizer, "tencent", nil
	case "mock":
		logger.Info("using mock asr provider")
		return asr.NewMockRecognizer(), "mock", nil
	default:
		return nil, "", fmt.Errorf("unsupported asr provider: %s", provider)
	}
}

func hasTencentCredentials() bool {
	return strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_ID")) != "" &&
		strings.TrimSpace(os.Getenv("TENCENTCLOUD_SECRET_KEY")) != ""
}

func getenvDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
