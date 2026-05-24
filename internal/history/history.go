package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	ID                string    `json:"id"`
	RawText           string    `json:"rawText"`
	ProcessedText     string    `json:"processedText,omitempty"`
	OptimizedText     string    `json:"optimizedText,omitempty"`
	FinalText         string    `json:"finalText"`
	Provider          string    `json:"provider"`
	OptimizerProvider string    `json:"optimizerProvider,omitempty"`
	Confidence        float64   `json:"confidence"`
	CreatedAt         time.Time `json:"createdAt"`
}

type JSONStore struct {
	mu   sync.Mutex
	path string
}

func NewJSONStore(path string) *JSONStore {
	return &JSONStore{path: path}
}

func (s *JSONStore) Add(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	entries = append([]Entry{entry}, entries...)
	if len(entries) > 100 {
		entries = entries[:100]
	}
	return s.saveLocked(entries)
}

func (s *JSONStore) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *JSONStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked([]Entry{})
}

func (s *JSONStore) loadLocked() ([]Entry, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []Entry{}, nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *JSONStore) saveLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
