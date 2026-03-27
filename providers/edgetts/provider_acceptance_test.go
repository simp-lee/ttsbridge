package edgetts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
)

// AC-3: provider 的合成选项结构必须只表达远端 TTS 请求语义，不得包含任何本地媒体处理字段。
func TestSynthesizeOptions_DoNotExposeLocalMediaFields(t *testing.T) {
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

	optionsType := reflect.TypeOf(SynthesizeOptions{})
	for i := range optionsType.NumField() {
		field := optionsType.Field(i)
		if _, forbidden := forbiddenFields[field.Name]; forbidden {
			t.Fatalf("SynthesizeOptions unexpectedly exposes local media field %q", field.Name)
		}

		lowerName := strings.ToLower(field.Name)
		for _, token := range forbiddenTokens {
			if strings.Contains(lowerName, token) {
				t.Fatalf("SynthesizeOptions field %q should not contain local media token %q", field.Name, token)
			}
		}
	}
}

// AC-14: provider 差异封装在 provider 实现和专属选项中，不回流核心抽象。
func TestProviderSpecificOptions_OnlyShareMinimalCrossProviderRequestFields(t *testing.T) {
	sharedAllowed := map[string]struct{}{
		"Text":             {},
		"Voice":            {},
		"ProgressCallback": {},
	}
	edgeOnlyAllowed := map[string]struct{}{
		"Rate":                    {},
		"Volume":                  {},
		"Pitch":                   {},
		"WordBoundaryEnabled":     {},
		"SentenceBoundaryEnabled": {},
		"OutputFormat":            {},
		"BoundaryCallback":        {},
	}

	edgeFields := reflect.VisibleFields(reflect.TypeOf(SynthesizeOptions{}))
	volcFields := reflect.VisibleFields(reflect.TypeOf(volcengine.SynthesizeOptions{}))
	volcFieldSet := make(map[string]struct{}, len(volcFields))
	for _, field := range volcFields {
		volcFieldSet[field.Name] = struct{}{}
	}

	sharedFields := make(map[string]struct{})
	for _, field := range edgeFields {
		if _, ok := volcFieldSet[field.Name]; ok {
			sharedFields[field.Name] = struct{}{}
			if _, allowed := sharedAllowed[field.Name]; !allowed {
				t.Fatalf("provider option field %q is shared across providers but should stay provider-specific", field.Name)
			}
			continue
		}
		if _, allowed := edgeOnlyAllowed[field.Name]; !allowed {
			t.Fatalf("unexpected EdgeTTS-only option field %q; update acceptance guard if this new provider-specific concept is intentional", field.Name)
		}
	}

	if len(sharedFields) != len(sharedAllowed) {
		t.Fatalf("shared option fields = %v, want exactly %v", sharedFields, sharedAllowed)
	}
	for fieldName := range sharedAllowed {
		if _, ok := sharedFields[fieldName]; !ok {
			t.Fatalf("expected shared core request field %q to remain present across providers", fieldName)
		}
	}

	for _, field := range volcFields {
		if _, ok := sharedAllowed[field.Name]; ok {
			continue
		}
		t.Fatalf("unexpected Volcengine-only field %q leaked into the shared provider option shape", field.Name)
	}
}
