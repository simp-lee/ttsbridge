package edgetts

import "testing"

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
