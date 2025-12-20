package cli

import (
	"context"

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

func newVolcengineAdapter(cfg *ProviderConfig) ProviderAdapter {
	p := volcengine.New()

	if cfg != nil {
		if cfg.Proxy != "" {
			p.WithProxy(cfg.Proxy)
		}
		if cfg.HTTPTimeout > 0 {
			p.WithHTTPTimeout(cfg.HTTPTimeout)
		}
		if cfg.MaxAttempts > 0 {
			p.WithMaxAttempts(cfg.MaxAttempts)
		}
	}

	return &volcengineAdapter{provider: p}
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
