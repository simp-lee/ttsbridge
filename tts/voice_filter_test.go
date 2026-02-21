package tts

import "testing"

func TestFilterVoices(t *testing.T) {
	voices := []Voice{
		{ID: "zh-CN-XiaoxiaoNeural", Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts"},
		{ID: "zh-CN-YunxiNeural", Language: "zh-CN", Gender: GenderMale, Provider: "edgetts"},
		{ID: "en-US-JennyNeural", Language: "en-US", Gender: GenderFemale, Provider: "edgetts"},
		{ID: "en-US-GuyNeural", Language: "en-US", Gender: GenderMale, Provider: "edgetts"},
		{ID: "zh-CN-Default", Language: "zh-CN", Gender: GenderFemale, Provider: "volcengine"},
		{ID: "ja-JP-NanamiNeural", Language: "ja-JP", Gender: GenderFemale, Provider: "edgetts"},
	}

	tests := []struct {
		name     string
		filter   VoiceFilter
		expected int
	}{
		{"no filter", VoiceFilter{}, 6},
		{"language zh-CN", VoiceFilter{Language: "zh-CN"}, 3},
		{"language prefix zh", VoiceFilter{Language: "zh"}, 3},
		{"language en", VoiceFilter{Language: "en"}, 2},
		{"gender female", VoiceFilter{Gender: GenderFemale}, 4},
		{"gender male", VoiceFilter{Gender: GenderMale}, 2},
		{"provider edgetts", VoiceFilter{Provider: "edgetts"}, 5},
		{"provider volcengine", VoiceFilter{Provider: "volcengine"}, 1},
		{"language + gender", VoiceFilter{Language: "zh-CN", Gender: GenderFemale}, 2},
		{"language + provider", VoiceFilter{Language: "en-US", Provider: "edgetts"}, 2},
		{"all filters", VoiceFilter{Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts"}, 1},
		{"no match", VoiceFilter{Language: "fr-FR"}, 0},
		{"empty result provider", VoiceFilter{Provider: "nonexistent"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterVoices(voices, tt.filter)
			if len(result) != tt.expected {
				t.Errorf("FilterVoices() returned %d results, expected %d", len(result), tt.expected)
			}
		})
	}
}

func TestFilterVoices_FilterFunc(t *testing.T) {
	voices := []Voice{
		{ID: "zh-CN-XiaoxiaoNeural", Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts", Extra: &testExtra{Status: "GA", Styles: []string{"cheerful", "sad"}}},
		{ID: "zh-CN-YunxiNeural", Language: "zh-CN", Gender: GenderMale, Provider: "edgetts", Extra: &testExtra{Status: "Preview", Styles: []string{"narration"}}},
		{ID: "en-US-JennyNeural", Language: "en-US", Gender: GenderFemale, Provider: "edgetts", Extra: &testExtra{Status: "GA", Styles: []string{"cheerful"}}},
		{ID: "en-US-GuyNeural", Language: "en-US", Gender: GenderMale, Provider: "edgetts"},
	}

	tests := []struct {
		name     string
		filter   VoiceFilter
		expected int
	}{
		{
			name: "FilterFunc only - by Extra status GA",
			filter: VoiceFilter{
				FilterFunc: func(v Voice) bool {
					extra, ok := GetExtra[*testExtra](&v)
					return ok && extra.Status == "GA"
				},
			},
			expected: 2,
		},
		{
			name: "FilterFunc only - by Extra style cheerful",
			filter: VoiceFilter{
				FilterFunc: func(v Voice) bool {
					extra, ok := GetExtra[*testExtra](&v)
					if !ok {
						return false
					}
					for _, s := range extra.Styles {
						if s == "cheerful" {
							return true
						}
					}
					return false
				},
			},
			expected: 2,
		},
		{
			name: "FilterFunc combined with Language",
			filter: VoiceFilter{
				Language: "zh-CN",
				FilterFunc: func(v Voice) bool {
					extra, ok := GetExtra[*testExtra](&v)
					return ok && extra.Status == "GA"
				},
			},
			expected: 1,
		},
		{
			name: "FilterFunc rejects all",
			filter: VoiceFilter{
				FilterFunc: func(v Voice) bool { return false },
			},
			expected: 0,
		},
		{
			name: "FilterFunc accepts all",
			filter: VoiceFilter{
				FilterFunc: func(v Voice) bool { return true },
			},
			expected: 4,
		},
		{
			name:     "nil FilterFunc has no effect",
			filter:   VoiceFilter{},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterVoices(voices, tt.filter)
			if len(result) != tt.expected {
				var ids []string
				for _, v := range result {
					ids = append(ids, v.ID)
				}
				t.Errorf("FilterVoices() returned %d results %v, expected %d", len(result), ids, tt.expected)
			}
		})
	}
}

func TestFilterVoices_FilterByCategory(t *testing.T) {
	type categoryExtra struct {
		Categories []string
	}

	voices := []Voice{
		{ID: "zh-news", Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts", Extra: &categoryExtra{Categories: []string{"News", "Assistant"}}},
		{ID: "zh-chat", Language: "zh-CN", Gender: GenderMale, Provider: "edgetts", Extra: &categoryExtra{Categories: []string{"Chat"}}},
		{ID: "en-news", Language: "en-US", Gender: GenderFemale, Provider: "edgetts", Extra: &categoryExtra{Categories: []string{"News"}}},
	}

	containsCategory := func(category string) func(Voice) bool {
		return func(v Voice) bool {
			extra, ok := GetExtra[*categoryExtra](&v)
			if !ok {
				return false
			}
			for _, c := range extra.Categories {
				if c == category {
					return true
				}
			}
			return false
		}
	}

	tests := []struct {
		name      string
		filter    VoiceFilter
		wantCount int
		wantFirst string
	}{
		{name: "category only", filter: VoiceFilter{FilterFunc: containsCategory("News")}, wantCount: 2, wantFirst: "zh-news"},
		{name: "category with language", filter: VoiceFilter{Language: "zh", FilterFunc: containsCategory("News")}, wantCount: 1, wantFirst: "zh-news"},
		{name: "missing category", filter: VoiceFilter{FilterFunc: containsCategory("Narration")}, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterVoices(voices, tt.filter)
			if len(got) != tt.wantCount {
				t.Fatalf("FilterVoices() count = %d, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].ID != tt.wantFirst {
				t.Fatalf("first matched ID = %q, want %q", got[0].ID, tt.wantFirst)
			}
		})
	}
}

func TestFilterVoices_FilterByPersonality(t *testing.T) {
	type personalityExtra struct {
		Personalities []string
	}

	voices := []Voice{
		{ID: "voice-cheerful", Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts", Extra: &personalityExtra{Personalities: []string{"Cheerful", "Friendly"}}},
		{ID: "voice-serious", Language: "zh-CN", Gender: GenderMale, Provider: "edgetts", Extra: &personalityExtra{Personalities: []string{"Serious"}}},
		{ID: "voice-nil-extra", Language: "en-US", Gender: GenderFemale, Provider: "edgetts"},
	}

	hasPersonality := func(target string) func(Voice) bool {
		return func(v Voice) bool {
			extra, ok := GetExtra[*personalityExtra](&v)
			if !ok {
				return false
			}
			for _, p := range extra.Personalities {
				if p == target {
					return true
				}
			}
			return false
		}
	}

	result := FilterVoices(voices, VoiceFilter{FilterFunc: hasPersonality("Cheerful")})
	if len(result) != 1 {
		t.Fatalf("FilterVoices() count = %d, want 1", len(result))
	}
	if result[0].ID != "voice-cheerful" {
		t.Fatalf("matched ID = %q, want %q", result[0].ID, "voice-cheerful")
	}

	noMatch := FilterVoices(voices, VoiceFilter{FilterFunc: hasPersonality("Calm")})
	if len(noMatch) != 0 {
		t.Fatalf("FilterVoices() count = %d, want 0", len(noMatch))
	}
}

func TestFilterVoices_EmptyInput(t *testing.T) {
	result := FilterVoices(nil, VoiceFilter{Language: "en"})
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(result))
	}

	result = FilterVoices([]Voice{}, VoiceFilter{Language: "en"})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(result))
	}
}
