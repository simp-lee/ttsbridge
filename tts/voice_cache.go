package tts

import (
	"context"
	"strings"
	"sync"
	"time"
)

// defaultVoiceCacheTTL is the default time-to-live for cached voice lists.
const defaultVoiceCacheTTL = 24 * time.Hour

const (
	defaultBackgroundFetchTimeout = 10 * time.Second
	defaultVoiceCacheStopWaitTime = 150 * time.Millisecond
)

// VoiceCache caches a Provider's voice list to reduce repeated API calls.
// It fetches the full list (all locales) once and filters by locale on Get.
//
// Usage:
//
//	cache := tts.NewVoiceCache(myFetcher, tts.WithTTL(12*time.Hour))
//	voices, err := cache.Get(ctx, "zh-CN")
type VoiceCache struct {
	mu        sync.RWMutex
	voices    []Voice
	fetchedAt time.Time
	ttl       time.Duration
	fetcher   func(ctx context.Context) ([]Voice, error)

	// background refresh
	bgInterval   time.Duration
	cancel       context.CancelFunc
	done         chan struct{}
	stopOnce     sync.Once
	stopWait     time.Duration
	fetchTimeout time.Duration
}

// VoiceCacheOption configures a VoiceCache.
type VoiceCacheOption func(*VoiceCache)

// WithTTL sets the cache time-to-live. Default is 24 hours.
func WithTTL(d time.Duration) VoiceCacheOption {
	return func(c *VoiceCache) {
		if d > 0 {
			c.ttl = d
		}
	}
}

// WithBackgroundRefresh enables a background goroutine that refreshes
// the cache at the given interval. The goroutine is started at the end
// of NewVoiceCache, after all options are applied. Call Stop() to terminate it.
func WithBackgroundRefresh(interval time.Duration) VoiceCacheOption {
	return func(c *VoiceCache) {
		c.bgInterval = interval
	}
}

// NewVoiceCache creates a VoiceCache with the given fetcher and options.
// The fetcher should return the full voice list (all locales).
func NewVoiceCache(fetcher func(ctx context.Context) ([]Voice, error), opts ...VoiceCacheOption) *VoiceCache {
	if fetcher == nil {
		panic("tts: VoiceCache fetcher must not be nil")
	}

	c := &VoiceCache{
		ttl:          defaultVoiceCacheTTL,
		fetcher:      fetcher,
		stopWait:     defaultVoiceCacheStopWaitTime,
		fetchTimeout: defaultBackgroundFetchTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.bgInterval > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.done = make(chan struct{})
		go c.startBackgroundRefresh(ctx, c.bgInterval)
	}
	return c
}

// Get returns the cached voice list, filtered by locale.
// An empty locale returns all voices.
//
// Behavior on fetch failure:
//   - If stale data exists, returns stale data with nil error (stale-while-revalidate).
//   - If no data exists, returns the fetch error.
func (c *VoiceCache) Get(ctx context.Context, locale string) ([]Voice, error) {
	// Fast path: RLock check
	c.mu.RLock()
	if c.isValid() {
		voices := c.filterByLocale(locale)
		c.mu.RUnlock()
		return voices, nil
	}
	c.mu.RUnlock()

	// Slow path: WLock with double-check
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isValid() {
		return c.filterByLocale(locale), nil
	}

	// Fetch new data
	voices, err := c.fetcher(ctx)
	if err != nil {
		// Stale-while-revalidate: return stale data if available
		if len(c.voices) > 0 {
			return c.filterByLocale(locale), nil
		}
		return nil, err
	}

	c.voices = voices
	c.fetchedAt = time.Now()
	return c.filterByLocale(locale), nil
}

// Stop terminates the background refresh goroutine, if any.
// It is safe to call multiple times.
func (c *VoiceCache) Stop() {
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
			if c.done != nil {
				select {
				case <-c.done:
				case <-time.After(c.stopWait):
				}
			}
		}
	})
}

// isValid reports whether the cache is populated and not expired.
// Caller must hold at least RLock.
func (c *VoiceCache) isValid() bool {
	if len(c.voices) == 0 {
		return false
	}
	return time.Since(c.fetchedAt) < c.ttl
}

// filterByLocale returns voices matching the locale.
// Empty locale returns all voices.
// Matches against Voice.Language and Voice.Languages using case-insensitive comparison.
// Caller must hold at least RLock.
func (c *VoiceCache) filterByLocale(locale string) []Voice {
	if locale == "" {
		// Return a copy to avoid data races on the slice
		result := make([]Voice, len(c.voices))
		copy(result, c.voices)
		return result
	}

	var result []Voice
	for i := range c.voices {
		v := &c.voices[i]
		if strings.EqualFold(string(v.Language), locale) {
			result = append(result, *v)
			continue
		}
		for _, lang := range v.Languages {
			if strings.EqualFold(string(lang), locale) {
				result = append(result, *v)
				break
			}
		}
	}
	return result
}

// FindCached looks up a voice by ID from the cache without triggering a fetch.
// Returns the voice and true if found, or a zero Voice and false if the cache
// is empty or the voice is not present.
func (c *VoiceCache) FindCached(voiceID string) (Voice, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.voices {
		if c.voices[i].ID == voiceID {
			return c.voices[i], true
		}
	}
	return Voice{}, false
}

// startBackgroundRefresh periodically fetches the voice list in the background.
func (c *VoiceCache) startBackgroundRefresh(ctx context.Context, interval time.Duration) {
	defer close(c.done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fetchCtx, cancel := context.WithTimeout(ctx, c.fetchTimeout)
			voices, err := c.fetcher(fetchCtx)
			cancel()
			if err != nil {
				// Silently keep stale data on background refresh failure
				continue
			}
			c.mu.Lock()
			c.voices = voices
			c.fetchedAt = time.Now()
			c.mu.Unlock()
		}
	}
}
