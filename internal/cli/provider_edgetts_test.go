package cli

import (
	"reflect"
	"testing"
	"time"
)

func TestNewEdgeTTSAdapter_HTTPTimeoutAlsoSetsReceiveTimeout(t *testing.T) {
	adapter, err := newEdgeTTSAdapter(&ProviderConfig{HTTPTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("newEdgeTTSAdapter() error: %v", err)
	}

	edgeAdapter, ok := adapter.(*edgeTTSAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *edgeTTSAdapter", adapter)
	}

	providerValue := reflect.ValueOf(edgeAdapter.provider).Elem()
	receiveTimeout := time.Duration(providerValue.FieldByName("receiveTimeout").Int())
	if receiveTimeout != 5*time.Second {
		t.Fatalf("receiveTimeout = %v, want %v", receiveTimeout, 5*time.Second)
	}

	connectTimeout := time.Duration(providerValue.FieldByName("connectTimeout").Int())
	if connectTimeout != 5*time.Second {
		t.Fatalf("connectTimeout = %v, want %v", connectTimeout, 5*time.Second)
	}
}
