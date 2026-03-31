package cli

import (
	"fmt"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	edgeTTSProviderName = "edgetts"
	edgeTTSDefaultVoice = "zh-CN-XiaoxiaoNeural"
)

func init() {
	RegisterProvider(edgeTTSProviderName, ProviderRegistration{
		Factory:      newEdgeTTSProvider,
		DefaultVoice: edgeTTSDefaultVoice,
	})
}

func newEdgeTTSProvider(cfg *ProviderConfig) (tts.Provider, error) {
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

	return p, nil
}
