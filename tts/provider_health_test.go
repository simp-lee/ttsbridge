package tts

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func nilContextForTest() context.Context {
	var ctx context.Context
	return ctx
}

func TestNewProviderHealth_Defaults(t *testing.T) {
	ph := NewProviderHealth(func(ctx context.Context) bool { return true })

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
	ph := NewProviderHealth(
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
	ph := NewProviderHealth(func(ctx context.Context) bool { return true })
	if !ph.IsHealthy() {
		t.Error("IsHealthy() should return true by default")
	}
}

func TestProviderHealth_Start_ImmediateCheck(t *testing.T) {
	var calls atomic.Int32
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(time.Hour), // long interval so only immediate check fires
	)

	ph.Start(context.Background())
	defer ph.Stop()

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
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return false // always fail
		},
		WithCheckInterval(20*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(100*time.Millisecond),
	)

	ph.Start(context.Background())
	defer ph.Stop()

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
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			n := calls.Add(1)
			// Fail first 2 calls, then succeed
			return n > 2
		},
		WithCheckInterval(20*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(time.Hour),
	)

	ph.Start(context.Background())
	defer ph.Stop()

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
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			n := calls.Add(1)
			// Fail first 4 calls (enough to enter cooldown), then succeed
			return n > 4
		},
		WithCheckInterval(15*time.Millisecond),
		WithMaxFails(3),
		WithCooldownTime(60*time.Millisecond),
	)

	ph.Start(context.Background())
	defer ph.Stop()

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
	ph := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCheckInterval(time.Hour),
	)

	ph.Start(context.Background())

	// Multiple Stop calls should not panic
	ph.Stop()
	ph.Stop()
	ph.Stop()
}

func TestProviderHealth_StopWithoutStart(t *testing.T) {
	ph := NewProviderHealth(func(ctx context.Context) bool { return true })

	// Stop without Start should not panic
	ph.Stop()
	ph.Stop()
}

func TestProviderHealth_StopCancelsChecker(t *testing.T) {
	var calls atomic.Int32
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	ph.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	ph.Stop()

	countAfterStop := calls.Load()
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != countAfterStop {
		t.Error("checker should not be called after Stop")
	}
}

func TestNewProviderHealth_NilCheckerPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil checker, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != "tts: ProviderHealth checker must not be nil" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	NewProviderHealth(nil)
}

func TestWithCheckInterval_IgnoresNonPositive(t *testing.T) {
	ph := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCheckInterval(0),
	)
	if ph.checkInterval != defaultCheckInterval {
		t.Errorf("checkInterval = %v, want default %v for zero input", ph.checkInterval, defaultCheckInterval)
	}

	ph2 := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCheckInterval(-5*time.Second),
	)
	if ph2.checkInterval != defaultCheckInterval {
		t.Errorf("checkInterval = %v, want default %v for negative input", ph2.checkInterval, defaultCheckInterval)
	}
}

func TestWithMaxFails_IgnoresNonPositive(t *testing.T) {
	phZero := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithMaxFails(0),
	)
	if phZero.maxFails != defaultMaxFails {
		t.Errorf("maxFails = %d, want default %d for zero input", phZero.maxFails, defaultMaxFails)
	}

	phNegative := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithMaxFails(-2),
	)
	if phNegative.maxFails != defaultMaxFails {
		t.Errorf("maxFails = %d, want default %d for negative input", phNegative.maxFails, defaultMaxFails)
	}
}

func TestWithCooldownTime_IgnoresNonPositive(t *testing.T) {
	phZero := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCooldownTime(0),
	)
	if phZero.cooldownTime != defaultCooldownTime {
		t.Errorf("cooldownTime = %v, want default %v for zero input", phZero.cooldownTime, defaultCooldownTime)
	}

	phNegative := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCooldownTime(-3*time.Second),
	)
	if phNegative.cooldownTime != defaultCooldownTime {
		t.Errorf("cooldownTime = %v, want default %v for negative input", phNegative.cooldownTime, defaultCooldownTime)
	}
}

func TestProviderHealth_DoubleStartNoLeak(t *testing.T) {
	var calls atomic.Int32
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	// Start twice without explicit Stop
	ph.Start(context.Background())
	time.Sleep(30 * time.Millisecond)
	ph.Start(context.Background())
	time.Sleep(30 * time.Millisecond)

	ph.Stop()
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

	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(10*time.Millisecond),
	)

	ph.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)

	countAfterCancel := calls.Load()
	time.Sleep(50 * time.Millisecond)

	if calls.Load() != countAfterCancel {
		t.Error("checker should not be called after parent context cancel")
	}

	// Stop should still be safe after context cancel
	ph.Stop()
}

func TestProviderHealth_StartNilContext(t *testing.T) {
	var calls atomic.Int32
	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			return true
		},
		WithCheckInterval(time.Hour),
	)

	defer ph.Stop()

	ph.Start(nilContextForTest())
	time.Sleep(50 * time.Millisecond)

	if calls.Load() < 1 {
		t.Error("checker should be called when Start(nil) is used")
	}
	if !ph.IsHealthy() {
		t.Error("IsHealthy() should remain true after successful Start(nil) check")
	}
}

func TestProviderHealth_ConcurrentStartStop_NoDeadlock(t *testing.T) {
	ph := NewProviderHealth(
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
					ph.Start(context.Background())
				} else {
					ph.Stop()
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

	ph.Stop()
}

func TestProviderHealth_ConcurrentStop_IsSafe(t *testing.T) {
	ph := NewProviderHealth(
		func(ctx context.Context) bool { return true },
		WithCheckInterval(5*time.Millisecond),
	)
	ph.Start(context.Background())

	const callers = 16
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ph.Stop()
		}()
	}
	wg.Wait()
}

func TestProviderHealth_Stop_BoundedWhenCheckerHangs(t *testing.T) {
	started := make(chan struct{}, 1)
	hang := make(chan struct{})

	ph := NewProviderHealth(
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

	ph.Start(context.Background())
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checker did not start in time")
	}

	stopped := make(chan struct{})
	go func() {
		ph.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// expected: Stop should be bounded even if checker hangs
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Stop() blocked indefinitely with hanging checker")
	}
}

func TestProviderHealth_CheckTimeout_AllowsNextCycleAfterBlockingChecker(t *testing.T) {
	var calls atomic.Int32

	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			<-ctx.Done()
			return false
		},
		WithCheckInterval(20*time.Millisecond),
		WithCheckTimeout(40*time.Millisecond),
	)

	ph.Start(context.Background())
	defer ph.Stop()

	time.Sleep(160 * time.Millisecond)

	if got := calls.Load(); got < 2 {
		t.Fatalf("checker calls = %d, want >= 2 to prove next cycle runs after timeout", got)
	}
}

func TestProviderHealth_CheckTimeout_NoCheckerPileupWhenCheckerIgnoresCancel(t *testing.T) {
	var calls atomic.Int32
	hang := make(chan struct{})

	ph := NewProviderHealth(
		func(ctx context.Context) bool {
			calls.Add(1)
			<-hang
			return true
		},
		WithCheckInterval(10*time.Millisecond),
		WithCheckTimeout(20*time.Millisecond),
	)

	ph.Start(context.Background())
	defer ph.Stop()

	time.Sleep(120 * time.Millisecond)

	if got := calls.Load(); got != 1 {
		t.Fatalf("checker calls = %d, want 1 to avoid goroutine pile-up on timeout", got)
	}
}
