package volcengine

import (
	"strings"

	"github.com/simp-lee/ttsbridge/tts"
)

var volcengineCapabilities = tts.ProviderCapabilities{
	RawSSML:              false,
	ProsodyParams:        false,
	PlainTextOnly:        true,
	BoundaryEvents:       false,
	Streaming:            false,
	SupportedFormats:     []string{tts.AudioFormatWAV},
	PreferredAudioFormat: tts.AudioFormatWAV,
}

func (p *Provider) Capabilities() tts.ProviderCapabilities {
	return volcengineCapabilities.Clone()
}

func (p *Provider) buildRequest(request tts.SynthesisRequest) (*synthesizeOptions, string, error) {
	if err := request.ValidateAgainst(providerName, p.Capabilities()); err != nil {
		return nil, "", err
	}

	voiceID := strings.TrimSpace(request.VoiceID)
	if voiceID == "" {
		voiceID = "BV700_streaming"
	}

	return &synthesizeOptions{
		Text:  request.Text,
		Voice: voiceID,
	}, voiceID, nil
}

func buildResult(audio []byte, voiceID string) (*tts.SynthesisResult, error) {
	_, _, profile, ok := parseCanonicalWAV(audio)
	if !ok {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "failed to derive audio metadata", Provider: providerName}
	}

	actualProfile := tts.VoiceAudioProfile{
		Format:     tts.AudioFormatWAV,
		SampleRate: int(profile.sampleRate),
		Channels:   int(profile.channels),
		Lossless:   true,
	}

	duration, err := tts.InferDuration(audio, actualProfile)
	if err != nil {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "failed to derive audio metadata", Provider: providerName, Err: err}
	}

	return &tts.SynthesisResult{
		Audio:      audio,
		Format:     tts.AudioFormatWAV,
		SampleRate: actualProfile.SampleRate,
		Duration:   duration,
		Provider:   providerName,
		VoiceID:    voiceID,
	}, nil
}
