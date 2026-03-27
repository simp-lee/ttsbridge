package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	RegisterProvider(providerName, func(cfg *ProviderConfig) (ProviderAdapter, error) {
		_ = cfg
		return &testAdapter{name: providerName}, nil
	})

	adapter, err := GetProvider(providerName, &ProviderConfig{})
	if err != nil {
		t.Fatalf("GetProvider(%q) error: %v", providerName, err)
	}
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
	got, err := GetProvider("provider-that-does-not-exist", nil)
	if err != nil {
		t.Fatalf("GetProvider() error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("GetProvider() = %#v, want nil for unknown provider", got)
	}
}

func TestProviderRegistry_RegisterProviderPanicsOnDuplicateName(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{})

	providerName := "duplicate-provider"
	RegisterProvider(providerName, func(cfg *ProviderConfig) (ProviderAdapter, error) {
		_ = cfg
		return &testAdapter{name: providerName}, nil
	})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("RegisterProvider() expected panic on duplicate provider name")
		}
		if !strings.Contains(fmt.Sprint(recovered), "already registered") {
			t.Fatalf("panic = %v, want duplicate registration message", recovered)
		}
	}()

	RegisterProvider(providerName, func(cfg *ProviderConfig) (ProviderAdapter, error) {
		_ = cfg
		return &testAdapter{name: providerName + "-other"}, nil
	})
}

func TestProviderRegistry_GetProviderReturnsFactoryError(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{
		"broken": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return nil, errors.New("invalid config")
		},
	})

	adapter, err := GetProvider("broken", &ProviderConfig{Proxy: "://bad"})
	if err == nil {
		t.Fatal("GetProvider() error = nil, want factory error")
	}
	if adapter != nil {
		t.Fatalf("GetProvider() adapter = %#v, want nil on factory error", adapter)
	}
}

func TestValidateProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "empty allowed", rawURL: "", wantErr: false},
		{name: "valid http", rawURL: "http://127.0.0.1:8080", wantErr: false},
		{name: "missing scheme and host", rawURL: "bad-proxy", wantErr: true},
		{name: "parse error", rawURL: "://bad-proxy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProxyURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateProxyURL(%q) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
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
			adapter, err := GetProvider(tt.providerName, &ProviderConfig{})
			if err != nil {
				t.Fatalf("GetProvider(%q) error: %v", tt.providerName, err)
			}
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
