package volcengine

import (
	"reflect"
	"strings"
	"testing"
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
