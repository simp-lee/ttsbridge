package tts

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
)

// defaultVoiceCacheTTL is the default time-to-live for cached voice lists.
const defaultVoiceCacheTTL = 24 * time.Hour

const (
	defaultBackgroundFetchTimeout = 10 * time.Second
	defaultVoiceCacheStopWaitTime = 150 * time.Millisecond
)

// ErrNilVoiceCacheFetcher indicates that VoiceCache was constructed
// with a nil fetcher function.
var ErrNilVoiceCacheFetcher = errors.New("tts: VoiceCache fetcher must not be nil")

// VoiceCache caches a Provider's voice list to reduce repeated API calls.
// It fetches the full list once and applies VoiceFilter on Get.
//
// Usage:
//
//	cache, err := tts.NewVoiceCache(myFetcher, tts.WithTTL(12*time.Hour))
//	voices, err := cache.Get(ctx, tts.VoiceFilter{Language: "zh-CN"})
type VoiceCache struct {
	mu        sync.RWMutex
	voices    []Voice
	hasData   bool
	fetchedAt time.Time
	ttl       time.Duration
	fetcher   func(ctx context.Context) ([]Voice, error)

	// background refresh
	bgInterval   time.Duration
	cancel       context.CancelFunc
	done         chan struct{}
	stopOnce     sync.Once
	stopErr      error
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
// Returns ErrNilVoiceCacheFetcher if fetcher is nil.
func NewVoiceCache(fetcher func(ctx context.Context) ([]Voice, error), opts ...VoiceCacheOption) (*VoiceCache, error) {
	if fetcher == nil {
		return nil, ErrNilVoiceCacheFetcher
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
		//nolint:gosec // VoiceCache stores the cancel func and invokes it from Stop to end background refresh.
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.done = make(chan struct{})
		go c.startBackgroundRefresh(ctx, c.bgInterval)
	}
	return c, nil
}

// Get returns the cached voice list filtered by the caller's filter.
//
// Behavior on fetch failure:
//   - If stale data exists, returns stale data with nil error (stale-while-revalidate).
//   - If no data exists, returns the fetch error.
func (c *VoiceCache) Get(ctx context.Context, filter VoiceFilter) ([]Voice, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Fast path: RLock check
	c.mu.RLock()
	if c.isValid() {
		voices := c.filterVoices(filter)
		c.mu.RUnlock()
		return voices, nil
	}
	c.mu.RUnlock()

	// Slow path: WLock with double-check
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isValid() {
		return c.filterVoices(filter), nil
	}

	// Fetch new data
	voices, err := c.fetcher(ctx)
	if err != nil {
		// Stale-while-revalidate: return stale data if available
		if c.hasData {
			return c.filterVoices(filter), nil
		}
		return nil, err
	}

	c.voices = cloneVoices(voices)
	c.hasData = true
	c.fetchedAt = time.Now()
	return c.filterVoices(filter), nil
}

// Stop terminates the background refresh goroutine, if any.
// It is safe to call multiple times.
// If the background goroutine does not stop within the configured wait window,
// Stop returns an explicit error instead of silently reporting success.
func (c *VoiceCache) Stop() error {
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
			if c.done != nil {
				select {
				case <-c.done:
				case <-time.After(c.stopWait):
					c.stopErr = errors.New("tts: voice cache background refresh did not stop before timeout")
				}
			}
		}
	})
	return c.stopErr
}

// isValid reports whether the cache is populated and not expired.
// Caller must hold at least RLock.
func (c *VoiceCache) isValid() bool {
	if !c.hasData {
		return false
	}
	return time.Since(c.fetchedAt) < c.ttl
}

// filterVoices applies the caller's voice filter to the cached inventory.
// Caller must hold at least RLock.
func (c *VoiceCache) filterVoices(filter VoiceFilter) []Voice { return FilterVoices(c.voices, filter) }

// FindCached looks up a voice by ID from the cache without triggering a fetch.
// Returns the voice and true if found, or a zero Voice and false if the cache
// is empty or the voice is not present.
func (c *VoiceCache) FindCached(voiceID string) (Voice, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.voices {
		if c.voices[i].ID == voiceID {
			return cloneVoice(c.voices[i]), true
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
			c.voices = cloneVoices(voices)
			c.hasData = true
			c.fetchedAt = time.Now()
			c.mu.Unlock()
		}
	}
}

func cloneVoices(voices []Voice) []Voice {
	if len(voices) == 0 {
		return nil
	}
	result := make([]Voice, len(voices))
	for i := range voices {
		result[i] = cloneVoice(voices[i])
	}
	return result
}

func cloneVoice(voice Voice) Voice {
	clone := voice
	if len(voice.Languages) > 0 {
		clone.Languages = append([]Language(nil), voice.Languages...)
	}
	if voice.Extra != nil {
		clone.Extra = deepCopyValue(reflect.ValueOf(voice.Extra)).Interface()
	}
	return clone
}

func deepCopyValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Pointer:
		return deepCopyPointer(value)
	case reflect.Interface:
		return deepCopyInterface(value)
	case reflect.Slice:
		return deepCopySlice(value)
	case reflect.Array:
		return deepCopyArray(value)
	case reflect.Map:
		return deepCopyMap(value)
	case reflect.Struct:
		return deepCopyStruct(value)
	default:
		return value
	}
}

func deepCopyPointer(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}

	copyPtr := reflect.New(value.Type().Elem())
	copyPtr.Elem().Set(deepCopyValue(value.Elem()))
	return copyPtr
}

func deepCopyInterface(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}

	copyElem := deepCopyValue(value.Elem())
	copyIface := reflect.New(value.Type()).Elem()
	copyIface.Set(copyElem)
	return copyIface
}

func deepCopySlice(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}

	copySlice := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
	for i := 0; i < value.Len(); i++ {
		copySlice.Index(i).Set(deepCopyValue(value.Index(i)))
	}
	return copySlice
}

func deepCopyArray(value reflect.Value) reflect.Value {
	copyArray := reflect.New(value.Type()).Elem()
	for i := 0; i < value.Len(); i++ {
		copyArray.Index(i).Set(deepCopyValue(value.Index(i)))
	}
	return copyArray
}

func deepCopyMap(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}

	copyMap := reflect.MakeMapWithSize(value.Type(), value.Len())
	for _, key := range value.MapKeys() {
		copyMap.SetMapIndex(deepCopyValue(key), deepCopyValue(value.MapIndex(key)))
	}
	return copyMap
}

func deepCopyStruct(value reflect.Value) reflect.Value {
	copyStruct := reflect.New(value.Type()).Elem()
	copyStruct.Set(value)
	for i := 0; i < value.NumField(); i++ {
		if !copyStruct.Field(i).CanSet() {
			continue
		}
		copyStruct.Field(i).Set(deepCopyValue(value.Field(i)))
	}
	return copyStruct
}
