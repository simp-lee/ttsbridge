package volcengine

import (
	"bytes"
	"errors"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestBuildRequestDirect(t *testing.T) {
	provider := New()

	tests := []struct {
		name      string
		request   tts.SynthesisRequest
		wantVoice string
		wantText  string
		wantCode  string
	}{
		{
			name: "plain text",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModePlainText,
				Text:      "hello volcengine",
				VoiceID:   "  BV001_streaming  ",
			},
			wantVoice: "BV001_streaming",
			wantText:  "hello volcengine",
		},
		{
			name: "prosody reject",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModePlainTextWithProsody,
				Text:      "hello volcengine",
				Prosody:   tts.ProsodyParams{Rate: 1.1},
			},
			wantCode: tts.ErrCodeUnsupportedCapability,
		},
		{
			name: "raw ssml reject",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModeRawSSML,
				SSML:      "<speak>hello</speak>",
			},
			wantCode: tts.ErrCodeUnsupportedCapability,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, voiceID, err := provider.buildRequest(tt.request)
			if tt.wantCode != "" {
				if err == nil {
					t.Fatal("expected buildRequest() to fail")
				}
				var ttsErr *tts.Error
				if !errors.As(err, &ttsErr) {
					t.Fatalf("error type = %T, want *tts.Error", err)
				}
				if ttsErr.Code != tt.wantCode {
					t.Fatalf("error code = %q, want %q", ttsErr.Code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildRequest() error: %v", err)
			}
			if options == nil {
				t.Fatal("options = nil, want non-nil")
			}
			if voiceID != tt.wantVoice {
				t.Fatalf("voiceID = %q, want %q", voiceID, tt.wantVoice)
			}
			if options.Voice != tt.wantVoice {
				t.Fatalf("options.Voice = %q, want %q", options.Voice, tt.wantVoice)
			}
			if options.Text != tt.wantText {
				t.Fatalf("options.Text = %q, want %q", options.Text, tt.wantText)
			}
		})
	}
}

func TestBuildResultDirect_WAVMetadataMatchesAudio(t *testing.T) {
	audio := makeWAVChunkWithProfileForTest(16000, 1, 16, make([]byte, 16000*2))

	result, err := buildResult(audio, "BV700_streaming")
	if err != nil {
		t.Fatalf("buildResult() error: %v", err)
	}

	if !bytes.Equal(result.Audio, audio) {
		t.Fatalf("result.Audio length = %d, want %d with identical bytes", len(result.Audio), len(audio))
	}
	if result.Format != tts.AudioFormatWAV {
		t.Fatalf("result.Format = %q, want %q", result.Format, tts.AudioFormatWAV)
	}
	if result.SampleRate != 16000 {
		t.Fatalf("result.SampleRate = %d, want %d", result.SampleRate, 16000)
	}
	wantDuration, err := tts.InferDuration(audio, tts.VoiceAudioProfile{Format: tts.AudioFormatWAV})
	if err != nil {
		t.Fatalf("InferDuration() error: %v", err)
	}
	if result.Duration != wantDuration {
		t.Fatalf("result.Duration = %v, want %v", result.Duration, wantDuration)
	}
	if result.Provider != providerName {
		t.Fatalf("result.Provider = %q, want %q", result.Provider, providerName)
	}
	if result.VoiceID != "BV700_streaming" {
		t.Fatalf("result.VoiceID = %q, want %q", result.VoiceID, "BV700_streaming")
	}
}
