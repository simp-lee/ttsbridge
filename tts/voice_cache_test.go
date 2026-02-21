package tts

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testVoices returns a fixed set of voices for testing.
func testVoices() []Voice {
	return []Voice{
		{ID: "zh-CN-XiaoxiaoNeural", Name: "Xiaoxiao", Language: "zh-CN", Gender: GenderFemale, Provider: "edgetts"},
		{ID: "zh-CN-YunxiNeural", Name: "Yunxi", Language: "zh-CN", Gender: GenderMale, Provider: "edgetts"},
		{ID: "en-US-JennyNeural", Name: "Jenny", Language: "en-US", Gender: GenderFemale, Provider: "edgetts"},
		{ID: "ja-JP-NanamiNeural", Name: "Nanami", Language: "ja-JP", Languages: []Language{"ja-JP", "en-US"}, Gender: GenderFemale, Provider: "edgetts"},
	}
}

func TestVoiceCache_FirstFetch(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher)
	voices, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 4 {
		t.Errorf("got %d voices, want 4", len(voices))
	}
	if calls.Load() != 1 {
		t.Errorf("fetcher called %d times, want 1", calls.Load())
	}
}

func TestVoiceCache_CacheHit(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher)

	// First call populates cache
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should hit cache
	voices, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 4 {
		t.Errorf("got %d voices, want 4", len(voices))
	}
	if calls.Load() != 1 {
		t.Errorf("fetcher called %d times, want 1", calls.Load())
	}
}

func TestVoiceCache_TTLExpiry(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher, WithTTL(20*time.Millisecond))

	// First call
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("fetcher called %d times after first Get, want 1", calls.Load())
	}

	// Wait for TTL to expire
	time.Sleep(30 * time.Millisecond)

	// Should trigger re-fetch
	_, err = cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("fetcher called %d times after TTL expiry, want 2", calls.Load())
	}
}

func TestVoiceCache_StaleOnError(t *testing.T) {
	var calls atomic.Int32
	fetchErr := errors.New("network error")
	fetcher := func(ctx context.Context) ([]Voice, error) {
		n := calls.Add(1)
		if n == 1 {
			return testVoices(), nil
		}
		return nil, fetchErr
	}

	cache := NewVoiceCache(fetcher, WithTTL(10*time.Millisecond))

	// First call succeeds
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Second call fails but should return stale data
	voices, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("expected nil error with stale data, got: %v", err)
	}
	if len(voices) != 4 {
		t.Errorf("expected stale data with 4 voices, got %d", len(voices))
	}
}

func TestVoiceCache_NoDataOnError(t *testing.T) {
	fetchErr := errors.New("network error")
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return nil, fetchErr
	}

	cache := NewVoiceCache(fetcher)

	_, err := cache.Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when no data and fetch fails")
	}
	if !errors.Is(err, fetchErr) {
		t.Errorf("got error %v, want %v", err, fetchErr)
	}
}

func TestVoiceCache_LocaleFilter(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher)

	tests := []struct {
		name     string
		locale   string
		expected int
	}{
		{"all voices", "", 4},
		{"zh-CN exact", "zh-CN", 2},
		{"en-US exact", "en-US", 2}, // Jenny (Language) + Nanami (Languages includes en-US)
		{"ja-JP exact", "ja-JP", 1},
		{"case insensitive", "ZH-CN", 2},
		{"no match", "fr-FR", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voices, err := cache.Get(context.Background(), tt.locale)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(voices) != tt.expected {
				t.Errorf("got %d voices for locale %q, want %d", len(voices), tt.locale, tt.expected)
			}
		})
	}
}

func TestVoiceCache_ConcurrentAccess(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond) // simulate latency
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			voices, err := cache.Get(context.Background(), "")
			if err != nil {
				errs <- err
				return
			}
			if len(voices) != 4 {
				errs <- errors.New("unexpected voice count")
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Get error: %v", err)
	}

	// With double-check locking, fetcher should be called a small number of times
	// (ideally 1, but concurrent initial calls may cause a few more)
	if calls.Load() > 3 {
		t.Errorf("fetcher called %d times with 20 concurrent Gets, expected <= 3", calls.Load())
	}
}

func TestVoiceCache_BackgroundRefresh(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher, WithTTL(time.Hour), WithBackgroundRefresh(20*time.Millisecond))
	defer cache.Stop()

	// First call to populate
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for background refresh to trigger at least once
	time.Sleep(60 * time.Millisecond)

	if calls.Load() < 2 {
		t.Errorf("fetcher called %d times, expected at least 2 (initial + background)", calls.Load())
	}
}

func TestVoiceCache_Stop(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher, WithBackgroundRefresh(10*time.Millisecond))

	// Populate cache
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let background refresh run a bit
	time.Sleep(30 * time.Millisecond)
	cache.Stop()

	countAfterStop := calls.Load()

	// Wait and verify no more fetches happen
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != countAfterStop {
		t.Errorf("fetcher called after Stop: before=%d, after=%d", countAfterStop, calls.Load())
	}
}

func TestVoiceCache_StopIdempotent(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher, WithBackgroundRefresh(10*time.Millisecond))
	_, _ = cache.Get(context.Background(), "")

	// Multiple Stop calls should not panic
	cache.Stop()
	cache.Stop()
}

func TestVoiceCache_StopWithoutBackgroundRefresh(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := NewVoiceCache(fetcher)

	// Stop on a cache without background refresh should not panic
	cache.Stop()
}

func TestVoiceCache_Stop_BoundedWhenBackgroundFetcherIgnoresCancel(t *testing.T) {
	started := make(chan struct{}, 1)
	hang := make(chan struct{})

	fetcher := func(ctx context.Context) ([]Voice, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-hang
		return nil, nil
	}

	cache := NewVoiceCache(fetcher, WithBackgroundRefresh(10*time.Millisecond))

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background fetcher did not start in time")
	}

	stopped := make(chan struct{})
	go func() {
		cache.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// expected: Stop should be bounded even if fetcher hangs
	case <-time.After(400 * time.Millisecond):
		t.Fatal("Stop() blocked indefinitely with hanging background fetcher")
	}
}

func TestNewVoiceCache_NilFetcherPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil fetcher, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != "tts: VoiceCache fetcher must not be nil" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	NewVoiceCache(nil)
}

func TestWithTTL_IgnoresNonPositive(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cacheZero := NewVoiceCache(fetcher, WithTTL(0))
	if cacheZero.ttl != defaultVoiceCacheTTL {
		t.Errorf("ttl = %v, want default %v for zero input", cacheZero.ttl, defaultVoiceCacheTTL)
	}

	cacheNegative := NewVoiceCache(fetcher, WithTTL(-1*time.Second))
	if cacheNegative.ttl != defaultVoiceCacheTTL {
		t.Errorf("ttl = %v, want default %v for negative input", cacheNegative.ttl, defaultVoiceCacheTTL)
	}
}

func TestDefaultVoiceCacheTTL_Is24Hours(t *testing.T) {
	if defaultVoiceCacheTTL != 24*time.Hour {
		t.Fatalf("defaultVoiceCacheTTL = %v; want %v", defaultVoiceCacheTTL, 24*time.Hour)
	}
}

func TestNewVoiceCache_DefaultTTLIs24Hours(t *testing.T) {
	cache := NewVoiceCache(func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	})

	if cache.ttl != 24*time.Hour {
		t.Fatalf("cache.ttl = %v; want %v", cache.ttl, 24*time.Hour)
	}
}

func TestVoiceCache_FindCached_Empty(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}
	cache := NewVoiceCache(fetcher)

	// Before any Get, cache is empty
	_, ok := cache.FindCached("zh-CN-XiaoxiaoNeural")
	if ok {
		t.Error("FindCached should return false on empty cache")
	}
}

func TestVoiceCache_FindCached_Hit(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}
	cache := NewVoiceCache(fetcher)

	// Populate cache
	_, err := cache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	voice, ok := cache.FindCached("zh-CN-XiaoxiaoNeural")
	if !ok {
		t.Fatal("FindCached should return true for cached voice")
	}
	if voice.ID != "zh-CN-XiaoxiaoNeural" {
		t.Errorf("voice.ID = %q, want %q", voice.ID, "zh-CN-XiaoxiaoNeural")
	}
}

func TestVoiceCache_FindCached_Miss(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}
	cache := NewVoiceCache(fetcher)

	// Populate cache
	_, _ = cache.Get(context.Background(), "")

	_, ok := cache.FindCached("nonexistent-voice")
	if ok {
		t.Error("FindCached should return false for unknown voice ID")
	}
}
