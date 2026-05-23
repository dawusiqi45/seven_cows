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
	"github.com/dawusiqi45/seven_cows/internal/secrets"
	"github.com/dawusiqi45/seven_cows/internal/server"
	"github.com/dawusiqi45/seven_cows/internal/textproc"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dataDir := flag.String("data-dir", "data", "runtime data directory")
	staticDir := flag.String("static-dir", "web/static", "web static assets directory")
	secretsFile := flag.String("secrets-file", "../voiceinput.local.json", "local secrets config file, outside git repository by default")
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
	localSecrets, err := secrets.Load(*secretsFile)
	if err != nil {
		logger.Error("load local secrets", "path", *secretsFile, "error", err)
		os.Exit(1)
	}

	historyStore := history.NewJSONStore(*dataDir + string(os.PathSeparator) + "history.json")
	processor := textproc.NewProcessor(appConfig.Text)
	recognizer, provider, err := buildRecognizer(appConfig.ASR.Provider, localSecrets, logger)
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

func buildRecognizer(configProvider string, localSecrets secrets.Config, logger *slog.Logger) (asr.Recognizer, string, error) {
	provider := secrets.First(
		os.Getenv("VOICEINPUT_ASR_PROVIDER"),
		localSecrets.ASRProvider,
		configProvider,
	)
	if provider == "" || strings.EqualFold(provider, "mock") {
		if hasTencentCredentials(localSecrets) {
			provider = "tencent"
		} else {
			logger.Info("using mock asr provider")
			return asr.NewMockRecognizer(), "mock", nil
		}
	}

	switch strings.ToLower(provider) {
	case "tencent":
		recognizer, err := asr.NewTencentRecognizer(asr.TencentConfig{
			SecretID:       secrets.First(os.Getenv("TENCENTCLOUD_SECRET_ID"), localSecrets.TencentCloud.SecretID),
			SecretKey:      secrets.First(os.Getenv("TENCENTCLOUD_SECRET_KEY"), localSecrets.TencentCloud.SecretKey),
			Region:         firstDefault("ap-shanghai", os.Getenv("TENCENTCLOUD_REGION"), localSecrets.TencentCloud.Region),
			Engine:         firstDefault("16k_zh", os.Getenv("TENCENT_ASR_ENGINE"), localSecrets.TencentCloud.Engine),
			VoiceFormat:    "wav",
			FilterModal:    0,
			ConvertNumMode: 1,
			HotwordList:    secrets.First(os.Getenv("TENCENT_ASR_HOTWORDS"), localSecrets.TencentCloud.Hotwords),
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

func hasTencentCredentials(localSecrets secrets.Config) bool {
	return secrets.First(os.Getenv("TENCENTCLOUD_SECRET_ID"), localSecrets.TencentCloud.SecretID) != "" &&
		secrets.First(os.Getenv("TENCENTCLOUD_SECRET_KEY"), localSecrets.TencentCloud.SecretKey) != ""
}

func firstDefault(fallback string, values ...string) string {
	value := secrets.First(values...)
	if value == "" {
		return fallback
	}
	return value
}
