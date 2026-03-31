package volcengine

import (
	"context"
	"reflect"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
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
			voices, err := provider.ListVoices(context.Background(), tts.VoiceFilter{Language: tt.locale})
			if err != nil {
				t.Fatalf("ListVoices(%q) error: %v", tt.locale, err)
			}
			if len(voices) != tt.want {
				t.Fatalf("ListVoices(%q) returned %d voices, want %d", tt.locale, len(voices), tt.want)
			}
		})
	}
}

func TestGetAllVoices_PopulatesMultilingualCatalogMetadata(t *testing.T) {
	voicesByID := make(map[string]tts.Voice, len(GetAllVoices()))
	for _, voice := range GetAllVoices() {
		voicesByID[voice.ID] = voice
	}

	tests := []struct {
		voiceID string
		want    []tts.Language
	}{
		{
			voiceID: "BV700_streaming",
			want: []tts.Language{
				tts.LanguageZhCN,
				tts.LanguageEnUS,
				tts.LanguageJaJP,
				tts.LanguagePtBR,
				tts.LanguageEsMX,
				tts.LanguageIDID,
			},
		},
		{
			voiceID: "BV034_streaming",
			want:    []tts.Language{tts.LanguageZhCN, tts.LanguageEnUS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.voiceID, func(t *testing.T) {
			voice, ok := voicesByID[tt.voiceID]
			if !ok {
				t.Fatalf("voice %q not found", tt.voiceID)
			}
			if !reflect.DeepEqual(voice.Languages, tt.want) {
				t.Fatalf("voice %q languages = %v, want %v", tt.voiceID, voice.Languages, tt.want)
			}
		})
	}
}

func TestListVoices_ExposesMultilingualVoicesThroughSharedLanguageFilter(t *testing.T) {
	provider := New()

	tests := []struct {
		name   string
		locale string
		want   []string
	}{
		{
			name:   "english prefix includes multilingual and dedicated english voices",
			locale: "en",
			want:   []string{"BV700_streaming", "BV034_streaming", "BV503_streaming", "BV504_streaming"},
		},
		{
			name:   "japanese prefix includes multilingual chinese voice",
			locale: "ja",
			want:   []string{"BV700_streaming", "BV522_streaming", "BV524_streaming"},
		},
		{
			name:   "brazilian portuguese exact locale matches multilingual voice",
			locale: "pt-BR",
			want:   []string{"BV700_streaming"},
		},
		{
			name:   "mexican spanish exact locale matches multilingual voice",
			locale: "es-MX",
			want:   []string{"BV700_streaming"},
		},
		{
			name:   "indonesian exact locale matches multilingual voice",
			locale: "id-ID",
			want:   []string{"BV700_streaming"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voices, err := provider.ListVoices(context.Background(), tts.VoiceFilter{Language: tt.locale})
			if err != nil {
				t.Fatalf("ListVoices(%q) error: %v", tt.locale, err)
			}

			got := make([]string, len(voices))
			for i, voice := range voices {
				got[i] = voice.ID
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ListVoices(%q) IDs = %v, want %v", tt.locale, got, tt.want)
			}
		})
	}
}
