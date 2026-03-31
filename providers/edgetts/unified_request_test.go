package edgetts

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type synthesisPlanExpectation struct {
	voiceID              string
	text                 string
	finalFormat          string
	nativeFormat         string
	outputFormat         string
	rate                 float64
	volume               float64
	pitch                float64
	wantBoundaryEvents   bool
	wantLimitationsEmpty bool
}

func TestPrepareSynthesisDirect_PlainText(t *testing.T) {
	provider := New()
	plan, err := provider.prepareSynthesis(tts.SynthesisRequest{
		InputMode:          tts.InputModePlainText,
		Text:               "hello world",
		VoiceID:            "  en-US-AvaMultilingualNeural  ",
		NeedBoundaryEvents: true,
	})
	if err != nil {
		t.Fatalf("prepareSynthesis() error: %v", err)
	}

	assertSynthesisPlanMatches(t, plan, synthesisPlanExpectation{
		voiceID:              "en-US-AvaMultilingualNeural",
		text:                 "hello world",
		finalFormat:          tts.AudioFormatMP3,
		nativeFormat:         tts.AudioFormatMP3,
		outputFormat:         defaultOutputFormat,
		rate:                 1.0,
		volume:               1.0,
		pitch:                1.0,
		wantBoundaryEvents:   true,
		wantLimitationsEmpty: true,
	})
}

func TestPrepareSynthesisDirect_PlainTextWithProsody(t *testing.T) {
	provider := New()
	plan, err := provider.prepareSynthesis(tts.SynthesisRequest{
		InputMode: tts.InputModePlainTextWithProsody,
		Text:      "prosody request",
		Prosody: tts.ProsodyParams{
			Rate:   1.25,
			Volume: 0.8,
		},
	})
	if err != nil {
		t.Fatalf("prepareSynthesis() error: %v", err)
	}

	assertSynthesisPlanMatches(t, plan, synthesisPlanExpectation{
		voiceID:              defaultVoice,
		text:                 "prosody request",
		finalFormat:          tts.AudioFormatMP3,
		nativeFormat:         tts.AudioFormatMP3,
		outputFormat:         defaultOutputFormat,
		rate:                 1.25,
		volume:               0.8,
		pitch:                1.0,
		wantBoundaryEvents:   false,
		wantLimitationsEmpty: true,
	})
}

func TestPrepareSynthesisDirect_ExplicitZeroVolumePreserved(t *testing.T) {
	provider := New()
	plan, err := provider.prepareSynthesis(tts.SynthesisRequest{
		InputMode: tts.InputModePlainTextWithProsody,
		Text:      "mute volume",
		Prosody:   tts.ProsodyParams{Volume: 0, VolumeSet: true},
	})
	if err != nil {
		t.Fatalf("prepareSynthesis() error: %v", err)
	}

	assertSynthesisPlanMatches(t, plan, synthesisPlanExpectation{
		voiceID:              defaultVoice,
		text:                 "mute volume",
		finalFormat:          tts.AudioFormatMP3,
		nativeFormat:         tts.AudioFormatMP3,
		outputFormat:         defaultOutputFormat,
		rate:                 1.0,
		volume:               0,
		pitch:                1.0,
		wantBoundaryEvents:   false,
		wantLimitationsEmpty: true,
	})
}

func assertSynthesisPlanMatches(t *testing.T, plan *synthesisPlan, want synthesisPlanExpectation) {
	t.Helper()

	assertSynthesisPlanCore(t, plan, want)
	options := requireSynthesisOptions(t, plan)
	assertSynthesisPlanOptions(t, options, want)
	assertSynthesisPlanBoundaryFlags(t, options, want.wantBoundaryEvents)
	assertSynthesisPlanProsody(t, options, want)
	assertSynthesisPlanProfile(t, plan, want)
}

func assertSynthesisPlanCore(t *testing.T, plan *synthesisPlan, want synthesisPlanExpectation) {
	t.Helper()

	if plan.voiceID != want.voiceID {
		t.Fatalf("plan.voiceID = %q, want %q", plan.voiceID, want.voiceID)
	}
	if plan.finalFormat != want.finalFormat {
		t.Fatalf("plan.finalFormat = %q, want %q", plan.finalFormat, want.finalFormat)
	}
	if want.wantLimitationsEmpty && len(plan.limitations) != 0 {
		t.Fatalf("plan.limitations = %v, want empty", plan.limitations)
	}
}

func requireSynthesisOptions(t *testing.T, plan *synthesisPlan) *synthesizeOptions {
	t.Helper()

	if plan.options == nil {
		t.Fatal("plan.options = nil, want non-nil")
	}
	return plan.options
}

func assertSynthesisPlanOptions(t *testing.T, options *synthesizeOptions, want synthesisPlanExpectation) {
	t.Helper()

	if options.Text != want.text {
		t.Fatalf("plan.options.Text = %q, want %q", options.Text, want.text)
	}
	if options.Voice != want.voiceID {
		t.Fatalf("plan.options.Voice = %q, want %q", options.Voice, want.voiceID)
	}
	if options.OutputFormat != want.outputFormat {
		t.Fatalf("plan.options.OutputFormat = %q, want %q", options.OutputFormat, want.outputFormat)
	}
}

func assertSynthesisPlanBoundaryFlags(t *testing.T, options *synthesizeOptions, wantBoundaryEvents bool) {
	t.Helper()

	if options.WordBoundaryEnabled != wantBoundaryEvents || options.SentenceBoundaryEnabled != wantBoundaryEvents {
		t.Fatalf("boundary flags = (%v, %v), want %v", options.WordBoundaryEnabled, options.SentenceBoundaryEnabled, wantBoundaryEvents)
	}
}

func assertSynthesisPlanProsody(t *testing.T, options *synthesizeOptions, want synthesisPlanExpectation) {
	t.Helper()

	if options.Rate != want.rate || options.Volume != want.volume || options.Pitch != want.pitch {
		t.Fatalf("prosody = (%v, %v, %v), want (%v, %v, %v)", options.Rate, options.Volume, options.Pitch, want.rate, want.volume, want.pitch)
	}
}

func assertSynthesisPlanProfile(t *testing.T, plan *synthesisPlan, want synthesisPlanExpectation) {
	t.Helper()

	if plan.nativeProfile.Format != want.nativeFormat {
		t.Fatalf("plan.nativeProfile.Format = %q, want %q", plan.nativeProfile.Format, want.nativeFormat)
	}
}

func TestPrepareSynthesisRejectsUnsupportedOutputFormatsDirect(t *testing.T) {
	provider := New()

	for _, format := range []string{tts.AudioFormatPCM, tts.AudioFormatWAV} {
		t.Run(format, func(t *testing.T) {
			_, err := provider.prepareSynthesis(tts.SynthesisRequest{
				InputMode:    tts.InputModePlainText,
				Text:         "unsupported format",
				OutputFormat: format,
			})
			if err == nil {
				t.Fatalf("expected format %q to fail", format)
			}

			var ttsErr *tts.Error
			if !errors.As(err, &ttsErr) {
				t.Fatalf("error type = %T, want *tts.Error", err)
			}
			if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
				t.Fatalf("error code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
			}
		})
	}
}

func TestPrepareSynthesisRejectsRawSSMLDirect(t *testing.T) {
	provider := New()

	_, err := provider.prepareSynthesis(tts.SynthesisRequest{
		InputMode: tts.InputModeRawSSML,
		SSML:      "<speak>hello</speak>",
	})
	if err == nil {
		t.Fatal("expected raw SSML request to fail")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
		t.Fatalf("error code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
	}
}

type buildResultExpectation struct {
	wantAudio      []byte
	wantFormat     string
	wantSampleRate int
	wantProfile    tts.VoiceAudioProfile
}

func TestBuildResultDirect_MetadataMatchesMP3Audio(t *testing.T) {
	mp3Profile := mustParseOutputFormat(t, defaultOutputFormat)
	audio := bytes.Repeat([]byte{0x11}, 6000)
	plan := &synthesisPlan{
		voiceID:       "edge-voice",
		finalFormat:   tts.AudioFormatMP3,
		nativeProfile: mp3Profile,
	}

	assertBuildResultMatches(t, audio, plan, []tts.BoundaryEvent{{Type: tts.BoundaryTypeWord, Text: "hello"}}, buildResultExpectation{
		wantAudio:      audio,
		wantFormat:     tts.AudioFormatMP3,
		wantSampleRate: mp3Profile.SampleRate,
		wantProfile:    mp3Profile,
	})
}

func mustParseOutputFormat(t *testing.T, format string) tts.VoiceAudioProfile {
	t.Helper()

	profile, ok := ParseOutputFormat(format)
	if !ok {
		t.Fatalf("ParseOutputFormat(%q) = false, want true", format)
	}
	return profile
}

func assertBuildResultMatches(t *testing.T, audio []byte, plan *synthesisPlan, events []tts.BoundaryEvent, want buildResultExpectation) {
	t.Helper()

	provider := New()
	result, err := provider.buildResult(audio, plan, events)
	if err != nil {
		t.Fatalf("buildResult() error: %v", err)
	}
	if !bytes.Equal(result.Audio, want.wantAudio) {
		t.Fatalf("result.Audio length = %d, want %d with identical bytes", len(result.Audio), len(want.wantAudio))
	}
	if result.Format != want.wantFormat {
		t.Fatalf("result.Format = %q, want %q", result.Format, want.wantFormat)
	}
	if result.SampleRate != want.wantSampleRate {
		t.Fatalf("result.SampleRate = %d, want %d", result.SampleRate, want.wantSampleRate)
	}

	wantDuration, err := tts.InferDuration(result.Audio, want.wantProfile)
	if err != nil {
		t.Fatalf("InferDuration() error: %v", err)
	}
	if result.Duration != wantDuration {
		t.Fatalf("result.Duration = %v, want %v", result.Duration, wantDuration)
	}
	if result.Provider != providerName {
		t.Fatalf("result.Provider = %q, want %q", result.Provider, providerName)
	}
	if result.VoiceID != plan.voiceID {
		t.Fatalf("result.VoiceID = %q, want %q", result.VoiceID, plan.voiceID)
	}
	if !reflect.DeepEqual(result.BoundaryEvents, events) {
		t.Fatalf("result.BoundaryEvents = %v, want %v", result.BoundaryEvents, events)
	}
	if !reflect.DeepEqual(result.Limitations, plan.limitations) {
		t.Fatalf("result.Limitations = %v, want %v", result.Limitations, plan.limitations)
	}
}
