package asr

import (
	"context"
	"fmt"
)

type Result struct {
	Text       string  `json:"text"`
	Provider   string  `json:"provider"`
	Confidence float64 `json:"confidence"`
}

type Recognizer interface {
	Recognize(ctx context.Context, audio []byte, contentType string) (Result, error)
}

type MockRecognizer struct{}

func NewMockRecognizer() *MockRecognizer {
	return &MockRecognizer{}
}

func (r *MockRecognizer) Recognize(ctx context.Context, audio []byte, contentType string) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}

	if len(audio) == 0 {
		return Result{}, fmt.Errorf("audio payload is empty")
	}

	return Result{
		Text:       "这是一次语音输入演示，换行当前版本已经完成了输入流程框架。",
		Provider:   "mock",
		Confidence: 0.80,
	}, nil
}
