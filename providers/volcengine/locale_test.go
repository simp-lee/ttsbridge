package volcengine

import (
	"context"
	"testing"
)

func TestListVoices_UsesSharedLocaleSemantics(t *testing.T) {
	provider := New()

	tests := []struct {
		name   string
		locale string
		want   int
	}{
		{name: "uppercase exact locale", locale: "ZH-CN", want: 17},
		{name: "whitespace padded prefix", locale: "  zh  ", want: 17},
		{name: "malformed prefix rejected", locale: "en-", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voices, err := provider.ListVoices(context.Background(), tt.locale)
			if err != nil {
				t.Fatalf("ListVoices(%q) error: %v", tt.locale, err)
			}
			if len(voices) != tt.want {
				t.Fatalf("ListVoices(%q) returned %d voices, want %d", tt.locale, len(voices), tt.want)
			}
		})
	}
}
