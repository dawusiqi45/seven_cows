package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	recognizer := asr.NewMockRecognizer()

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
