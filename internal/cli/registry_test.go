package cli

import (
	"context"
	"sort"
	"testing"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

type testAdapter struct {
	name string
}

func (a *testAdapter) Name() string { return a.name }

func (a *testAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return []tts.Voice{{ID: "voice-1", Language: tts.Language(locale), Provider: a.name}}, nil
}

func (a *testAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	return []byte(opts.Text), nil
}

func (a *testAdapter) DefaultVoice() string { return "voice-1" }

func (a *testAdapter) DefaultFormat() string { return "mp3" }

func (a *testAdapter) SupportsRateVolumePitch() bool { return true }

func TestProviderRegistry_DefaultProvidersRegistered(t *testing.T) {
	providers := ListProviders()
	if len(providers) < 2 {
		t.Fatalf("ListProviders() returned %d providers, want at least 2", len(providers))
	}

	want := map[string]bool{"edgetts": true, "volcengine": true}
	for _, p := range providers {
		delete(want, p)
	}
	for missing := range want {
		t.Fatalf("default provider %q not registered", missing)
	}

	if !sort.StringsAreSorted(providers) {
		t.Fatalf("ListProviders() is not sorted: %v", providers)
	}
}

func TestProviderRegistry_RegisterAndGetProvider(t *testing.T) {
	providerName := "test-provider-registry"
	RegisterProvider(providerName, func(cfg *ProviderConfig) ProviderAdapter {
		_ = cfg
		return &testAdapter{name: providerName}
	})

	adapter := GetProvider(providerName, &ProviderConfig{})
	if adapter == nil {
		t.Fatalf("GetProvider(%q) returned nil", providerName)
	}
	if adapter.Name() != providerName {
		t.Fatalf("adapter.Name() = %q, want %q", adapter.Name(), providerName)
	}

	voices, err := adapter.ListVoices(context.Background(), "en-US")
	if err != nil {
		t.Fatalf("ListVoices() error: %v", err)
	}
	if len(voices) != 1 || voices[0].Provider != providerName {
		t.Fatalf("ListVoices() = %+v, want provider %q", voices, providerName)
	}

	audio, err := adapter.Synthesize(context.Background(), &SynthesizeRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}
	if string(audio) != "hello" {
		t.Fatalf("Synthesize() audio = %q, want %q", string(audio), "hello")
	}
}

func TestProviderRegistry_GetUnknownProvider(t *testing.T) {
	if got := GetProvider("provider-that-does-not-exist", nil); got != nil {
		t.Fatalf("GetProvider() = %#v, want nil for unknown provider", got)
	}
}

func TestProviderRegistry_GenericProvidersAndAdaptersRemainCompatible(t *testing.T) {
	var _ tts.Provider[*edgetts.SynthesizeOptions] = (*edgetts.Provider)(nil)
	var _ tts.Provider[*volcengine.SynthesizeOptions] = (*volcengine.Provider)(nil)

	tests := []struct {
		providerName      string
		wantAdapterName   string
		wantDefaultFormat string
		wantRVP           bool
	}{
		{providerName: "edgetts", wantAdapterName: "edgetts", wantDefaultFormat: "mp3", wantRVP: true},
		{providerName: "volcengine", wantAdapterName: "volcengine", wantDefaultFormat: "wav", wantRVP: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.providerName, func(t *testing.T) {
			adapter := GetProvider(tt.providerName, &ProviderConfig{})
			if adapter == nil {
				t.Fatalf("GetProvider(%q) returned nil", tt.providerName)
			}
			if adapter.Name() != tt.wantAdapterName {
				t.Fatalf("adapter.Name() = %q, want %q", adapter.Name(), tt.wantAdapterName)
			}
			if adapter.DefaultVoice() == "" {
				t.Fatalf("adapter.DefaultVoice() should not be empty for %q", tt.providerName)
			}
			if adapter.DefaultFormat() != tt.wantDefaultFormat {
				t.Fatalf("adapter.DefaultFormat() = %q, want %q", adapter.DefaultFormat(), tt.wantDefaultFormat)
			}
			if adapter.SupportsRateVolumePitch() != tt.wantRVP {
				t.Fatalf("adapter.SupportsRateVolumePitch() = %v, want %v", adapter.SupportsRateVolumePitch(), tt.wantRVP)
			}
		})
	}
}
