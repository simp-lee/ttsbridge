package cli

import (
	"context"
	"fmt"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	volcengineProviderName = "volcengine"
	volcengineDefaultVoice = "BV700_streaming"
)

func init() {
	RegisterProvider(volcengineProviderName, newVolcengineAdapter)
}

// volcengineAdapter adapts volcengine.Provider to ProviderAdapter
type volcengineAdapter struct {
	provider *volcengine.Provider
}

func newVolcengineAdapter(cfg *ProviderConfig) (ProviderAdapter, error) {
	p := volcengine.New()

	if cfg != nil {
		if cfg.Proxy != "" {
			if err := validateProxyURL(cfg.Proxy); err != nil {
				return nil, fmt.Errorf("volcengine proxy: %w", err)
			}
			p.WithProxy(cfg.Proxy)
		}
		if cfg.HTTPTimeout > 0 {
			p.WithHTTPTimeout(cfg.HTTPTimeout)
		}
		if cfg.MaxAttempts > 0 {
			p.WithMaxAttempts(cfg.MaxAttempts)
		}
	}

	return &volcengineAdapter{provider: p}, nil
}

func (a *volcengineAdapter) Name() string {
	return volcengineProviderName
}

func (a *volcengineAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return a.provider.ListVoices(ctx, locale)
}

func (a *volcengineAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	volcOpts := &volcengine.SynthesizeOptions{
		Text:  opts.Text,
		Voice: opts.Voice,
	}
	return a.provider.Synthesize(ctx, volcOpts)
}

func (a *volcengineAdapter) DefaultVoice() string {
	return volcengineDefaultVoice
}

func (a *volcengineAdapter) DefaultFormat() string {
	return "wav"
}

func (a *volcengineAdapter) SupportsRateVolumePitch() bool {
	return false
}
