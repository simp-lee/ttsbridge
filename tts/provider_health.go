package tts

import (
	"context"
	"sync"
	"time"
)

// Default health check configuration
const (
	defaultCheckInterval = 5 * time.Minute
	defaultMaxFails      = 3
	defaultCooldownTime  = 60 * time.Second
	defaultCheckTimeout  = 10 * time.Second
	defaultStopWaitTime  = 150 * time.Millisecond
)

// ProviderHealth monitors provider availability via periodic health checks.
// It supports consecutive failure counting with cooldown and automatic recovery.
//
// Usage:
//
//	health := tts.NewProviderHealth(func(ctx context.Context) bool {
//	    // perform a lightweight check against the provider
//	    return provider.Ping(ctx) == nil
//	}, tts.WithCheckInterval(2*time.Minute), tts.WithMaxFails(5))
//	health.Start(ctx)
//	defer health.Stop()
//
//	if health.IsHealthy() { ... }
type ProviderHealth struct {
	mu            sync.RWMutex
	lifecycleMu   sync.Mutex
	isHealthy     bool
	lastCheck     time.Time
	failureCount  int
	cooldownUntil time.Time

	checkInterval time.Duration
	maxFails      int
	cooldownTime  time.Duration
	checkTimeout  time.Duration
	stopWaitTime  time.Duration

	checker  func(ctx context.Context) bool
	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once

	checkInFlight bool
}

// ProviderHealthOption configures a ProviderHealth.
type ProviderHealthOption func(*ProviderHealth)

// WithCheckInterval sets the interval between health checks. Default is 5 minutes.
// Non-positive values are ignored to prevent time.NewTicker panics.
func WithCheckInterval(d time.Duration) ProviderHealthOption {
	return func(ph *ProviderHealth) {
		if d > 0 {
			ph.checkInterval = d
		}
	}
}

// WithMaxFails sets the consecutive failure threshold before entering cooldown. Default is 3.
func WithMaxFails(n int) ProviderHealthOption {
	return func(ph *ProviderHealth) {
		if n > 0 {
			ph.maxFails = n
		}
	}
}

// WithCooldownTime sets the cooldown duration after maxFails consecutive failures. Default is 60s.
func WithCooldownTime(d time.Duration) ProviderHealthOption {
	return func(ph *ProviderHealth) {
		if d > 0 {
			ph.cooldownTime = d
		}
	}
}

// WithCheckTimeout sets the maximum time for a single checker execution.
// If exceeded, the check is treated as a failure and no overlapping check is started.
func WithCheckTimeout(d time.Duration) ProviderHealthOption {
	return func(ph *ProviderHealth) {
		if d > 0 {
			ph.checkTimeout = d
		}
	}
}

// NewProviderHealth creates a ProviderHealth with the given checker function and options.
// The checker function should return true if the provider is healthy.
// The health monitor is initially considered healthy but does not start
// automatic checking until Start is called.
func NewProviderHealth(checker func(ctx context.Context) bool, opts ...ProviderHealthOption) *ProviderHealth {
	if checker == nil {
		panic("tts: ProviderHealth checker must not be nil")
	}
	ph := &ProviderHealth{
		isHealthy:     true,
		checkInterval: defaultCheckInterval,
		maxFails:      defaultMaxFails,
		cooldownTime:  defaultCooldownTime,
		checkTimeout:  defaultCheckTimeout,
		stopWaitTime:  defaultStopWaitTime,
		checker:       checker,
	}
	for _, opt := range opts {
		opt(ph)
	}
	return ph
}

// Start begins periodic health checking in a background goroutine.
// It runs an immediate check, then checks at the configured interval.
// The goroutine respects both the provided ctx and Stop().
// Calling Start on an already-running health monitor stops the previous
// goroutine before starting a new one.
func (ph *ProviderHealth) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	ph.lifecycleMu.Lock()
	defer ph.lifecycleMu.Unlock()

	ph.stopLocked() // ensure any previous goroutine is stopped

	ctx, ph.cancel = context.WithCancel(ctx)
	ph.done = make(chan struct{})
	ph.checkInFlight = false
	ph.stopOnce = sync.Once{} // reset for re-start safety

	go ph.run(ctx, ph.done)
}

// IsHealthy reports whether the provider is currently considered healthy.
func (ph *ProviderHealth) IsHealthy() bool {
	ph.mu.RLock()
	defer ph.mu.RUnlock()
	return ph.isHealthy
}

// Stop terminates the background health check goroutine and waits for it to exit.
// It is safe to call multiple times and safe to call without a preceding Start.
func (ph *ProviderHealth) Stop() {
	ph.lifecycleMu.Lock()
	defer ph.lifecycleMu.Unlock()
	ph.stopLocked()
}

func (ph *ProviderHealth) stopLocked() {
	ph.stopOnce.Do(func() {
		if ph.cancel != nil {
			ph.cancel()
		}
		if ph.done != nil {
			<-ph.done
			waitUntil := time.Now().Add(ph.stopWaitTime)
			for {
				ph.mu.RLock()
				inFlight := ph.checkInFlight
				ph.mu.RUnlock()
				if !inFlight || !time.Now().Before(waitUntil) {
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}
		ph.cancel = nil
		ph.done = nil
	})
}

// run is the main loop for the background health check goroutine.
func (ph *ProviderHealth) run(ctx context.Context, done chan struct{}) {
	defer close(done)

	// Immediate check on start
	ph.performCheck(ctx)

	ticker := time.NewTicker(ph.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ph.performCheck(ctx)
		}
	}
}

// performCheck executes a single health check and updates state accordingly.
func (ph *ProviderHealth) performCheck(ctx context.Context) {
	ph.mu.Lock()
	if time.Now().Before(ph.cooldownUntil) || ph.checkInFlight {
		ph.mu.Unlock()
		return
	}
	checker := ph.checker
	timeout := ph.checkTimeout
	ph.checkInFlight = true
	ph.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan bool, 1)
	go func() {
		ok := checker(checkCtx)
		select {
		case resultCh <- ok:
		default:
		}
		ph.mu.Lock()
		ph.checkInFlight = false
		ph.mu.Unlock()
	}()

	ok := false
	select {
	case ok = <-resultCh:
	case <-ctx.Done():
		return
	case <-checkCtx.Done():
		ok = false
	}

	ph.mu.Lock()
	defer ph.mu.Unlock()

	ph.lastCheck = time.Now()

	if ok {
		ph.failureCount = 0
		ph.isHealthy = true
		return
	}

	ph.failureCount++
	if ph.failureCount >= ph.maxFails {
		ph.isHealthy = false
		ph.cooldownUntil = time.Now().Add(ph.cooldownTime)
		ph.failureCount = 0 // reset for next cycle after cooldown
	}
}
