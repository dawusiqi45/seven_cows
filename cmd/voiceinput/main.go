package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dawusiqi45/seven_cows/internal/asr"
	"github.com/dawusiqi45/seven_cows/internal/config"
	"github.com/dawusiqi45/seven_cows/internal/history"
	"github.com/dawusiqi45/seven_cows/internal/llm"
	"github.com/dawusiqi45/seven_cows/internal/logging"
	"github.com/dawusiqi45/seven_cows/internal/secrets"
	"github.com/dawusiqi45/seven_cows/internal/server"
	"github.com/dawusiqi45/seven_cows/internal/textproc"
	"go.uber.org/zap"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dataDir := flag.String("data-dir", "data", "runtime data directory")
	staticDir := flag.String("static-dir", "web/static", "web static assets directory")
	secretsFile := flag.String("secrets-file", "voiceinput.local.json", "local ASR config file")
	logFile := flag.String("log-file", "", "log file path, defaults to data/logs/voiceinput.log")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create data directory: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*logFile) == "" {
		*logFile = filepath.Join(*dataDir, "logs", "voiceinput.log")
	}
	logger, err := logging.New(*logFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()
	logger.Info("logger initialized", zap.String("log_file", *logFile))

	configStore := config.NewFileStore(*dataDir + string(os.PathSeparator) + "config.json")
	appConfig, err := configStore.Load()
	if err != nil {
		logger.Error("load config failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("config loaded", zap.String("data_dir", *dataDir), zap.String("asr_provider", appConfig.ASR.Provider))
	localSecrets, err := secrets.Load(*secretsFile)
	if err != nil {
		logger.Error("load local secrets failed", zap.String("path", *secretsFile), zap.Error(err))
		os.Exit(1)
	}
	logger.Info("local secrets checked",
		zap.String("path", *secretsFile),
		zap.Bool("tencent_configured", hasTencentCredentials(localSecrets)),
		zap.Bool("glm_configured", hasGLMCredentials(localSecrets)),
	)

	historyStore := history.NewJSONStore(*dataDir + string(os.PathSeparator) + "history.json")
	processor := textproc.NewProcessor(appConfig.Text)
	recognizer, provider, err := buildRecognizer(appConfig.ASR.Provider, localSecrets, logger)
	if err != nil {
		logger.Error("create recognizer failed", zap.Error(err))
		os.Exit(1)
	}
	appConfig.ASR.Provider = provider
	optimizer, optimizerProvider, err := buildOptimizer(localSecrets, logger)
	if err != nil {
		logger.Error("create optimizer failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("text optimizer selected", zap.String("provider", optimizerProvider), zap.String("mode", appConfig.LLM.Mode))

	app := server.New(server.Dependencies{
		Config:      appConfig,
		ConfigStore: configStore,
		History:     historyStore,
		Processor:   processor,
		Recognizer:  recognizer,
		Optimizer:   optimizer,
		StaticDir:   *staticDir,
		Logger:      logger,
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("voice input server started", zap.String("url", "http://"+*addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", zap.Error(err))
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func buildRecognizer(configProvider string, localSecrets secrets.Config, logger *zap.Logger) (asr.Recognizer, string, error) {
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
		if !hasTencentCredentials(localSecrets) {
			logger.Warn("tencent asr selected but credentials are not configured, falling back to mock")
			return asr.NewMockRecognizer(), "mock", nil
		}
		recognizer, err := asr.NewTencentRecognizer(asr.TencentConfig{
			SecretID:       secrets.First(os.Getenv("TENCENTCLOUD_SECRET_ID"), localSecrets.TencentCloud.SecretID),
			SecretKey:      secrets.First(os.Getenv("TENCENTCLOUD_SECRET_KEY"), localSecrets.TencentCloud.SecretKey),
			Region:         firstDefault("ap-shanghai", os.Getenv("TENCENTCLOUD_REGION"), localSecrets.TencentCloud.Region),
			Engine:         firstDefault("16k_zh", os.Getenv("TENCENT_ASR_ENGINE"), localSecrets.TencentCloud.Engine),
			VoiceFormat:    "wav",
			FilterModal:    0,
			ConvertNumMode: 1,
			HotwordList:    secrets.First(os.Getenv("TENCENT_ASR_HOTWORDS"), localSecrets.TencentCloud.Hotwords),
		}, logger)
		if err != nil {
			return nil, "", err
		}
		logger.Info("using tencent asr provider", zap.String("endpoint", "asr.tencentcloudapi.com"))
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

func hasGLMCredentials(localSecrets secrets.Config) bool {
	return secrets.First(os.Getenv("GLM_API_KEY"), localSecrets.LLM.APIKey) != ""
}

func buildOptimizer(localSecrets secrets.Config, logger *zap.Logger) (llm.Optimizer, string, error) {
	provider := secrets.First(os.Getenv("VOICEINPUT_LLM_PROVIDER"), localSecrets.LLM.Provider)
	if provider == "" {
		provider = "glm"
	}

	switch strings.ToLower(provider) {
	case "glm":
		if !hasGLMCredentials(localSecrets) {
			logger.Warn("glm optimizer enabled but api key is not configured, falling back to no optimization")
			return llm.NewNoopOptimizer("missing_glm_key"), "missing_glm_key", nil
		}
		optimizer, err := llm.NewGLMOptimizer(llm.GLMConfig{
			APIKey:         secrets.First(os.Getenv("GLM_API_KEY"), localSecrets.LLM.APIKey),
			Model:          firstDefault("glm-5", os.Getenv("GLM_MODEL"), localSecrets.LLM.Model),
			BaseURL:        firstDefault("https://open.bigmodel.cn/api/paas/v4/chat/completions", os.Getenv("GLM_BASE_URL"), localSecrets.LLM.BaseURL),
			TimeoutSeconds: firstPositive(localSecrets.LLM.TimeoutSeconds, 60),
		}, logger)
		if err != nil {
			return nil, "", err
		}
		return optimizer, "glm", nil
	case "mock", "none", "disabled":
		return llm.NewNoopOptimizer(provider), provider, nil
	default:
		return nil, "", fmt.Errorf("unsupported llm provider: %s", provider)
	}
}

func firstDefault(fallback string, values ...string) string {
	value := secrets.First(values...)
	if value == "" {
		return fallback
	}
	return value
}

func firstPositive(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
