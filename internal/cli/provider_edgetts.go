package cli

import (
	"context"
	"fmt"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	edgeTTSProviderName = "edgetts"
	edgeTTSDefaultVoice = "zh-CN-XiaoxiaoNeural"
)

func init() {
	RegisterProvider(edgeTTSProviderName, newEdgeTTSAdapter)
}

// edgeTTSAdapter adapts edgetts.Provider to ProviderAdapter
type edgeTTSAdapter struct {
	provider *edgetts.Provider
}

func newEdgeTTSAdapter(cfg *ProviderConfig) (ProviderAdapter, error) {
	p := edgetts.New()

	if cfg != nil {
		if cfg.Proxy != "" {
			if err := validateProxyURL(cfg.Proxy); err != nil {
				return nil, fmt.Errorf("edgetts proxy: %w", err)
			}
			p.WithProxy(cfg.Proxy)
		}
		if cfg.HTTPTimeout > 0 {
			p.WithHTTPTimeout(cfg.HTTPTimeout)
			p.WithConnectTimeout(cfg.HTTPTimeout)
			p.WithReceiveTimeout(cfg.HTTPTimeout)
		}
		if cfg.MaxAttempts > 0 {
			p.WithMaxAttempts(cfg.MaxAttempts)
		}
	}

	return &edgeTTSAdapter{provider: p}, nil
}

func (a *edgeTTSAdapter) Name() string {
	return edgeTTSProviderName
}

func (a *edgeTTSAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return a.provider.ListVoices(ctx, locale)
}

func (a *edgeTTSAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	edgeOpts := &edgetts.SynthesizeOptions{
		Text:   opts.Text,
		Voice:  opts.Voice,
		Rate:   opts.Rate,
		Volume: opts.Volume,
		Pitch:  opts.Pitch,
	}
	return a.provider.Synthesize(ctx, edgeOpts)
}

func (a *edgeTTSAdapter) DefaultVoice() string {
	return edgeTTSDefaultVoice
}

func (a *edgeTTSAdapter) DefaultFormat() string {
	return "mp3"
}

func (a *edgeTTSAdapter) SupportsRateVolumePitch() bool {
	return true
}
