package tts_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type fallbackTestProvider struct {
	name      string
	available bool
	synthFn   func(context.Context, string) ([]byte, error)
	called    int
	availCall int
}

func nilContextForFallbackTest() context.Context {
	var ctx context.Context
	return ctx
}

func (p *fallbackTestProvider) Name() string {
	return p.name
}

func (p *fallbackTestProvider) Synthesize(ctx context.Context, opts string) ([]byte, error) {
	p.called++
	if p.synthFn == nil {
		return nil, errors.New("synthesize not implemented")
	}
	return p.synthFn(ctx, opts)
}

func (p *fallbackTestProvider) SynthesizeStream(context.Context, string) (tts.AudioStream, error) {
	return nil, io.EOF
}

func (p *fallbackTestProvider) ListVoices(context.Context, string) ([]tts.Voice, error) {
	return nil, nil
}

func (p *fallbackTestProvider) IsAvailable(context.Context) bool {
	p.availCall++
	return p.available
}

func TestProviderFallbackChain_PrimaryFailSecondarySuccess(t *testing.T) {
	errPrimary := errors.New("primary unavailable")
	primary := &fallbackTestProvider{
		name:      "primary",
		available: true,
		synthFn: func(context.Context, string) ([]byte, error) {
			return nil, errPrimary
		},
	}
	secondary := &fallbackTestProvider{
		name:      "secondary",
		available: true,
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("secondary audio"), nil
		},
	}

	audio, err := tts.SynthesizeWithFallback(context.Background(), "hello", primary, secondary)
	if err != nil {
		t.Fatalf("SynthesizeWithFallback() error = %v", err)
	}
	if string(audio) != "secondary audio" {
		t.Fatalf("SynthesizeWithFallback() got audio %q, want %q", string(audio), "secondary audio")
	}
	if primary.called != 1 {
		t.Fatalf("primary called %d times, want 1", primary.called)
	}
	if secondary.called != 1 {
		t.Fatalf("secondary called %d times, want 1", secondary.called)
	}
}

func TestProviderFallbackChain_AllProvidersFail_ReturnsError(t *testing.T) {
	errPrimary := errors.New("primary failed")
	errSecondary := errors.New("secondary failed")
	primary := &fallbackTestProvider{
		name:      "primary",
		available: true,
		synthFn: func(context.Context, string) ([]byte, error) {
			return nil, errPrimary
		},
	}
	secondary := &fallbackTestProvider{
		name:      "secondary",
		available: true,
		synthFn: func(context.Context, string) ([]byte, error) {
			return nil, errSecondary
		},
	}

	_, err := tts.SynthesizeWithFallback(context.Background(), "hello", primary, secondary)
	if err == nil {
		t.Fatal("SynthesizeWithFallback() error = nil, want aggregated error")
	}

	var fallbackErr *tts.FallbackError
	if !errors.As(err, &fallbackErr) {
		t.Fatalf("expected *tts.FallbackError, got %T (%v)", err, err)
	}
	if len(fallbackErr.Failures) != 2 {
		t.Fatalf("fallback failures = %d, want 2", len(fallbackErr.Failures))
	}
	if fallbackErr.Failures[0].Provider != "primary" {
		t.Fatalf("first failure provider = %q, want %q", fallbackErr.Failures[0].Provider, "primary")
	}
	if fallbackErr.Failures[1].Provider != "secondary" {
		t.Fatalf("second failure provider = %q, want %q", fallbackErr.Failures[1].Provider, "secondary")
	}
	if !errors.Is(err, errPrimary) {
		t.Fatalf("expected aggregated error to include primary error: %v", err)
	}
	if !errors.Is(err, errSecondary) {
		t.Fatalf("expected aggregated error to include secondary error: %v", err)
	}
}

func TestProviderFallbackChain_DoesNotPreflightAvailability(t *testing.T) {
	primary := &fallbackTestProvider{
		name:      "primary",
		available: false,
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("primary audio"), nil
		},
	}
	secondary := &fallbackTestProvider{
		name:      "secondary",
		available: true,
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("secondary audio"), nil
		},
	}

	audio, err := tts.SynthesizeWithFallback(context.Background(), "hello", primary, secondary)
	if err != nil {
		t.Fatalf("SynthesizeWithFallback() error = %v", err)
	}
	if string(audio) != "primary audio" {
		t.Fatalf("audio = %q, want %q", string(audio), "primary audio")
	}
	if primary.availCall != 0 {
		t.Fatalf("primary IsAvailable called %d times, want 0", primary.availCall)
	}
	if primary.called != 1 {
		t.Fatalf("primary Synthesize called %d times, want 1", primary.called)
	}
	if secondary.availCall != 0 {
		t.Fatalf("secondary IsAvailable called %d times, want 0", secondary.availCall)
	}
	if secondary.called != 0 {
		t.Fatalf("secondary Synthesize called %d times, want 0", secondary.called)
	}
}

func TestProviderFallbackChain_ContextCanceledBeforeFirstProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	primary := &fallbackTestProvider{
		name: "primary",
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("unexpected"), nil
		},
	}
	secondary := &fallbackTestProvider{
		name: "secondary",
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("unexpected"), nil
		},
	}

	_, err := tts.SynthesizeWithFallback(ctx, "hello", primary, secondary)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SynthesizeWithFallback() error = %v, want %v", err, context.Canceled)
	}

	var fallbackErr *tts.FallbackError
	if errors.As(err, &fallbackErr) {
		t.Fatalf("expected direct context error, got fallback error: %v", err)
	}
	if primary.called != 0 {
		t.Fatalf("primary called %d times, want 0", primary.called)
	}
	if secondary.called != 0 {
		t.Fatalf("secondary called %d times, want 0", secondary.called)
	}
}

func TestProviderFallbackChain_NilContextUsesBackground(t *testing.T) {
	primary := &fallbackTestProvider{
		name: "primary",
		synthFn: func(ctx context.Context, _ string) ([]byte, error) {
			if ctx == nil {
				t.Fatal("SynthesizeWithFallback() passed nil context to provider")
			}
			return []byte("primary audio"), nil
		},
	}

	audio, err := tts.SynthesizeWithFallback[string](nilContextForFallbackTest(), "hello", primary)
	if err != nil {
		t.Fatalf("SynthesizeWithFallback() error = %v", err)
	}
	if string(audio) != "primary audio" {
		t.Fatalf("audio = %q, want %q", string(audio), "primary audio")
	}
	if primary.called != 1 {
		t.Fatalf("primary called %d times, want 1", primary.called)
	}
}

func TestProviderFallbackChain_ContextCanceledBetweenProviders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	primary := &fallbackTestProvider{
		name: "primary",
		synthFn: func(context.Context, string) ([]byte, error) {
			cancel()
			return nil, errors.New("primary failed")
		},
	}
	secondary := &fallbackTestProvider{
		name: "secondary",
		synthFn: func(context.Context, string) ([]byte, error) {
			return []byte("unexpected"), nil
		},
	}

	_, err := tts.SynthesizeWithFallback(ctx, "hello", primary, secondary)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SynthesizeWithFallback() error = %v, want %v", err, context.Canceled)
	}

	var fallbackErr *tts.FallbackError
	if errors.As(err, &fallbackErr) {
		t.Fatalf("expected direct context error, got fallback error: %v", err)
	}
	if primary.called != 1 {
		t.Fatalf("primary called %d times, want 1", primary.called)
	}
	if secondary.called != 0 {
		t.Fatalf("secondary called %d times, want 0", secondary.called)
	}
}
