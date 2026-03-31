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

func testVoiceFilter(language string) VoiceFilter {
	if language == "" {
		return VoiceFilter{}
	}
	return VoiceFilter{Language: language}
}

func mustNewVoiceCache(t *testing.T, fetcher func(ctx context.Context) ([]Voice, error), opts ...VoiceCacheOption) *VoiceCache {
	t.Helper()
	cache, err := NewVoiceCache(fetcher, opts...)
	if err != nil {
		t.Fatalf("NewVoiceCache: unexpected error: %v", err)
	}
	return cache
}

func nilContextForVoiceCacheTest() context.Context {
	var ctx context.Context
	return ctx
}

func TestVoiceCache_FirstFetch(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)
	voices, err := cache.Get(context.Background(), testVoiceFilter(""))
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

	cache := mustNewVoiceCache(t, fetcher)

	// First call populates cache
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should hit cache
	voices, err := cache.Get(context.Background(), testVoiceFilter(""))
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

	cache := mustNewVoiceCache(t, fetcher, WithTTL(20*time.Millisecond))

	// First call
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("fetcher called %d times after first Get, want 1", calls.Load())
	}

	// Wait for TTL to expire
	time.Sleep(30 * time.Millisecond)

	// Should trigger re-fetch
	_, err = cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("fetcher called %d times after TTL expiry, want 2", calls.Load())
	}
}

func TestVoiceCache_EmptyFetchIsCachedUntilTTLExpiry(t *testing.T) {
	var calls atomic.Int32
	fetcher := func(ctx context.Context) ([]Voice, error) {
		calls.Add(1)
		return []Voice{}, nil
	}

	cache := mustNewVoiceCache(t, fetcher, WithTTL(20*time.Millisecond))

	voices, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("first Get() error: %v", err)
	}
	if len(voices) != 0 {
		t.Fatalf("first Get() returned %d voices, want 0", len(voices))
	}
	if calls.Load() != 1 {
		t.Fatalf("fetcher called %d times after first Get, want 1", calls.Load())
	}

	voices, err = cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("second Get() error: %v", err)
	}
	if len(voices) != 0 {
		t.Fatalf("second Get() returned %d voices, want 0", len(voices))
	}
	if calls.Load() != 1 {
		t.Fatalf("empty successful fetch should be cached, fetcher called %d times", calls.Load())
	}

	time.Sleep(30 * time.Millisecond)

	voices, err = cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("third Get() after TTL expiry error: %v", err)
	}
	if len(voices) != 0 {
		t.Fatalf("third Get() returned %d voices, want 0", len(voices))
	}
	if calls.Load() != 2 {
		t.Fatalf("fetcher called %d times after TTL expiry, want 2", calls.Load())
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

	cache := mustNewVoiceCache(t, fetcher, WithTTL(10*time.Millisecond))

	// First call succeeds
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Second call fails but should return stale data
	voices, err := cache.Get(context.Background(), testVoiceFilter(""))
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

	cache := mustNewVoiceCache(t, fetcher)

	_, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err == nil {
		t.Fatal("expected error when no data and fetch fails")
	}
	if !errors.Is(err, fetchErr) {
		t.Errorf("got error %v, want %v", err, fetchErr)
	}
}

func TestVoiceCache_NilContextUsesBackground(t *testing.T) {
	var fetchCtx context.Context
	fetcher := func(ctx context.Context) ([]Voice, error) {
		fetchCtx = ctx
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)

	voices, err := cache.Get(nilContextForVoiceCacheTest(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("Get(nilContextForVoiceCacheTest(), ...) error: %v", err)
	}
	if len(voices) != 4 {
		t.Fatalf("Get(nilContextForVoiceCacheTest(), ...) returned %d voices, want 4", len(voices))
	}
	if fetchCtx == nil {
		t.Fatal("fetcher received nil context")
	}
}

func TestVoiceCache_LocaleFilter(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)

	tests := []struct {
		name     string
		locale   string
		expected int
	}{
		{"all voices", "", 4},
		{"zh prefix", "zh", 2},
		{"zh-CN exact", "zh-CN", 2},
		{"en prefix", "en", 2},
		{"en-US exact", "en-US", 2}, // Jenny (Language) + Nanami (Languages includes en-US)
		{"ja-JP exact", "ja-JP", 1},
		{"case insensitive", "ZH-CN", 2},
		{"no match", "fr-FR", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voices, err := cache.Get(context.Background(), testVoiceFilter(tt.locale))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(voices) != tt.expected {
				t.Errorf("got %d voices for locale %q, want %d", len(voices), tt.locale, tt.expected)
			}
		})
	}
}

func TestVoiceCache_LocaleFilterRejectsInvalidLocalePrefixes(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)

	for _, locale := range []string{"e", "z", "en-", "zh-"} {
		t.Run(locale, func(t *testing.T) {
			voices, err := cache.Get(context.Background(), testVoiceFilter(locale))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(voices) != 0 {
				t.Fatalf("Get(%q) returned %d voices, want 0", locale, len(voices))
			}
		})
	}
}

func TestVoiceCache_LocaleFilterMatchesSupportsLanguageSemantics(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)
	voices := testVoices()
	locales := []string{"zh", "zh-CN", "en", "en-US", "ja", "ja-JP", "e", "en-", "zh-", "fr-FR"}

	for _, locale := range locales {
		t.Run(locale, func(t *testing.T) {
			got, err := cache.Get(context.Background(), testVoiceFilter(locale))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			wantIDs := make([]string, 0)
			for i := range voices {
				if voices[i].SupportsLanguage(locale) {
					wantIDs = append(wantIDs, voices[i].ID)
				}
			}

			if len(got) != len(wantIDs) {
				t.Fatalf("Get(%q) returned %d voices, want %d", locale, len(got), len(wantIDs))
			}
			for i, voice := range got {
				if voice.ID != wantIDs[i] {
					t.Fatalf("Get(%q) voice[%d]=%q, want %q", locale, i, voice.ID, wantIDs[i])
				}
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

	cache := mustNewVoiceCache(t, fetcher)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			voices, err := cache.Get(context.Background(), testVoiceFilter(""))
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

	cache := mustNewVoiceCache(t, fetcher, WithTTL(time.Hour), WithBackgroundRefresh(20*time.Millisecond))
	t.Cleanup(func() {
		if err := cache.Stop(); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}
	})

	// First call to populate
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
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

	cache := mustNewVoiceCache(t, fetcher, WithBackgroundRefresh(10*time.Millisecond))

	// Populate cache
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let background refresh run a bit
	time.Sleep(30 * time.Millisecond)
	if err := cache.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

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

	cache := mustNewVoiceCache(t, fetcher, WithBackgroundRefresh(10*time.Millisecond))
	_, _ = cache.Get(context.Background(), testVoiceFilter(""))

	// Multiple Stop calls should not panic
	if err := cache.Stop(); err != nil {
		t.Fatalf("first Stop() error: %v", err)
	}
	if err := cache.Stop(); err != nil {
		t.Fatalf("second Stop() error: %v", err)
	}
}

func TestVoiceCache_StopWithoutBackgroundRefresh(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cache := mustNewVoiceCache(t, fetcher)

	// Stop on a cache without background refresh should not panic
	if err := cache.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestVoiceCache_Stop_ReturnsErrorWhenBackgroundFetcherIgnoresCancel(t *testing.T) {
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

	cache := mustNewVoiceCache(t, fetcher, WithBackgroundRefresh(10*time.Millisecond))
	t.Cleanup(func() {
		close(hang)
		select {
		case <-cache.done:
		case <-time.After(200 * time.Millisecond):
			t.Errorf("background refresh goroutine did not exit after releasing fetcher")
		}
	})

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("background fetcher did not start in time")
	}

	stopped := make(chan struct{})
	var stopErr error
	go func() {
		stopErr = cache.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		if stopErr == nil {
			t.Fatal("expected Stop() to report timeout when background fetcher does not exit")
		}
	case <-time.After(400 * time.Millisecond):
		t.Fatal("Stop() blocked indefinitely with hanging background fetcher")
	}
}

func TestNewVoiceCache_NilFetcherReturnsError(t *testing.T) {
	cache, err := NewVoiceCache(nil)
	if err == nil {
		t.Fatal("expected error for nil fetcher, got nil")
	}
	if cache != nil {
		t.Fatal("expected nil cache for nil fetcher")
	}
	if !errors.Is(err, ErrNilVoiceCacheFetcher) {
		t.Errorf("expected ErrNilVoiceCacheFetcher, got: %v", err)
	}
}

func TestWithTTL_IgnoresNonPositive(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}

	cacheZero := mustNewVoiceCache(t, fetcher, WithTTL(0))
	if cacheZero.ttl != defaultVoiceCacheTTL {
		t.Errorf("ttl = %v, want default %v for zero input", cacheZero.ttl, defaultVoiceCacheTTL)
	}

	cacheNegative := mustNewVoiceCache(t, fetcher, WithTTL(-1*time.Second))
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
	cache := mustNewVoiceCache(t, func(ctx context.Context) ([]Voice, error) {
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
	cache := mustNewVoiceCache(t, fetcher)

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
	cache := mustNewVoiceCache(t, fetcher)

	// Populate cache
	_, err := cache.Get(context.Background(), testVoiceFilter(""))
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

func TestVoiceCache_ReturnsDeepCopies(t *testing.T) {
	voicesWithExtra := []Voice{
		{
			ID:        "zh-CN-XiaoxiaoNeural",
			Name:      "Xiaoxiao",
			Language:  "zh-CN",
			Languages: []Language{"zh-CN", "en-US"},
			Gender:    GenderFemale,
			Provider:  "edgetts",
			Extra: &testExtra{
				Status: "GA",
				Styles: []string{"cheerful"},
			},
		},
	}

	cache := mustNewVoiceCache(t, func(ctx context.Context) ([]Voice, error) {
		return voicesWithExtra, nil
	})

	got, err := cache.Get(context.Background(), testVoiceFilter("zh"))
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Get()) = %d, want 1", len(got))
	}

	got[0].Languages[0] = "fr-FR"
	extra, ok := GetExtra[*testExtra](&got[0])
	if !ok {
		t.Fatalf("GetExtra() failed for cloned voice")
	}
	extra.Status = "Preview"
	extra.Styles[0] = "sad"

	again, err := cache.Get(context.Background(), testVoiceFilter("zh"))
	if err != nil {
		t.Fatalf("second Get() error: %v", err)
	}
	if again[0].Languages[0] != "zh-CN" {
		t.Fatalf("cached Languages mutated: got %q, want %q", again[0].Languages[0], "zh-CN")
	}
	againExtra, ok := GetExtra[*testExtra](&again[0])
	if !ok {
		t.Fatalf("GetExtra() failed for second Get result")
	}
	if againExtra.Status != "GA" {
		t.Fatalf("cached Extra.Status mutated: got %q, want %q", againExtra.Status, "GA")
	}
	if againExtra.Styles[0] != "cheerful" {
		t.Fatalf("cached Extra.Styles mutated: got %q, want %q", againExtra.Styles[0], "cheerful")
	}

	fromFind, ok := cache.FindCached("zh-CN-XiaoxiaoNeural")
	if !ok {
		t.Fatal("FindCached() should return cloned voice")
	}
	findExtra, ok := GetExtra[*testExtra](&fromFind)
	if !ok {
		t.Fatalf("GetExtra() failed for FindCached result")
	}
	findExtra.Status = "Retired"

	finalVoice, ok := cache.FindCached("zh-CN-XiaoxiaoNeural")
	if !ok {
		t.Fatal("FindCached() should still find voice")
	}
	finalExtra, ok := GetExtra[*testExtra](&finalVoice)
	if !ok {
		t.Fatalf("GetExtra() failed for final FindCached result")
	}
	if finalExtra.Status != "GA" {
		t.Fatalf("FindCached exposed mutable Extra: got %q, want %q", finalExtra.Status, "GA")
	}
}

func TestVoiceCache_FindCached_Miss(t *testing.T) {
	fetcher := func(ctx context.Context) ([]Voice, error) {
		return testVoices(), nil
	}
	cache := mustNewVoiceCache(t, fetcher)

	// Populate cache
	_, _ = cache.Get(context.Background(), testVoiceFilter(""))

	_, ok := cache.FindCached("nonexistent-voice")
	if ok {
		t.Error("FindCached should return false for unknown voice ID")
	}
}
