package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dawusiqi45/seven_cows/internal/asr"
	"github.com/dawusiqi45/seven_cows/internal/config"
	"github.com/dawusiqi45/seven_cows/internal/history"
	"github.com/dawusiqi45/seven_cows/internal/textproc"
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
	StaticDir   string
	Logger      *slog.Logger
}

type App struct {
	config      config.AppConfig
	configStore ConfigStore
	history     HistoryStore
	processor   *textproc.Processor
	recognizer  asr.Recognizer
	staticDir   string
	logger      *slog.Logger
}

func New(deps Dependencies) *App {
	return &App{
		config:      deps.Config,
		configStore: deps.ConfigStore,
		history:     deps.History,
		processor:   deps.Processor,
		recognizer:  deps.Recognizer,
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
	return withJSONErrors(mux, a.logger)
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
			http.Error(w, "save config failed", http.StatusInternalServerError)
			return
		}
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
			http.Error(w, "clear history failed", http.StatusInternalServerError)
			return
		}
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
		http.Error(w, "read audio failed", http.StatusBadRequest)
		return
	}

	result, err := a.recognizer.Recognize(r.Context(), audio, r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	finalText := a.processor.Process(result.Text)
	entry := history.Entry{
		ID:         time.Now().Format("20060102150405.000000000"),
		RawText:    result.Text,
		FinalText:  finalText,
		Provider:   result.Provider,
		Confidence: result.Confidence,
		CreatedAt:  time.Now(),
	}
	if err := a.history.Add(entry); err != nil {
		a.logger.Warn("save history failed", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rawText":    result.Text,
		"finalText":  finalText,
		"provider":   result.Provider,
		"confidence": result.Confidence,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func withJSONErrors(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("request panic", "error", recovered)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
