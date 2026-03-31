package tts

import (
	"context"
	"errors"
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

// ErrProviderHealthStopTimeout indicates that Stop canceled the monitoring loop
// but an in-flight checker did not exit within the configured wait window.
var ErrProviderHealthStopTimeout = errors.New("tts: provider health check did not stop before timeout")

// ErrNilProviderHealthChecker indicates that ProviderHealth was constructed
// without a checker callback.
var ErrNilProviderHealthChecker = errors.New("tts: ProviderHealth checker must not be nil")

// ProviderHealth monitors provider availability via periodic health checks.
// It supports consecutive failure counting with cooldown and automatic recovery.
//
// Usage:
//
//	health, err := tts.NewProviderHealth(func(ctx context.Context) bool {
//	    // perform a lightweight check against the provider
//	    return provider.Ping(ctx) == nil
//	}, tts.WithCheckInterval(2*time.Minute), tts.WithMaxFails(5))
//	if err != nil {
//	    return err
//	}
//	if err := health.Start(ctx); err != nil {
//	    return err
//	}
//	defer func() {
//	    if err := health.Stop(); err != nil {
//	        log.Printf("provider health stop: %v", err)
//	    }
//	}()
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

	checker func(ctx context.Context) bool
	cancel  context.CancelFunc
	done    chan struct{}

	checkInFlight   bool
	checkerStuck    bool
	activeCheckDone <-chan struct{}
	timedOutCheck   <-chan struct{}
}

type providerHealthCheckRun struct {
	checker func(ctx context.Context) bool
	timeout time.Duration
	doneCh  chan struct{}
}

type providerHealthCheckOutcome struct {
	ok       bool
	timedOut bool
	stopped  bool
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
// The checker function should return true if the provider is healthy. The
// checker is expected to respect ctx cancellation promptly. A checker that
// times out is first treated as an ordinary failed check; it is only marked as
// terminally stuck if it is still running when a later scheduled probe arrives.
// A later Start first stops the old loop and returns
// [ErrProviderHealthStopTimeout] if that stuck checker still does not exit.
// The health monitor is initially considered healthy but does not start
// automatic checking until Start is called.
func NewProviderHealth(checker func(ctx context.Context) bool, opts ...ProviderHealthOption) (*ProviderHealth, error) {
	if checker == nil {
		return nil, ErrNilProviderHealthChecker
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
	return ph, nil
}

// Start begins periodic health checking in a background goroutine.
// It runs an immediate check, then checks at the configured interval.
// The goroutine respects both the provided ctx and Stop().
// Calling Start on an already-running health monitor stops the previous
// goroutine before starting a new one. If a previous checker ignored
// cancellation or is still draining after a timeout, Start returns
// [ErrProviderHealthStopTimeout] instead of silently pretending to restart.
func (ph *ProviderHealth) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ph.lifecycleMu.Lock()
	defer ph.lifecycleMu.Unlock()

	if err := ph.stopLocked(); err != nil {
		return err
	}

	ph.mu.Lock()
	ph.resetCheckStateLocked()
	ph.mu.Unlock()

	ctx, ph.cancel = context.WithCancel(ctx)
	ph.done = make(chan struct{})

	go ph.run(ctx, ph.done)
	return nil
}

func (ph *ProviderHealth) resetCheckStateLocked() {
	ph.checkInFlight = false
	ph.checkerStuck = false
	ph.activeCheckDone = nil
	ph.timedOutCheck = nil
}

// IsHealthy reports whether the provider is currently considered healthy.
func (ph *ProviderHealth) IsHealthy() bool {
	ph.mu.RLock()
	defer ph.mu.RUnlock()
	return ph.isHealthy
}

// Stop terminates the background health check goroutine and waits for it to exit.
// If an in-flight checker ignores cancellation and does not exit within the
// configured wait window, Stop returns [ErrProviderHealthStopTimeout] instead of
// silently pretending the monitor fully stopped.
// It is safe to call multiple times and safe to call without a preceding Start.
func (ph *ProviderHealth) Stop() error {
	ph.lifecycleMu.Lock()
	defer ph.lifecycleMu.Unlock()
	return ph.stopLocked()
}

func (ph *ProviderHealth) stopLocked() error {
	if ph.cancel != nil {
		ph.cancel()
	}
	if ph.done != nil {
		<-ph.done
	}

	err := ph.waitForInFlightCheckerLocked()
	ph.cancel = nil
	ph.done = nil
	return err
}

func (ph *ProviderHealth) waitForInFlightCheckerLocked() error {
	deadline := time.Now().Add(ph.stopWaitTime)
	for {
		ph.mu.RLock()
		inFlight := ph.checkInFlight
		activeDone := ph.activeCheckDone
		ph.mu.RUnlock()

		if !inFlight {
			return nil
		}
		if activeDone == nil {
			if !time.Now().Before(deadline) {
				return ErrProviderHealthStopTimeout
			}
			time.Sleep(1 * time.Millisecond)
			continue
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ErrProviderHealthStopTimeout
		}

		select {
		case <-activeDone:
			ph.mu.Lock()
			ph.clearActiveCheckLocked(activeDone)
			ph.mu.Unlock()
		case <-time.After(remaining):
			return ErrProviderHealthStopTimeout
		}
	}
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
	run, ok := ph.beginCheck()
	if !ok {
		return
	}

	outcome := ph.runCheck(ctx, run)
	ph.applyCheckOutcome(run.doneCh, outcome)
}

func (ph *ProviderHealth) beginCheck() (providerHealthCheckRun, bool) {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	if ph.checkerStuck {
		return providerHealthCheckRun{}, false
	}
	if ph.reconcileInFlightChecksLocked() {
		return providerHealthCheckRun{}, false
	}
	if ph.shouldSkipCheckLocked(time.Now()) {
		return providerHealthCheckRun{}, false
	}

	doneCh := make(chan struct{})
	ph.checkInFlight = true
	ph.activeCheckDone = doneCh
	return providerHealthCheckRun{checker: ph.checker, timeout: ph.checkTimeout, doneCh: doneCh}, true
}

func (ph *ProviderHealth) reconcileInFlightChecksLocked() bool {
	if activeDone := ph.activeCheckDone; activeDone != nil {
		if isClosed(activeDone) {
			ph.clearActiveCheckLocked(activeDone)
		} else if ph.timedOutCheck != nil {
			ph.markCheckerStuckLocked()
			return true
		}
	}
	if ph.checkInFlight && ph.timedOutCheck != nil {
		if isClosed(ph.timedOutCheck) {
			ph.clearTimedOutCheckLocked(ph.timedOutCheck)
		} else {
			ph.markCheckerStuckLocked()
			return true
		}
	}
	return false
}

func (ph *ProviderHealth) shouldSkipCheckLocked(now time.Time) bool {
	return now.Before(ph.cooldownUntil) || ph.checkInFlight
}

func (ph *ProviderHealth) markCheckerStuckLocked() {
	ph.isHealthy = false
	ph.failureCount = 0
	ph.cooldownUntil = time.Time{}
	ph.checkerStuck = true
}

func (ph *ProviderHealth) runCheck(ctx context.Context, run providerHealthCheckRun) providerHealthCheckOutcome {
	checkCtx, cancel := context.WithTimeout(ctx, run.timeout)
	defer cancel()

	resultCh := make(chan bool, 1)
	go func() {
		defer close(run.doneCh)
		ok := run.checker(checkCtx)
		select {
		case resultCh <- ok:
		default:
		}
	}()

	var outcome providerHealthCheckOutcome
	select {
	case outcome.ok = <-resultCh:
	case <-ctx.Done():
		outcome.stopped = true
	case <-checkCtx.Done():
		outcome.timedOut = checkCtx.Err() == context.DeadlineExceeded
		outcome.stopped = !outcome.timedOut
	}
	return outcome
}

func (ph *ProviderHealth) applyCheckOutcome(doneCh <-chan struct{}, outcome providerHealthCheckOutcome) {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	if outcome.stopped {
		ph.applyStoppedCheckLocked(doneCh)
		return
	}

	checkedAt := time.Now()
	ph.lastCheck = checkedAt
	if outcome.timedOut {
		if !isClosed(doneCh) {
			ph.timedOutCheck = doneCh
		} else {
			ph.clearActiveCheckLocked(doneCh)
		}
	} else if isClosed(doneCh) {
		ph.clearActiveCheckLocked(doneCh)
	}

	if outcome.ok {
		ph.failureCount = 0
		ph.isHealthy = true
		return
	}

	ph.failureCount++
	if ph.failureCount >= ph.maxFails {
		ph.isHealthy = false
		ph.cooldownUntil = checkedAt.Add(ph.cooldownTime)
		ph.failureCount = 0 // reset for next cycle after cooldown
	}
}

func (ph *ProviderHealth) applyStoppedCheckLocked(doneCh <-chan struct{}) {
	if isClosed(doneCh) {
		ph.clearActiveCheckLocked(doneCh)
	}
}

func (ph *ProviderHealth) clearTimedOutCheckLocked(done <-chan struct{}) {
	if ph.timedOutCheck != done {
		return
	}
	ph.timedOutCheck = nil
	ph.clearActiveCheckLocked(done)
}

func (ph *ProviderHealth) clearActiveCheckLocked(done <-chan struct{}) {
	if ph.activeCheckDone != done {
		return
	}
	ph.activeCheckDone = nil
	ph.checkInFlight = false
	if ph.timedOutCheck == done {
		ph.timedOutCheck = nil
	}
}

func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
