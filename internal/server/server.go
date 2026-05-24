package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dawusiqi45/seven_cows/internal/asr"
	"github.com/dawusiqi45/seven_cows/internal/config"
	"github.com/dawusiqi45/seven_cows/internal/history"
	"github.com/dawusiqi45/seven_cows/internal/llm"
	"github.com/dawusiqi45/seven_cows/internal/textproc"
	"go.uber.org/zap"
)

type ConfigStore interface {
	Save(config.AppConfig) error
}

type HistoryStore interface {
	Add(history.Entry) error
	List() ([]history.Entry, error)
	Clear() error
}

type Dependencies struct {
	Config      config.AppConfig
	ConfigStore ConfigStore
	History     HistoryStore
	Processor   *textproc.Processor
	Recognizer  asr.Recognizer
	Optimizer   llm.Optimizer
	StaticDir   string
	Logger      *zap.Logger
}

type App struct {
	config      config.AppConfig
	configStore ConfigStore
	history     HistoryStore
	processor   *textproc.Processor
	recognizer  asr.Recognizer
	optimizer   llm.Optimizer
	staticDir   string
	logger      *zap.Logger
}

func New(deps Dependencies) *App {
	return &App{
		config:      deps.Config,
		configStore: deps.ConfigStore,
		history:     deps.History,
		processor:   deps.Processor,
		recognizer:  deps.Recognizer,
		optimizer:   deps.Optimizer,
		staticDir:   deps.StaticDir,
		logger:      deps.Logger,
	}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(a.staticDir)))
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/history", a.handleHistory)
	mux.HandleFunc("/api/process", a.handleProcess)
	mux.HandleFunc("/api/recognize", a.handleRecognize)
	return withRequestLogging(withJSONErrors(mux, a.logger), a.logger)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"timestamp": time.Now(),
		"provider":  a.config.ASR.Provider,
		"llm": map[string]any{
			"enabled":  a.config.LLM.Enabled,
			"provider": a.optimizer.Provider(),
			"mode":     a.config.LLM.Mode,
		},
	})
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.config)
	case http.MethodPost:
		var next config.AppConfig
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			http.Error(w, "invalid config json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(next.ASR.Provider) == "" {
			next.ASR.Provider = "mock"
		}
		a.config = next
		a.processor.UpdateConfig(next.Text)
		if err := a.configStore.Save(next); err != nil {
			a.logger.Error("save config failed", zap.Error(err))
			http.Error(w, "save config failed", http.StatusInternalServerError)
			return
		}
		a.logger.Info("config updated",
			zap.String("asr_provider", next.ASR.Provider),
			zap.Bool("auto_punctuation", next.Text.AutoPunctuation),
			zap.Bool("remove_fillers", next.Text.RemoveFillers),
			zap.Bool("enable_commands", next.Text.EnableCommands),
			zap.Bool("llm_enabled", next.LLM.Enabled),
			zap.String("llm_mode", next.LLM.Mode),
		)
		writeJSON(w, http.StatusOK, next)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries, err := a.history.List()
		if err != nil {
			http.Error(w, "load history failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	case http.MethodDelete:
		if err := a.history.Clear(); err != nil {
			a.logger.Error("clear history failed", zap.Error(err))
			http.Error(w, "clear history failed", http.StatusInternalServerError)
			return
		}
		a.logger.Info("history cleared")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"text": a.processor.Process(req.Text),
	})
}

func (a *App) handleRecognize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	audio, err := readAllLimited(r, 10<<20)
	if err != nil {
		a.logger.Warn("read audio failed", zap.Error(err), zap.String("content_type", r.Header.Get("Content-Type")))
		http.Error(w, "read audio failed", http.StatusBadRequest)
		return
	}

	started := time.Now()
	result, err := a.recognizer.Recognize(r.Context(), audio, r.Header.Get("Content-Type"))
	if err != nil {
		a.logger.Warn("recognition failed",
			zap.Duration("duration", time.Since(started)),
			zap.Int("audio_bytes", len(audio)),
			zap.String("content_type", r.Header.Get("Content-Type")),
			zap.Error(err),
		)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.logger.Info("recognition completed",
		zap.String("provider", result.Provider),
		zap.String("request_id", result.RequestID),
		zap.Duration("duration", time.Since(started)),
		zap.Int("audio_bytes", len(audio)),
		zap.String("content_type", r.Header.Get("Content-Type")),
	)

	processedText := a.processor.Process(result.Text)
	finalText := processedText
	optimizerProvider := ""
	if a.config.LLM.Enabled {
		optimizeStarted := time.Now()
		optimized, optimizeErr := a.optimizer.Optimize(r.Context(), llm.Input{
			Text: processedText,
			Mode: a.config.LLM.Mode,
		})
		if optimizeErr != nil {
			a.logger.Warn("text optimization failed",
				zap.Duration("duration", time.Since(optimizeStarted)),
				zap.String("optimizer", a.optimizer.Provider()),
				zap.Error(optimizeErr),
			)
		} else if !optimized.Skipped && strings.TrimSpace(optimized.Text) != "" {
			finalText = optimized.Text
			optimizerProvider = optimized.Provider
			a.logger.Info("text optimization completed",
				zap.String("optimizer", optimized.Provider),
				zap.String("model", optimized.Model),
				zap.Duration("duration", time.Since(optimizeStarted)),
			)
		}
	}
	entry := history.Entry{
		ID:                time.Now().Format("20060102150405.000000000"),
		RawText:           result.Text,
		ProcessedText:     processedText,
		FinalText:         finalText,
		Provider:          result.Provider,
		OptimizerProvider: optimizerProvider,
		Confidence:        result.Confidence,
		CreatedAt:         time.Now(),
	}
	if optimizerProvider != "" {
		entry.OptimizedText = finalText
	}
	if err := a.history.Add(entry); err != nil {
		a.logger.Warn("save history failed", zap.Error(err))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rawText":       result.Text,
		"processedText": processedText,
		"finalText":     finalText,
		"optimized":     optimizerProvider != "",
		"optimizer":     optimizerProvider,
		"provider":      result.Provider,
		"confidence":    result.Confidence,
		"requestId":     result.RequestID,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func withJSONErrors(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panic", zap.Any("error", recovered), zap.String("method", r.Method), zap.String("path", r.URL.Path))
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withRequestLogging(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("http request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", recorder.statusCode),
			zap.Int("bytes", recorder.bytesWritten),
			zap.Duration("duration", time.Since(started)),
			zap.String("remote_addr", r.RemoteAddr),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	written, err := r.ResponseWriter.Write(data)
	r.bytesWritten += written
	return written, err
}
