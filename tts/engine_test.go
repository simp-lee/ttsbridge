package tts_test

import (
	"context"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

// mockProvider 模拟提供商用于测试
type mockProvider struct {
	name      string
	available bool
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Synthesize(ctx context.Context, opts *tts.SynthesizeOptions) ([]byte, error) {
	return []byte("mock audio data"), nil
}

func (m *mockProvider) SynthesizeStream(ctx context.Context, opts *tts.SynthesizeOptions) (tts.AudioStream, error) {
	return nil, nil
}

func (m *mockProvider) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return []tts.Voice{
		{
			ID:       "test-voice",
			Name:     "Test Voice",
			Provider: m.name,
		},
	}, nil
}

func (m *mockProvider) IsAvailable(ctx context.Context) bool {
	return m.available
}

func TestNewEngine(t *testing.T) {
	provider := &mockProvider{name: "test", available: true}
	engine, err := tts.NewEngine(provider)

	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if engine == nil {
		t.Fatal("engine should not be nil")
	}

	providers := engine.ListProviders()
	if len(providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(providers))
	}
}

func TestEngineRegisterProvider(t *testing.T) {
	provider1 := &mockProvider{name: "test1", available: true}
	provider2 := &mockProvider{name: "test2", available: true}

	engine, err := tts.NewEngine(provider1)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = engine.RegisterProvider(provider2)
	if err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	providers := engine.ListProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}

	// 测试 nil provider
	err = engine.RegisterProvider(nil)
	if err == nil {
		t.Error("should return error for nil provider")
	}
}

func TestEngineSynthesize(t *testing.T) {
	provider := &mockProvider{name: "test", available: true}
	engine, err := tts.NewEngine(provider)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	opts := &tts.SynthesizeOptions{
		Text:   "Hello World",
		Voice:  "test-voice",
		Rate:   1.0,
		Volume: 1.0,
		Pitch:  1.0,
	}

	ctx := context.Background()
	audio, err := engine.Synthesize(ctx, opts)
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}

	if len(audio) == 0 {
		t.Error("audio data should not be empty")
	}
}

func TestEngineSetDefault(t *testing.T) {
	provider1 := &mockProvider{name: "test1", available: true}
	provider2 := &mockProvider{name: "test2", available: true}

	engine, err := tts.NewEngine(provider1)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	err = engine.RegisterProvider(provider2)
	if err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	err = engine.SetDefault("test2")
	if err != nil {
		t.Errorf("set default failed: %v", err)
	}

	err = engine.SetDefault("nonexistent")
	if err == nil {
		t.Error("should return error for nonexistent provider")
	}
}

func TestEngineValidation(t *testing.T) {
	provider := &mockProvider{name: "test", available: true}
	engine, err := tts.NewEngine(provider)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	ctx := context.Background()

	// 测试空文本
	_, err = engine.Synthesize(ctx, &tts.SynthesizeOptions{
		Text: "",
	})
	if err == nil {
		t.Error("should return error for empty text")
	}

	// 测试 nil options
	_, err = engine.Synthesize(ctx, nil)
	if err == nil {
		t.Error("should return error for nil options")
	}
}
