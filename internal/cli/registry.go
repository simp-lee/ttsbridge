// Package cli provides the CLI application for ttsbridge.
package cli

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

// ProviderConfig contains common provider configuration
type ProviderConfig struct {
	Proxy       string
	HTTPTimeout time.Duration
	MaxAttempts int
}

// ProviderFactory creates a provider with the given configuration.
type ProviderFactory func(cfg *ProviderConfig) (tts.Provider, error)

type ProviderRegistration struct {
	Factory      ProviderFactory
	DefaultVoice string
}

// registry stores registered provider factories
var registry = make(map[string]ProviderRegistration)

// RegisterProvider registers a provider factory with the given name
func RegisterProvider(name string, registration ProviderRegistration) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("provider %q already registered", name))
	}
	if registration.Factory == nil {
		panic(fmt.Sprintf("provider %q registration must include a factory", name))
	}
	registry[name] = registration
}

// GetProvider returns a provider for the given name, or nil if not found.
func GetProvider(name string, cfg *ProviderConfig) (tts.Provider, error) {
	registration, ok := registry[name]
	if !ok {
		return nil, nil
	}
	return registration.Factory(cfg)
}

func GetProviderDefaultVoice(name string) string {
	registration, ok := registry[name]
	if !ok {
		return ""
	}
	return registration.DefaultVoice
}

func validateProxyURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return newProviderConfigValidationError(fmt.Sprintf("invalid proxy URL %q", rawURL), err)
	}
	if parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return newProviderConfigValidationError(
			fmt.Sprintf("invalid proxy URL %q", rawURL),
			errors.New("proxy URL must include scheme and host"),
		)
	}

	return nil
}

// ListProviders returns the names of all registered providers (sorted)
func ListProviders() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
