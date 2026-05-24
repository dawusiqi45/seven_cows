package llm

import (
	"context"
	"strings"
)

type Input struct {
	Text string
	Mode string
}

type Result struct {
	Text     string
	Provider string
	Model    string
	Skipped  bool
}

type Optimizer interface {
	Optimize(ctx context.Context, input Input) (Result, error)
	Provider() string
}

type NoopOptimizer struct {
	provider string
}

func NewNoopOptimizer(provider string) *NoopOptimizer {
	if strings.TrimSpace(provider) == "" {
		provider = "none"
	}
	return &NoopOptimizer{provider: provider}
}

func (o *NoopOptimizer) Optimize(ctx context.Context, input Input) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	default:
	}
	return Result{
		Text:     strings.TrimSpace(input.Text),
		Provider: o.provider,
		Skipped:  true,
	}, nil
}

func (o *NoopOptimizer) Provider() string {
	return o.provider
}
