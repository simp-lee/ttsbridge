// Package cli provides the CLI application for ttsbridge.
package cli

import (
	"context"
	"sort"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

// ProviderAdapter adapts different TTS providers to a common interface for CLI use.
// This allows the CLI to work with any provider without knowing its specific implementation.
type ProviderAdapter interface {
	// Name returns the provider name
	Name() string

	// ListVoices returns available voices, optionally filtered by locale
	ListVoices(ctx context.Context, locale string) ([]tts.Voice, error)

	// Synthesize synthesizes text to speech with the given options
	Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error)

	// DefaultVoice returns the default voice ID for this provider
	DefaultVoice() string

	// DefaultFormat returns the default audio format (e.g., "mp3", "wav")
	DefaultFormat() string

	// SupportsRateVolumePitch returns true if the provider supports rate/volume/pitch adjustment
	SupportsRateVolumePitch() bool
}

// SynthesizeRequest is a provider-agnostic synthesis request
type SynthesizeRequest struct {
	Text   string
	Voice  string
	Rate   float64 // 0.5-2.0, default 1.0
	Volume float64 // 0.0-2.0, default 1.0
	Pitch  float64 // 0.5-2.0, default 1.0
}

// ProviderConfig contains common provider configuration
type ProviderConfig struct {
	Proxy       string
	HTTPTimeout time.Duration
	MaxAttempts int
}

// ProviderFactory creates a ProviderAdapter with the given configuration
type ProviderFactory func(cfg *ProviderConfig) ProviderAdapter

// registry stores registered provider factories
var registry = make(map[string]ProviderFactory)

// RegisterProvider registers a provider factory with the given name
func RegisterProvider(name string, factory ProviderFactory) {
	registry[name] = factory
}

// GetProvider returns a provider adapter for the given name, or nil if not found
func GetProvider(name string, cfg *ProviderConfig) ProviderAdapter {
	factory, ok := registry[name]
	if !ok {
		return nil
	}
	return factory(cfg)
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
