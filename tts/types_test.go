package tts_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestProviderCapabilities_HelperMethods(t *testing.T) {
	caps := tts.ProviderCapabilities{
		SupportedFormats:     []string{" MP3 ", "WAV"},
		PreferredAudioFormat: " WAV ",
	}

	for _, format := range []string{"", tts.AudioFormatMP3, " MP3 ", tts.AudioFormatWAV, " wav "} {
		if !caps.SupportsFormat(format) {
			t.Fatalf("SupportsFormat(%q) = false, want true", format)
		}
	}
	if caps.SupportsFormat(tts.AudioFormatPCM) {
		t.Fatalf("SupportsFormat(%q) = true, want false", tts.AudioFormatPCM)
	}

	if got := caps.ResolvedOutputFormat(" MP3 "); got != tts.AudioFormatMP3 {
		t.Fatalf("ResolvedOutputFormat(requested) = %q, want %q", got, tts.AudioFormatMP3)
	}
	if got := caps.ResolvedOutputFormat(""); got != tts.AudioFormatWAV {
		t.Fatalf("ResolvedOutputFormat(empty) = %q, want %q", got, tts.AudioFormatWAV)
	}

	empty := tts.ProviderCapabilities{}
	if got := empty.ResolvedOutputFormat(""); got != "" {
		t.Fatalf("empty.ResolvedOutputFormat(empty) = %q, want empty string", got)
	}
	if !empty.SupportsFormat("") {
		t.Fatal("empty.SupportsFormat(empty) = false, want true")
	}
}

func TestSynthesisRequestValidateAgainst_UnsupportedCombinations(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		request      tts.SynthesisRequest
		capabilities tts.ProviderCapabilities
		wantCode     string
		wantMessage  string
	}{
		{
			name:     "plain text only rejects raw ssml",
			provider: "volcengine",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModeRawSSML,
				SSML:      "<speak>hello</speak>",
			},
			capabilities: tts.ProviderCapabilities{
				PlainTextOnly:  true,
				RawSSML:        true,
				ProsodyParams:  true,
				BoundaryEvents: true,
			},
			wantCode:    tts.ErrCodeUnsupportedCapability,
			wantMessage: "provider only supports plain text input",
		},
		{
			name:     "plain text only rejects prosody mode",
			provider: "volcengine",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModePlainTextWithProsody,
				Text:      "hello",
				Prosody:   tts.ProsodyParams{Rate: 1.1},
			},
			capabilities: tts.ProviderCapabilities{
				PlainTextOnly:  true,
				RawSSML:        true,
				ProsodyParams:  true,
				BoundaryEvents: true,
			},
			wantCode:    tts.ErrCodeUnsupportedCapability,
			wantMessage: "provider only supports plain text input",
		},
		{
			name:     "raw ssml unsupported",
			provider: "edgetts",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModeRawSSML,
				SSML:      "<speak>hello</speak>",
			},
			capabilities: tts.ProviderCapabilities{},
			wantCode:     tts.ErrCodeUnsupportedCapability,
			wantMessage:  "raw SSML is not supported by provider",
		},
		{
			name:     "prosody unsupported",
			provider: "volcengine",
			request: tts.SynthesisRequest{
				InputMode: tts.InputModePlainTextWithProsody,
				Text:      "hello",
				Prosody:   tts.ProsodyParams{Rate: 1.1},
			},
			capabilities: tts.ProviderCapabilities{},
			wantCode:     tts.ErrCodeUnsupportedCapability,
			wantMessage:  "prosody parameters are not supported by provider",
		},
		{
			name:     "boundary events unsupported",
			provider: "volcengine",
			request: tts.SynthesisRequest{
				InputMode:          tts.InputModePlainText,
				Text:               "hello",
				NeedBoundaryEvents: true,
			},
			capabilities: tts.ProviderCapabilities{
				SupportedFormats:     []string{tts.AudioFormatMP3},
				PreferredAudioFormat: tts.AudioFormatMP3,
			},
			wantCode:    tts.ErrCodeUnsupportedCapability,
			wantMessage: "boundary events are not supported by provider",
		},
		{
			name:     "requested format unsupported",
			provider: "volcengine",
			request: tts.SynthesisRequest{
				InputMode:    tts.InputModePlainText,
				Text:         "hello",
				OutputFormat: tts.AudioFormatWAV,
			},
			capabilities: tts.ProviderCapabilities{
				SupportedFormats:     []string{tts.AudioFormatMP3},
				PreferredAudioFormat: tts.AudioFormatMP3,
			},
			wantCode:    tts.ErrCodeUnsupportedFormat,
			wantMessage: "output format is not supported by provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.ValidateAgainst(tt.provider, tt.capabilities)
			if err == nil {
				t.Fatal("ValidateAgainst() error = nil, want failure")
			}

			var ttsErr *tts.Error
			if !errors.As(err, &ttsErr) {
				t.Fatalf("ValidateAgainst() error type = %T, want *tts.Error", err)
			}
			if ttsErr.Code != tt.wantCode {
				t.Fatalf("Code = %q, want %q", ttsErr.Code, tt.wantCode)
			}
			if ttsErr.Message != tt.wantMessage {
				t.Fatalf("Message = %q, want %q", ttsErr.Message, tt.wantMessage)
			}
			if ttsErr.Provider != tt.provider {
				t.Fatalf("Provider = %q, want %q", ttsErr.Provider, tt.provider)
			}
		})
	}
}

func TestSynthesisRequestValidateStreamAgainst_StreamSpecificCombinations(t *testing.T) {
	t.Run("boundary events rejected for stream even when provider advertises both features", func(t *testing.T) {
		request := tts.SynthesisRequest{
			InputMode:          tts.InputModePlainText,
			Text:               "hello",
			NeedBoundaryEvents: true,
		}

		err := request.ValidateStreamAgainst("edgetts", tts.ProviderCapabilities{
			BoundaryEvents: true,
			Streaming:      true,
		})
		if err == nil {
			t.Fatal("ValidateStreamAgainst() error = nil, want failure")
		}

		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("ValidateStreamAgainst() error type = %T, want *tts.Error", err)
		}
		if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
			t.Fatalf("Code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
		}
		if ttsErr.Message != "boundary events are only available from Synthesize results" {
			t.Fatalf("Message = %q, want %q", ttsErr.Message, "boundary events are only available from Synthesize results")
		}
		if ttsErr.Provider != "edgetts" {
			t.Fatalf("Provider = %q, want %q", ttsErr.Provider, "edgetts")
		}
	})

	t.Run("streaming is rejected when provider does not advertise support", func(t *testing.T) {
		request := tts.SynthesisRequest{
			InputMode:    tts.InputModePlainText,
			Text:         "hello",
			OutputFormat: tts.AudioFormatWAV,
		}

		err := request.ValidateStreamAgainst("volcengine", tts.ProviderCapabilities{
			PlainTextOnly:        true,
			Streaming:            false,
			SupportedFormats:     []string{tts.AudioFormatWAV},
			PreferredAudioFormat: tts.AudioFormatWAV,
		})
		if err == nil {
			t.Fatal("ValidateStreamAgainst() error = nil, want failure")
		}

		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("ValidateStreamAgainst() error type = %T, want *tts.Error", err)
		}
		if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
			t.Fatalf("Code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
		}
		if ttsErr.Message != "streaming synthesis is not supported by provider" {
			t.Fatalf("Message = %q, want %q", ttsErr.Message, "streaming synthesis is not supported by provider")
		}
	})
}

func TestSynthesisRequestValidate_RejectsInvalidProsodyValues(t *testing.T) {
	tests := []struct {
		name        string
		prosody     tts.ProsodyParams
		wantMessage string
	}{
		{
			name:        "rate below minimum",
			prosody:     (tts.ProsodyParams{}).WithRate(0.4),
			wantMessage: "prosody rate must be between 0.5 and 2.0",
		},
		{
			name:        "rate must be finite",
			prosody:     (tts.ProsodyParams{}).WithRate(math.NaN()),
			wantMessage: "prosody rate must be a finite value",
		},
		{
			name:        "volume above maximum",
			prosody:     (tts.ProsodyParams{}).WithVolume(2.1),
			wantMessage: "prosody volume must be between 0.0 and 2.0",
		},
		{
			name:        "pitch must be finite",
			prosody:     (tts.ProsodyParams{}).WithPitch(math.Inf(1)),
			wantMessage: "prosody pitch must be a finite value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (tts.SynthesisRequest{
				InputMode: tts.InputModePlainTextWithProsody,
				Text:      "hello",
				Prosody:   tt.prosody,
			}).Validate("edgetts")
			if err == nil {
				t.Fatal("Validate() error = nil, want invalid prosody failure")
			}

			var ttsErr *tts.Error
			if !errors.As(err, &ttsErr) {
				t.Fatalf("Validate() error type = %T, want *tts.Error", err)
			}
			if ttsErr.Code != tts.ErrCodeInvalidInput {
				t.Fatalf("Code = %q, want %q", ttsErr.Code, tts.ErrCodeInvalidInput)
			}
			if ttsErr.Message != tt.wantMessage {
				t.Fatalf("Message = %q, want %q", ttsErr.Message, tt.wantMessage)
			}
		})
	}
}

func TestProsodyParams_ExplicitZeroVolumePreservesPresence(t *testing.T) {
	prosody := tts.ProsodyParams{Volume: 0, VolumeSet: true}
	if prosody.IsZero() {
		t.Fatal("ProsodyParams{Volume: 0, VolumeSet: true}.IsZero() = true, want false")
	}
	if !prosody.HasVolume() {
		t.Fatal("ProsodyParams{Volume: 0, VolumeSet: true}.HasVolume() = false, want true")
	}

	data, err := json.Marshal(prosody)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if string(data) != `{"volume":0}` {
		t.Fatalf("json.Marshal() = %s, want {\"volume\":0}", data)
	}

	var decoded tts.ProsodyParams
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal() error: %v", unmarshalErr)
	}
	if !decoded.HasVolume() {
		t.Fatal("decoded.HasVolume() = false, want true")
	}
	if !decoded.VolumeSet {
		t.Fatal("decoded.VolumeSet = false, want true")
	}
	if decoded.Volume != 0 {
		t.Fatalf("decoded.Volume = %v, want 0", decoded.Volume)
	}

	err = (tts.SynthesisRequest{
		InputMode: tts.InputModePlainTextWithProsody,
		Text:      "hello",
		Prosody:   decoded,
	}).Validate("edgetts")
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil for explicit mute volume", err)
	}
}

func TestSynthesisRequestValidate_RejectsAmbiguousZeroVolumeLiteral(t *testing.T) {
	err := (tts.SynthesisRequest{
		InputMode: tts.InputModePlainTextWithProsody,
		Text:      "hello",
		Prosody:   tts.ProsodyParams{Volume: 0},
	}).Validate("edgetts")
	if err == nil {
		t.Fatal("Validate() error = nil, want invalid ambiguous zero-volume prosody")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("Validate() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("Code = %q, want %q", ttsErr.Code, tts.ErrCodeInvalidInput)
	}
	if ttsErr.Message != "plain_text_with_prosody mode requires at least one prosody field; set VolumeSet to send volume 0 explicitly" {
		t.Fatalf("Message = %q, want explicit zero-volume guidance", ttsErr.Message)
	}
}
