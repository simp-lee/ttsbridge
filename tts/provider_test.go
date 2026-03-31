package tts_test

import (
	"context"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Capabilities() tts.ProviderCapabilities {
	return tts.ProviderCapabilities{
		ProsodyParams:        true,
		SupportedFormats:     []string{tts.AudioFormatMP3},
		PreferredAudioFormat: tts.AudioFormatMP3,
	}
}

func (m *mockProvider) Synthesize(ctx context.Context, req tts.SynthesisRequest) (*tts.SynthesisResult, error) {
	return &tts.SynthesisResult{
		Audio:    []byte("mock audio data"),
		Format:   tts.AudioFormatMP3,
		Provider: m.name,
		VoiceID:  req.VoiceID,
	}, nil
}

func (m *mockProvider) SynthesizeStream(ctx context.Context, req tts.SynthesisRequest) (tts.AudioStream, error) {
	return nil, nil
}

func (m *mockProvider) ListVoices(ctx context.Context, filter tts.VoiceFilter) ([]tts.Voice, error) {
	return []tts.Voice{
		{ID: "mock-voice-1", Name: "Mock Voice 1", Language: tts.Language(filter.Language), Gender: tts.GenderFemale, Provider: m.name},
	}, nil
}

func TestProviderInterface(t *testing.T) {
	var provider tts.Provider = &mockProvider{name: "test"}

	// 测试基本属性
	if provider.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", provider.Name())
	}

	capabilities := provider.Capabilities()
	if !capabilities.ProsodyParams {
		t.Error("Expected prosody support")
	}
	if capabilities.PreferredAudioFormat != tts.AudioFormatMP3 {
		t.Errorf("Expected preferred format '%s', got '%s'", tts.AudioFormatMP3, capabilities.PreferredAudioFormat)
	}

	ctx := context.Background()

	// 测试 Synthesize
	request := tts.SynthesisRequest{InputMode: tts.InputModePlainText, Text: "test", VoiceID: "mock-voice-1"}
	result, err := provider.Synthesize(ctx, request)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if string(result.Audio) != "mock audio data" {
		t.Errorf("Expected 'mock audio data', got '%s'", string(result.Audio))
	}

	stream, err := provider.SynthesizeStream(ctx, request)
	if err != nil {
		t.Fatalf("SynthesizeStream failed: %v", err)
	}
	if stream != nil {
		t.Error("Expected nil stream")
	}

	// 测试 ListVoices
	voices, err := provider.ListVoices(ctx, tts.VoiceFilter{Language: "en-US"})
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) == 0 {
		t.Error("Expected at least one voice")
	}
	if voices[0].ID != "mock-voice-1" {
		t.Errorf("Expected voice ID 'mock-voice-1', got '%s'", voices[0].ID)
	}
}
