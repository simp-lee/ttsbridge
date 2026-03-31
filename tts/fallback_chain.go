package tts

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errFallbackChainEmpty = errors.New("tts: fallback chain is empty")

// ProviderFailure records a failed attempt against a provider in a fallback chain.
type ProviderFailure struct {
	Provider string
	Err      error
}

// FallbackError aggregates all provider failures from a fallback chain.
type FallbackError struct {
	Failures []ProviderFailure
	joined   error
}

func newFallbackError(failures []ProviderFailure) *FallbackError {
	cloned := append([]ProviderFailure(nil), failures...)
	errList := make([]error, 0, len(cloned))
	for _, failure := range cloned {
		errList = append(errList, fmt.Errorf("%s: %w", failure.Provider, failure.Err))
	}
	return &FallbackError{Failures: cloned, joined: errors.Join(errList...)}
}

func (e *FallbackError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return "tts: all providers failed"
	}
	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		parts = append(parts, fmt.Sprintf("%s: %v", failure.Provider, failure.Err))
	}
	return "tts: all providers failed: " + strings.Join(parts, "; ")
}

func (e *FallbackError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.joined
}

// SynthesizeWithFallback tries providers in order and returns on first success.
// When all providers fail, it returns a FallbackError containing provider-level failures.
func SynthesizeWithFallback(ctx context.Context, request SynthesisRequest, providers ...Provider) (*SynthesisResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(providers) == 0 {
		return nil, errFallbackChainEmpty
	}

	failures := make([]ProviderFailure, 0, len(providers))
	for i, provider := range providers {
		providerName := fmt.Sprintf("provider[%d]", i)
		if provider == nil {
			failures = append(failures, ProviderFailure{Provider: providerName, Err: errors.New("provider is nil")})
			continue
		}
		if name := provider.Name(); name != "" {
			providerName = name
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result, err := provider.Synthesize(ctx, request)
		if err == nil {
			return result, nil
		}
		failures = append(failures, ProviderFailure{Provider: providerName, Err: err})
	}

	return nil, newFallbackError(failures)
}
