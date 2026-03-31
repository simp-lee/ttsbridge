package cli

import (
	"reflect"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
)

func TestNewEdgeTTSProvider_HTTPTimeoutAlsoSetsAllTimeouts(t *testing.T) {
	provider, err := newEdgeTTSProvider(&ProviderConfig{HTTPTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("newEdgeTTSProvider() error: %v", err)
	}

	edgeProvider, ok := provider.(*edgetts.Provider)
	if !ok {
		t.Fatalf("provider type = %T, want *edgetts.Provider", provider)
	}

	providerValue := reflect.ValueOf(edgeProvider).Elem()
	clientField := providerValue.FieldByName("client")
	if clientField.IsNil() {
		t.Fatal("client is nil, want initialized HTTP client")
	}

	httpTimeout := time.Duration(clientField.Elem().FieldByName("Timeout").Int())
	if httpTimeout != 5*time.Second {
		t.Fatalf("client.Timeout = %v, want %v", httpTimeout, 5*time.Second)
	}

	receiveTimeout := time.Duration(providerValue.FieldByName("receiveTimeout").Int())
	if receiveTimeout != 5*time.Second {
		t.Fatalf("receiveTimeout = %v, want %v", receiveTimeout, 5*time.Second)
	}

	connectTimeout := time.Duration(providerValue.FieldByName("connectTimeout").Int())
	if connectTimeout != 5*time.Second {
		t.Fatalf("connectTimeout = %v, want %v", connectTimeout, 5*time.Second)
	}
}
