package tts_test

import (
	"context"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type mockOptions struct {
	Text  string
	Voice string
}

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Synthesize(ctx context.Context, opts *mockOptions) ([]byte, error) {
	return []byte("mock audio data"), nil
}

func (m *mockProvider) SynthesizeStream(ctx context.Context, opts *mockOptions) (tts.AudioStream, error) {
	return nil, nil
}

func (m *mockProvider) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return []tts.Voice{
		{ID: "mock-voice-1", Name: "Mock Voice 1", Language: tts.Language(locale), Gender: tts.GenderFemale, Provider: m.name},
	}, nil
}

func (m *mockProvider) IsAvailable(ctx context.Context) bool {
	return true
}

func (m *mockProvider) Close() error {
	return nil
}

func TestProviderInterface(t *testing.T) {
	provider := &mockProvider{name: "test"}

	// 测试基本属性
	if provider.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", provider.Name())
	}

	ctx := context.Background()

	// 测试 Synthesize
	opts := &mockOptions{Text: "test", Voice: "mock-voice-1"}
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if string(audio) != "mock audio data" {
		t.Errorf("Expected 'mock audio data', got '%s'", string(audio))
	}

	// 测试 ListVoices
	voices, err := provider.ListVoices(ctx, "en-US")
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) == 0 {
		t.Error("Expected at least one voice")
	}
	if voices[0].ID != "mock-voice-1" {
		t.Errorf("Expected voice ID 'mock-voice-1', got '%s'", voices[0].ID)
	}

	// 测试 IsAvailable
	if !provider.IsAvailable(ctx) {
		t.Error("Expected provider to be available")
	}
}
