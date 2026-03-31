package edgetts

import (
	"reflect"
	"strings"
	"testing"
)

// AC-3: provider 的合成选项结构必须只表达远端 TTS 请求语义，不得包含任何本地媒体处理字段。
func TestInternalRequestTranslationShape_DoNotExposeLocalMediaFields(t *testing.T) {
	forbiddenFields := map[string]struct{}{
		"BackgroundMusic":        {},
		"BackgroundMusicFile":    {},
		"BackgroundMusicOptions": {},
		"AudioFile":              {},
		"AudioPath":              {},
		"MixOptions":             {},
		"FFmpegPath":             {},
		"FFprobePath":            {},
	}
	forbiddenTokens := []string{"background", "music", "mix", "ffmpeg", "ffprobe", "upload", "file", "path"}

	optionsType := reflect.TypeOf(synthesizeOptions{})
	for i := range optionsType.NumField() {
		field := optionsType.Field(i)
		if _, forbidden := forbiddenFields[field.Name]; forbidden {
			t.Fatalf("internal request translation struct unexpectedly exposes local media field %q", field.Name)
		}

		lowerName := strings.ToLower(field.Name)
		for _, token := range forbiddenTokens {
			if strings.Contains(lowerName, token) {
				t.Fatalf("internal request translation field %q should not contain local media token %q", field.Name, token)
			}
		}
	}
}

// AC-14: provider 内部请求翻译层必须保持聚焦，不得重新长出新的本地或公共入口字段。
func TestInternalRequestTranslationShape_RemainsFocused(t *testing.T) {
	allowedFields := map[string]struct{}{
		"Text":                    {},
		"Voice":                   {},
		"Rate":                    {},
		"Volume":                  {},
		"Pitch":                   {},
		"WordBoundaryEnabled":     {},
		"SentenceBoundaryEnabled": {},
		"OutputFormat":            {},
		"BoundaryCallback":        {},
		"ProgressCallback":        {},
	}

	fields := reflect.VisibleFields(reflect.TypeOf(synthesizeOptions{}))
	if len(fields) != len(allowedFields) {
		t.Fatalf("internal request translation field count = %d, want %d", len(fields), len(allowedFields))
	}

	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		seen[field.Name] = struct{}{}
		if _, allowed := allowedFields[field.Name]; !allowed {
			t.Fatalf("unexpected internal request translation field %q; update acceptance guard if this new provider-native concept is intentional", field.Name)
		}
	}

	for fieldName := range allowedFields {
		if _, ok := seen[fieldName]; !ok {
			t.Fatalf("expected internal request translation field %q to remain present", fieldName)
		}
	}
}
