package edgetts

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

// TestFormatProsody 测试 rate/volume 格式化函数
func TestFormatProsody(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"默认值 (1.0)", 1.0, "+0%"},
		{"零值", 0, "+0%"},
		{"提高 50%", 1.5, "+50%"},
		{"提高 100% (翻倍)", 2.0, "+100%"},
		{"降低 50%", 0.5, "-50%"},
		{"降低 20%", 0.8, "-20%"},
		{"提高 33%", 1.33, "+33%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProsody(tt.value)
			if result != tt.expected {
				t.Errorf("formatProsody(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

// TestFormatPitch 测试 pitch 格式化函数
func TestFormatPitch(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"默认值 (1.0)", 1.0, "+0Hz"},
		{"零值", 0, "+0Hz"},
		{"提高 50%", 1.5, "+50%"},
		{"降低 50%", 0.5, "-50%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPitch(tt.value)
			if result != tt.expected {
				t.Errorf("formatPitch(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestProviderSynthesize_NilOptions(t *testing.T) {
	provider := New()

	_, err := provider.Synthesize(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesizeStream_NilOptions(t *testing.T) {
	provider := New()

	_, err := provider.SynthesizeStream(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesize_EmptyAfterNormalization(t *testing.T) {
	provider := New()

	_, err := provider.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  " \t\n ",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err == nil {
		t.Fatal("expected error for empty normalized text")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesizeStream_EmptyAfterNormalization(t *testing.T) {
	provider := New()

	_, err := provider.SynthesizeStream(context.Background(), &SynthesizeOptions{
		Text:  "\n\t ",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err == nil {
		t.Fatal("expected error for empty normalized text")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

// --- VoiceCache integration tests ---

// mockVoiceEntries returns a small set of voiceListEntry for unit testing.
func mockVoiceEntries() []voiceListEntry {
	return []voiceListEntry{
		{
			Name:      "Edge Voice",
			ShortName: "en-US-TestNeural",
			Gender:    "Female",
			Locale:    "en-US",
			Status:    "GA",
		},
		{
			Name:      "Chinese Voice",
			ShortName: "zh-CN-XiaoxiaoNeural",
			Gender:    "Female",
			Locale:    "zh-CN",
			Status:    "GA",
		},
	}
}

func TestWithVoiceCache_ListVoicesUsesCache(t *testing.T) {
	var fetchCount atomic.Int32

	p := New()
	p.voiceCache = tts.NewVoiceCache(func(ctx context.Context) ([]tts.Voice, error) {
		fetchCount.Add(1)
		return filterAndConvertVoices(mockVoiceEntries(), ""), nil
	})

	ctx := context.Background()

	// First call: should trigger fetcher
	voices, err := p.ListVoices(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected 1 fetch call, got %d", fetchCount.Load())
	}

	// Second call: should hit cache, no additional fetch
	voices2, err := p.ListVoices(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices2) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices2))
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected still 1 fetch call, got %d", fetchCount.Load())
	}
}

func TestWithVoiceCache_ListVoicesFiltersLocale(t *testing.T) {
	p := New()
	p.voiceCache = tts.NewVoiceCache(func(ctx context.Context) ([]tts.Voice, error) {
		return filterAndConvertVoices(mockVoiceEntries(), ""), nil
	})

	ctx := context.Background()
	voices, err := p.ListVoices(ctx, "zh-CN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 1 {
		t.Fatalf("expected 1 voice for zh-CN, got %d", len(voices))
	}
	if voices[0].ID != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("expected zh-CN-XiaoxiaoNeural, got %s", voices[0].ID)
	}
}

func TestWithVoiceCache_BuilderReturnsSelf(t *testing.T) {
	p := New().WithVoiceCache()
	if p.voiceCache == nil {
		t.Fatal("expected voiceCache to be set after WithVoiceCache()")
	}
}

func TestWithVoiceCache_DefaultTTLIs24Hours(t *testing.T) {
	p := New().WithVoiceCache()
	t.Cleanup(p.Close)

	if p.voiceCache == nil {
		t.Fatal("expected voiceCache to be initialized")
	}

	ttlField := reflect.ValueOf(p.voiceCache).Elem().FieldByName("ttl")
	if !ttlField.IsValid() {
		t.Fatal("voiceCache ttl field not found")
	}
	if ttlField.Kind() != reflect.Int64 {
		t.Fatalf("voiceCache ttl kind = %v; want Int64", ttlField.Kind())
	}

	if got := time.Duration(ttlField.Int()); got != 24*time.Hour {
		t.Fatalf("WithVoiceCache default ttl = %v; want %v", got, 24*time.Hour)
	}
}

func TestClose_Idempotent(t *testing.T) {
	p := New().WithVoiceCache()
	// Should not panic when called multiple times
	p.Close()
	p.Close()
}

func TestClose_NilCache(t *testing.T) {
	p := New()
	// Should not panic when no cache is configured
	p.Close()
}
