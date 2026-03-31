package edgetts

import (
	"fmt"
	"strings"

	"github.com/simp-lee/ttsbridge/tts"
)

// Live Edge currently rejects both raw-24khz-16bit-mono-pcm and
// riff-24khz-16bit-mono-pcm, so the shared caller-facing contract only
// advertises mp3.
var edgeProviderCapabilities = tts.ProviderCapabilities{
	RawSSML:              false,
	ProsodyParams:        true,
	PlainTextOnly:        false,
	BoundaryEvents:       true,
	Streaming:            true,
	SupportedFormats:     []string{tts.AudioFormatMP3},
	PreferredAudioFormat: tts.AudioFormatMP3,
}

type synthesisPlan struct {
	options       *synthesizeOptions
	voiceID       string
	finalFormat   string
	nativeProfile tts.VoiceAudioProfile
	limitations   []string
}

func (p *Provider) Capabilities() tts.ProviderCapabilities {
	return edgeProviderCapabilities.Clone()
}

func newSynthesizeOptions() *synthesizeOptions {
	return &synthesizeOptions{
		Rate:   1.0,
		Volume: 1.0,
		Pitch:  1.0,
	}
}

func (p *Provider) prepareSynthesis(request tts.SynthesisRequest) (*synthesisPlan, error) {
	capabilities := p.Capabilities()
	if err := request.ValidateAgainst(providerName, capabilities); err != nil {
		return nil, err
	}

	voiceID := strings.TrimSpace(request.VoiceID)
	if voiceID == "" {
		voiceID = defaultVoice
	}

	finalFormat, nativeFormat, nativeProfile, err := p.resolveUnifiedOutputFormat(request.ResolvedOutputFormat(capabilities))
	if err != nil {
		return nil, err
	}

	options := newSynthesizeOptions()
	options.Voice = voiceID
	options.OutputFormat = nativeFormat
	options.WordBoundaryEnabled = request.NeedBoundaryEvents
	options.SentenceBoundaryEnabled = request.NeedBoundaryEvents

	switch request.InputMode {
	case tts.InputModePlainText:
		options.Text = request.Text
	case tts.InputModePlainTextWithProsody:
		options.Text = request.Text
		options.Rate = defaultProsodyValue(request.Prosody.HasRate(), request.Prosody.Rate)
		options.Volume = defaultProsodyValue(request.Prosody.HasVolume(), request.Prosody.Volume)
		options.Pitch = defaultProsodyValue(request.Prosody.HasPitch(), request.Prosody.Pitch)
	default:
		return nil, &tts.Error{Code: tts.ErrCodeUnsupportedCapability, Message: "raw SSML is not supported by provider", Provider: providerName}
	}

	plan := &synthesisPlan{
		options:       options,
		voiceID:       voiceID,
		finalFormat:   finalFormat,
		nativeProfile: nativeProfile,
	}

	return plan, nil
}

func defaultProsodyValue(hasValue bool, value float64) float64 {
	if !hasValue {
		return 1.0
	}
	return value
}

func (p *Provider) resolveUnifiedOutputFormat(requested string) (finalFormat string, nativeFormat string, nativeProfile tts.VoiceAudioProfile, err error) {
	switch requested {
	case "", tts.AudioFormatMP3:
		finalFormat = tts.AudioFormatMP3
		nativeFormat = defaultOutputFormat
	default:
		return "", "", tts.VoiceAudioProfile{}, &tts.Error{
			Code:     tts.ErrCodeUnsupportedFormat,
			Message:  fmt.Sprintf("output format %q is not supported by provider", requested),
			Provider: providerName,
		}
	}

	nativeProfile, ok := ParseOutputFormat(nativeFormat)
	if !ok {
		return "", "", tts.VoiceAudioProfile{}, &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  fmt.Sprintf("failed to resolve native audio profile for %q", nativeFormat),
			Provider: providerName,
		}
	}

	return finalFormat, nativeFormat, nativeProfile, nil
}

func (p *Provider) buildResult(audio []byte, plan *synthesisPlan, events []tts.BoundaryEvent) (*tts.SynthesisResult, error) {
	if plan == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "missing synthesis plan", Provider: providerName}
	}

	finalProfile := plan.nativeProfile
	finalProfile.Format = plan.finalFormat

	duration, err := tts.InferDuration(audio, finalProfile)
	if err != nil {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "failed to derive audio metadata", Provider: providerName, Err: err}
	}

	return &tts.SynthesisResult{
		Audio:          audio,
		Format:         plan.finalFormat,
		SampleRate:     finalProfile.SampleRate,
		Duration:       duration,
		Provider:       providerName,
		VoiceID:        plan.voiceID,
		BoundaryEvents: append([]tts.BoundaryEvent(nil), events...),
		Limitations:    append([]string(nil), plan.limitations...),
	}, nil
}
