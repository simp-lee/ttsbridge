package tts

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func nilContextForTest() context.Context {
	var ctx context.Context
	return ctx
}

func mustNewProviderHealth(t *testing.T, checker func(context.Context) bool, opts ...ProviderHealthOption) *ProviderHealth {
	t.Helper()
	ph, err := NewProviderHealth(checker, opts...)
	if err != nil {
		t.Fatalf("NewProviderHealth() unexpected error = %v", err)
	}
	return ph
}

func mustStartProviderHealth(t *testing.T, ph *ProviderHealth, ctx context.Context) {
	t.Helper()
	if err := ph.Start(ctx); err != nil {
		t.Fatalf("Start() unexpected error = %v", err)
	}
}

func mustStopProviderHealth(t *testing.T, ph *ProviderHealth) {
	t.Helper()
	if err := ph.Stop(); err != nil {
		t.Fatalf("Stop() unexpected error = %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func assertConditionStable(t *testing.T, duration time.Duration, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if !condition() {
			t.Fatalf("condition changed while waiting for %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNewProviderHealth_Defaults(t *testing.T) {
	ph := mustNewProviderHealth(t, func(ctx context.Context) bool { return true })

	if ph.checkInterval != 5*time.Minute {
		t.Errorf("checkInterval = %v, want 5m", ph.checkInterval)
	}
	if ph.maxFails != 3 {
		t.Errorf("maxFails = %d, want 3", ph.maxFails)
	}
	if ph.cooldownTime != 60*time.Second {
		t.Errorf("cooldownTime = %v, want 60s", ph.cooldownTime)
	}
	if ph.isHealthy != true {
		t.Error("initial isHealthy should be true")
	}
}

func TestNewProviderHealth_Options(t *testing.T) {
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCheckInterval(10*time.Second),
		WithMaxFails(5),
		WithCooldownTime(30*time.Second),
	)

	if ph.checkInterval != 10*time.Second {
		t.Errorf("checkInterval = %v, want 10s", ph.checkInterval)
	}
	if ph.maxFails != 5 {
		t.Errorf("maxFails = %d, want 5", ph.maxFails)
	}
	if ph.cooldownTime != 30*time.Second {
		t.Errorf("cooldownTime = %v, want 30s", ph.cooldownTime)
	}
}

func TestProviderHealth_IsHealthy_Default(t *testing.T) {
	ph := mustNewProviderHealth(t, func(ctx context.Context) bool { return true })
	if !ph.IsHealthy() {
		t.Error("IsHealthy() should return true by default")
	}
}

func TestProviderHealth_Start_ImmediateCheck(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(time.Hour), // long interval so only immediate check fires
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	// Give the goroutine a moment to run the immediate check
	time.Sleep(50 * time.Millisecond)

	if calls.Load() < 1 {
		t.Error("checker should be called at least once immediately on Start")
	}
	if !ph.IsHealthy() {
		t.Error("IsHealthy() should be true after successful check")
	}
}

func TestProviderHealth_FailureCountAndCooldown(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return false // always fail
		},
		WithCheckInterval(20*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(100*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	// Wait enough for immediate + 3 ticks (to hit maxFails)
	time.Sleep(120 * time.Millisecond)

	if ph.IsHealthy() {
		t.Error("IsHealthy() should be false after maxFails consecutive failures")
	}

	// Verify cooldown: read cooldownUntil
	ph.mu.RLock()
	inCooldown := time.Now().Before(ph.cooldownUntil)
	ph.mu.RUnlock()
	if !inCooldown {
		t.Error("should be in cooldown after maxFails failures")
	}
}

func TestProviderHealth_SuccessResetsFailureCount(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			n := calls.Add(1)
			// Fail first 2 calls, then succeed
			return n > 2
		},
		WithCheckInterval(20*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(time.Hour),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	// Wait for a few checks to complete
	time.Sleep(100 * time.Millisecond)

	if !ph.IsHealthy() {
		t.Error("IsHealthy() should be true after a successful check resets failure count")
	}
	ph.mu.RLock()
	fc := ph.failureCount
	ph.mu.RUnlock()
	if fc != 0 {
		t.Errorf("failureCount = %d, want 0 after success", fc)
	}
}

func TestProviderHealth_CooldownRecovery(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			n := calls.Add(1)
			// Fail first 4 calls (enough to enter cooldown), then succeed
			return n > 4
		},
		WithCheckInterval(15*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(60*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	// Wait for failures + cooldown entry
	time.Sleep(100 * time.Millisecond)

	// At this point should be in cooldown and unhealthy
	if ph.IsHealthy() {
		t.Error("should be unhealthy during cooldown")
	}

	// Wait for cooldown to expire and checks to resume
	time.Sleep(150 * time.Millisecond)

	// After cooldown, checker returns true → should recover
	if !ph.IsHealthy() {
		t.Error("should recover after cooldown expires and check succeeds")
	}
}

func TestProviderHealth_StopIdempotent(t *testing.T) {
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCheckInterval(time.Hour),
	)

	mustStartProviderHealth(t, ph, context.Background())

	// Multiple Stop calls should not panic
	mustStopProviderHealth(t, ph)
	mustStopProviderHealth(t, ph)
	mustStopProviderHealth(t, ph)
}

func TestProviderHealth_StopWithoutStart(t *testing.T) {
	ph := mustNewProviderHealth(t, func(ctx context.Context) bool { return true })

	// Stop without Start should not panic
	mustStopProviderHealth(t, ph)
	mustStopProviderHealth(t, ph)
}

func TestProviderHealth_StopCancelsChecker(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	time.Sleep(50 * time.Millisecond)
	mustStopProviderHealth(t, ph)

	countAfterStop := calls.Load()
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != countAfterStop {
		t.Error("checker should not be called after Stop")
	}
}

func TestNewProviderHealth_NilCheckerReturnsError(t *testing.T) {
	ph, err := NewProviderHealth(nil)
	if !errors.Is(err, ErrNilProviderHealthChecker) {
		t.Fatalf("NewProviderHealth() error = %v, want %v", err, ErrNilProviderHealthChecker)
	}
	if ph != nil {
		t.Fatalf("NewProviderHealth() returned %v, want nil ProviderHealth on error", ph)
	}
}

func TestWithCheckInterval_IgnoresNonPositive(t *testing.T) {
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCheckInterval(0),
	)
	if ph.checkInterval != defaultCheckInterval {
		t.Errorf("checkInterval = %v, want default %v for zero input", ph.checkInterval, defaultCheckInterval)
	}

	ph2 := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCheckInterval(-5*time.Second),
	)
	if ph2.checkInterval != defaultCheckInterval {
		t.Errorf("checkInterval = %v, want default %v for negative input", ph2.checkInterval, defaultCheckInterval)
	}
}

func TestWithMaxFails_IgnoresNonPositive(t *testing.T) {
	phZero := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithMaxFails(0),
	)
	if phZero.maxFails != defaultMaxFails {
		t.Errorf("maxFails = %d, want default %d for zero input", phZero.maxFails, defaultMaxFails)
	}

	phNegative := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithMaxFails(-2),
	)
	if phNegative.maxFails != defaultMaxFails {
		t.Errorf("maxFails = %d, want default %d for negative input", phNegative.maxFails, defaultMaxFails)
	}
}

func TestWithCooldownTime_IgnoresNonPositive(t *testing.T) {
	phZero := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCooldownTime(0),
	)
	if phZero.cooldownTime != defaultCooldownTime {
		t.Errorf("cooldownTime = %v, want default %v for zero input", phZero.cooldownTime, defaultCooldownTime)
	}

	phNegative := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCooldownTime(-3*time.Second),
	)
	if phNegative.cooldownTime != defaultCooldownTime {
		t.Errorf("cooldownTime = %v, want default %v for negative input", phNegative.cooldownTime, defaultCooldownTime)
	}
}

func TestProviderHealth_DoubleStartNoLeak(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	// Start twice without explicit Stop
	mustStartProviderHealth(t, ph, context.Background())
	time.Sleep(30 * time.Millisecond)
	mustStartProviderHealth(t, ph, context.Background())
	time.Sleep(30 * time.Millisecond)

	mustStopProviderHealth(t, ph)
	// Read counter AFTER Stop returns; Stop blocks until the goroutine exits,
	// so no in-flight checker call can race with this read.
	countAfterStop := calls.Load()
	time.Sleep(50 * time.Millisecond)

	// After Stop, no more calls should happen — proves first goroutine was stopped
	if calls.Load() != countAfterStop {
		t.Error("checker called after Stop; first goroutine may have leaked")
	}
}

func TestProviderHealth_ContextCancelStops(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)

	countAfterCancel := calls.Load()
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != countAfterCancel {
		t.Error("checker should not be called after parent context cancel")
	}

	// Stop should still be safe after context cancel
	mustStopProviderHealth(t, ph)
}

func TestProviderHealth_StartNilContext(t *testing.T) {
	var calls atomic.Int32
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(time.Hour),
	)

	defer mustStopProviderHealth(t, ph)

	mustStartProviderHealth(t, ph, nilContextForTest())
	time.Sleep(50 * time.Millisecond)

	if calls.Load() < 1 {
		t.Error("checker should be called when Start(nil) is used")
	}
	if !ph.IsHealthy() {
		t.Error("IsHealthy() should remain true after successful Start(nil) check")
	}
}

func TestProviderHealth_ConcurrentStartStop_NoDeadlock(t *testing.T) {
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			select {
			case <-ctx.Done():
				return false
			default:
				return true
			}
		},
		WithCheckInterval(2*time.Millisecond),
	)

	const workers = 8
	const iterations = 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if (j+offset)%2 == 0 {
					if err := ph.Start(context.Background()); err != nil {
						t.Errorf("Start() unexpected error = %v", err)
						return
					}
				} else {
					if stopErr := ph.Stop(); stopErr != nil {
						t.Errorf("Stop() unexpected error = %v", stopErr)
						return
					}
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent Start/Stop did not finish in time")
	}

	mustStopProviderHealth(t, ph)
}

func TestProviderHealth_ConcurrentStop_IsSafe(t *testing.T) {
	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool { return true },
		WithCheckInterval(5*time.Millisecond),
	)
	mustStartProviderHealth(t, ph, context.Background())

	const callers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- ph.Stop()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("Stop() unexpected error = %v", err)
		}
	}
}

func TestProviderHealth_Stop_ReturnsErrorWhenCheckerHangs(t *testing.T) {
	started := make(chan struct{}, 1)
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			select {
			case started <- struct{}{}:
			default:
			}
			<-hang
			return true
		},
		WithCheckInterval(time.Hour),
	)

	mustStartProviderHealth(t, ph, context.Background())
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	stopped := make(chan struct{})
	stopErr := make(chan error, 1)
	go func() {
		stopErr <- ph.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		if err := <-stopErr; !errors.Is(err, ErrProviderHealthStopTimeout) {
			t.Fatalf("Stop() error = %v, want %v", err, ErrProviderHealthStopTimeout)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Stop() blocked indefinitely with hanging checker")
	}

	close(hang)
}

func TestProviderHealth_Start_PropagatesPreviousStopTimeout(t *testing.T) {
	started := make(chan struct{}, 1)
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			select {
			case started <- struct{}{}:
			default:
			}
			<-hang
			return true
		},
		WithCheckInterval(time.Hour),
	)

	mustStartProviderHealth(t, ph, context.Background())
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	err := ph.Start(context.Background())
	if !errors.Is(err, ErrProviderHealthStopTimeout) {
		t.Fatalf("Start() error = %v, want %v", err, ErrProviderHealthStopTimeout)
	}

	close(hang)
	defer mustStopProviderHealth(t, ph)
}

func TestProviderHealth_CheckTimeout_AllowsNextCycleAfterBlockingChecker(t *testing.T) {
	var calls atomic.Int32

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			<-ctx.Done()
			return false
		},
		WithCheckInterval(20*time.Millisecond),
		WithCheckTimeout(40*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	waitForCondition(t, 120*time.Millisecond, "checker to run at least twice after timeout", func() bool {
		return calls.Load() >= 2
	})
}

func TestProviderHealth_CheckTimeout_NoCheckerPileupWhenCheckerIgnoresCancel(t *testing.T) {
	var calls atomic.Int32
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			calls.Add(1)
			<-hang
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(20*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)
	defer close(hang)

	waitForCondition(t, 80*time.Millisecond, "checker to be marked stuck after timeout", func() bool {
		ph.mu.RLock()
		defer ph.mu.RUnlock()
		return ph.checkerStuck
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 to avoid goroutine pile-up on timeout", got)
	}
}

func TestProviderHealth_CheckTimeout_DelayedCancelCleanupDoesNotLatchStuck(t *testing.T) {
	var calls atomic.Int32
	const (
		checkTimeout  = 10 * time.Millisecond
		cleanupDelay  = 12 * time.Millisecond
		checkInterval = 50 * time.Millisecond
	)

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			call := calls.Add(1)
			if call == 1 {
				<-ctx.Done()
				time.Sleep(cleanupDelay)
				return false
			}
			return true
		},
		WithCheckInterval(checkInterval),
		WithCheckTimeout(checkTimeout),
		WithMaxFails(1),
		WithCooldownTime(5*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	waitForCondition(t, 80*time.Millisecond, "initial timed out check to mark provider unhealthy", func() bool {
		return !ph.IsHealthy()
	})

	if ph.IsHealthy() {
		t.Fatal("IsHealthy() = true, want false after initial timeout failure")
	}
	ph.mu.RLock()
	stuckAfterCleanup := ph.checkerStuck
	ph.mu.RUnlock()
	if stuckAfterCleanup {
		t.Fatal("checkerStuck = true, want false when checker exits shortly after cancellation")
	}

	waitForCondition(t, 120*time.Millisecond, "next scheduled check to recover health", func() bool {
		return calls.Load() >= 2 && ph.IsHealthy()
	})
	ph.mu.RLock()
	stuckAfterRecovery := ph.checkerStuck
	ph.mu.RUnlock()
	if stuckAfterRecovery {
		t.Fatal("checkerStuck = true, want false after recovery from delayed cancel cleanup")
	}
}

func TestProviderHealth_CheckTimeout_MarksUnhealthyWhenCheckerStaysStuck(t *testing.T) {
	started := make(chan struct{}, 1)
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			select {
			case started <- struct{}{}:
			default:
			}
			<-hang
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(20*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)
	defer close(hang)

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	waitForCondition(t, 80*time.Millisecond, "stuck checker to mark provider unhealthy", func() bool {
		return !ph.IsHealthy()
	})

	if ph.IsHealthy() {
		t.Fatal("IsHealthy() = true, want false after timed out checker remains stuck")
	}
}

func TestProviderHealth_CheckTimeout_StuckCheckerRequiresRestart(t *testing.T) {
	var calls atomic.Int32
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			call := calls.Add(1)
			if call == 1 {
				<-hang
			}
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(20*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	waitForCondition(t, 80*time.Millisecond, "stuck checker to mark provider unhealthy", func() bool {
		return !ph.IsHealthy()
	})
	if ph.IsHealthy() {
		t.Fatal("IsHealthy() = true, want false after stuck checker timeout")
	}

	close(hang)
	assertConditionStable(t, 40*time.Millisecond, "monitor to remain stopped until restart", func() bool {
		return calls.Load() == 1
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 while monitor stays stuck until restart", got)
	}

	mustStartProviderHealth(t, ph, context.Background())
	waitForCondition(t, 80*time.Millisecond, "restart to re-enable checks", func() bool {
		return calls.Load() >= 2 && ph.IsHealthy()
	})

	if got := calls.Load(); got < 2 {
		t.Fatalf("checker calls = %d, want >= 2 after restart re-enables checks", got)
	}
	if !ph.IsHealthy() {
		t.Fatal("IsHealthy() = false, want true after restart and successful check")
	}
}

func TestProviderHealth_StartDoesNotOverlapStillStuckTimedOutChecker(t *testing.T) {
	var calls atomic.Int32
	firstCallStarted := make(chan struct{}, 1)
	hang := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			call := calls.Add(1)
			if call == 1 {
				select {
				case firstCallStarted <- struct{}{}:
				default:
				}
				<-hang
			}
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(20*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	select {
	case <-firstCallStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	waitForCondition(t, 80*time.Millisecond, "stuck checker to mark provider unhealthy", func() bool {
		return !ph.IsHealthy()
	})
	if ph.IsHealthy() {
		t.Fatal("IsHealthy() = true, want false after stuck checker timeout")
	}

	err := ph.Start(context.Background())
	if !errors.Is(err, ErrProviderHealthStopTimeout) {
		t.Fatalf("Start() error = %v, want %v", err, ErrProviderHealthStopTimeout)
	}

	assertConditionStable(t, 30*time.Millisecond, "previous timed-out checker to remain the only call", func() bool {
		return calls.Load() == 1
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 while previous timed-out checker is still running", got)
	}

	close(hang)
	assertConditionStable(t, 30*time.Millisecond, "old checker exit to avoid implicit restart", func() bool {
		return calls.Load() == 1
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 after old checker exits until explicit restart", got)
	}

	mustStartProviderHealth(t, ph, context.Background())
	waitForCondition(t, 80*time.Millisecond, "explicit restart after timed-out checker exit", func() bool {
		return calls.Load() >= 2 && ph.IsHealthy()
	})

	if got := calls.Load(); got < 2 {
		t.Fatalf("checker calls = %d, want >= 2 after explicit restart once old checker exits", got)
	}
	if !ph.IsHealthy() {
		t.Fatal("IsHealthy() = false, want true after restart once prior checker has exited")
	}
}

func TestProviderHealth_StartDoesNotOverlapStillRunningStoppedChecker(t *testing.T) {
	var calls atomic.Int32
	firstCallStarted := make(chan struct{}, 1)
	release := make(chan struct{})

	ph := mustNewProviderHealth(t,
		func(ctx context.Context) bool {
			call := calls.Add(1)
			if call == 1 {
				select {
				case firstCallStarted <- struct{}{}:
				default:
				}
				<-release
			}
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(500*time.Millisecond),
	)

	mustStartProviderHealth(t, ph, context.Background())

	select {
	case <-firstCallStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	if err := ph.Stop(); !errors.Is(err, ErrProviderHealthStopTimeout) {
		t.Fatalf("Stop() error = %v, want %v", err, ErrProviderHealthStopTimeout)
	}
	err := ph.Start(context.Background())
	if !errors.Is(err, ErrProviderHealthStopTimeout) {
		t.Fatalf("Start() error = %v, want %v", err, ErrProviderHealthStopTimeout)
	}

	assertConditionStable(t, 30*time.Millisecond, "stopped checker to remain the only call", func() bool {
		return calls.Load() == 1
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 while previous stopped checker is still running", got)
	}

	close(release)
	assertConditionStable(t, 30*time.Millisecond, "old checker exit to avoid implicit restart", func() bool {
		return calls.Load() == 1
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 after old checker exits until explicit restart", got)
	}

	mustStartProviderHealth(t, ph, context.Background())
	defer mustStopProviderHealth(t, ph)

	waitForCondition(t, 80*time.Millisecond, "restart after stopped checker exit", func() bool {
		return calls.Load() >= 2
	})
	if got := calls.Load(); got < 2 {
		t.Fatalf("checker calls = %d, want >= 2 after restart once old checker exits", got)
	}
}
