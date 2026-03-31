package cli

import (
	"fmt"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	volcengineProviderName = "volcengine"
	volcengineDefaultVoice = "BV700_streaming"
)

func init() {
	RegisterProvider(volcengineProviderName, ProviderRegistration{
		Factory:      newVolcengineProvider,
		DefaultVoice: volcengineDefaultVoice,
	})
}

func newVolcengineProvider(cfg *ProviderConfig) (tts.Provider, error) {
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

	return p, nil
}
