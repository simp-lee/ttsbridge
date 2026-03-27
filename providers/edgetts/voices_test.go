package edgetts

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFilterAndConvertVoices(t *testing.T) {
	tests := []struct {
		name      string
		locale    string
		entries   []voiceListEntry
		expectIDs []string
	}{
		{
			name:   "returns non deprecated voices",
			locale: "",
			entries: []voiceListEntry{
				{
					Name:      "Edge Voice",
					ShortName: "en-US-TestNeural",
					Gender:    "Female",
					Locale:    "en-US",
					Status:    "GA",
				},
				{
					Name:      "Old Voice",
					ShortName: "en-US-Deprecated",
					Locale:    "en-US",
					Status:    "Deprecated",
				},
			},
			expectIDs: []string{"en-US-TestNeural"},
		},
		{
			name:   "respects locale filter",
			locale: "zh",
			entries: []voiceListEntry{
				{
					Name:                "Chinese Voice",
					ShortName:           "zh-CN-XiaoxiaoNeural",
					Gender:              "Female",
					Locale:              "zh-CN",
					SecondaryLocaleList: []string{"zh-HK"},
					Status:              "GA",
				},
				{
					Name:      "English Voice",
					ShortName: "en-US-JennyNeural",
					Gender:    "Female",
					Locale:    "en-US",
					Status:    "GA",
				},
			},
			expectIDs: []string{"zh-CN-XiaoxiaoNeural"},
		},
	}

	for _, tt := range tests {
		voices := filterAndConvertVoices(tt.entries, tt.locale)
		if len(voices) != len(tt.expectIDs) {
			t.Fatalf("%s: expected %d voices, got %d", tt.name, len(tt.expectIDs), len(voices))
		}
		for i, id := range tt.expectIDs {
			if voices[i].ID != id {
				t.Fatalf("%s: expected voice %q at index %d, got %q", tt.name, id, i, voices[i].ID)
			}
		}
	}
}

func TestCollectLanguages(t *testing.T) {
	languages := collectLanguages("en-US", []string{"en-GB", "en-US", "en-AU"})
	if len(languages) != 3 {
		t.Fatalf("expected 3 languages after dedupe, got %d", len(languages))
	}
	if string(languages[0]) != "en-US" {
		t.Fatalf("expected primary language en-US, got %s", languages[0])
	}
}

func TestVoiceExtraCopy(t *testing.T) {
	entry := voiceListEntry{
		ShortName:           "en-US-TestNeural",
		FriendlyName:        "Test",
		Locale:              "en-US",
		SecondaryLocaleList: []string{"en-GB"},
		StyleList:           []string{"chat"},
		RolePlayList:        []string{"narration"},
		SuggestedCodec:      "audio-24khz-48kbitrate-mono-mp3",
		Status:              "GA",
	}
	entry.VoiceTag.ContentCategories = []string{"General"}
	entry.VoiceTag.VoicePersonalities = []string{"Calm"}

	voices := filterAndConvertVoices([]voiceListEntry{entry}, "")
	if len(voices) != 1 {
		t.Fatalf("expected a single voice, got %d", len(voices))
	}
	voice := voices[0]
	extra, ok := voice.Extra.(*VoiceExtra)
	if !ok {
		t.Fatalf("expected VoiceExtra type, got %T", voice.Extra)
	}
	if len(extra.SecondaryLocales) != 1 || extra.SecondaryLocales[0] != "en-GB" {
		t.Fatalf("unexpected secondary locales: %+v", extra.SecondaryLocales)
	}
	if len(extra.Styles) != 1 || extra.Styles[0] != "chat" {
		t.Fatalf("unexpected styles: %+v", extra.Styles)
	}
	if len(extra.Categories) != 1 || extra.Categories[0] != "General" {
		t.Fatalf("unexpected categories: %+v", extra.Categories)
	}
	if extra.SuggestedCodec != "audio-24khz-48kbitrate-mono-mp3" {
		t.Fatalf("unexpected codec: %s", extra.SuggestedCodec)
	}
}

func TestVoiceCacheLocaleFilteringMatchesDirectFiltering(t *testing.T) {
	entries := []voiceListEntry{
		{
			Name:                "Chinese Voice",
			ShortName:           "zh-CN-XiaoxiaoNeural",
			Gender:              "Female",
			Locale:              "zh-CN",
			SecondaryLocaleList: []string{"zh-HK"},
			Status:              "GA",
		},
		{
			Name:      "English Voice",
			ShortName: "en-US-JennyNeural",
			Gender:    "Female",
			Locale:    "en-US",
			Status:    "GA",
		},
	}

	cache := tts.NewVoiceCache(func(ctx context.Context) ([]tts.Voice, error) {
		return filterAndConvertVoices(entries, ""), nil
	})

	for _, locale := range []string{"zh", "en", "zh-CN", "en-US", "e", "en-", "zh-"} {
		want := filterAndConvertVoices(entries, locale)
		got, err := cache.Get(context.Background(), locale)
		if err != nil {
			t.Fatalf("cache.Get(%q) error: %v", locale, err)
		}
		if len(got) != len(want) {
			t.Fatalf("locale %q: got %d voices, want %d", locale, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != want[i].ID {
				t.Fatalf("locale %q: got voice %q at index %d, want %q", locale, got[i].ID, i, want[i].ID)
			}
		}
	}
}

func TestFilterAndConvertVoices_RejectsInvalidLocalePrefixes(t *testing.T) {
	entries := []voiceListEntry{
		{
			Name:                "Chinese Voice",
			ShortName:           "zh-CN-XiaoxiaoNeural",
			Gender:              "Female",
			Locale:              "zh-CN",
			SecondaryLocaleList: []string{"zh-HK"},
			Status:              "GA",
		},
		{
			Name:      "English Voice",
			ShortName: "en-US-JennyNeural",
			Gender:    "Female",
			Locale:    "en-US",
			Status:    "GA",
		},
	}

	for _, locale := range []string{"e", "en-", "zh-"} {
		voices := filterAndConvertVoices(entries, locale)
		if len(voices) != 0 {
			t.Fatalf("filterAndConvertVoices(%q) returned %d voices, want 0", locale, len(voices))
		}
	}
}

func TestFetchVoiceList_ForbiddenWithGenericDateStaysAuthFailed(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Date": []string{"Wed, 25 Mar 2026 10:00:00 GMT"},
			},
			Body: io.NopCloser(strings.NewReader("Forbidden")),
		}, nil
	})}

	_, err := fetchVoiceList(context.Background(), client, "token")
	if err == nil {
		t.Fatal("fetchVoiceList() error = nil, want auth failure")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("fetchVoiceList() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeAuthFailed {
		t.Fatalf("fetchVoiceList() code = %s, want %s", ttsErr.Code, tts.ErrCodeAuthFailed)
	}
}

func TestProviderListVoices_NilContextUsesBackground(t *testing.T) {
	provider := New()
	provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Context() == nil {
			t.Fatal("request context is nil")
		}
		body := `[{"Name":"Edge Voice","ShortName":"en-US-TestNeural","Gender":"Female","Locale":"en-US","Status":"GA"}]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	voices, err := provider.ListVoices(nilContextForTest(), "en-US")
	if err != nil {
		t.Fatalf("ListVoices(nil, ...) error: %v", err)
	}
	if len(voices) != 1 {
		t.Fatalf("ListVoices(nil, ...) returned %d voices; want 1", len(voices))
	}
	if voices[0].ID != "en-US-TestNeural" {
		t.Fatalf("voice ID = %q; want %q", voices[0].ID, "en-US-TestNeural")
	}
}

func TestFetchVoiceList_ForbiddenWithMalformedClockSkewDateStaysAuthFailed(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Date": []string{"not-a-date"},
			},
			Body: io.NopCloser(strings.NewReader("Request timestamp expired")),
		}, nil
	})}

	_, err := fetchVoiceList(context.Background(), client, "token")
	if err == nil {
		t.Fatal("fetchVoiceList() error = nil, want auth failure")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("fetchVoiceList() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeAuthFailed {
		t.Fatalf("fetchVoiceList() code = %s, want %s", ttsErr.Code, tts.ErrCodeAuthFailed)
	}
	if ttsErr.Err == nil || !strings.Contains(ttsErr.Err.Error(), "failed to parse server date") {
		t.Fatalf("fetchVoiceList() wrapped err = %v, want parse failure detail", ttsErr.Err)
	}
}
