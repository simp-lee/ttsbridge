package cli

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

type failingListVoicesAdapter struct {
	name string
	err  error
}

type staticVoicesAdapter struct {
	name   string
	voices []tts.Voice
}

type blockingListVoicesAdapter struct {
	name    string
	started chan string
	budgets chan time.Duration
}

type nonCooperativeListVoicesAdapter struct {
	name     string
	started  chan string
	release  <-chan struct{}
	finished chan struct{}
}

func (a *failingListVoicesAdapter) Name() string { return a.name }

func (a *failingListVoicesAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return nil, a.err
}

func (a *failingListVoicesAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (a *failingListVoicesAdapter) DefaultVoice() string { return "voice-1" }

func (a *failingListVoicesAdapter) DefaultFormat() string { return "mp3" }

func (a *failingListVoicesAdapter) SupportsRateVolumePitch() bool { return false }

func (a *staticVoicesAdapter) Name() string { return a.name }

func (a *staticVoicesAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	_ = ctx
	_ = locale
	return a.voices, nil
}

func (a *staticVoicesAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (a *staticVoicesAdapter) DefaultVoice() string { return "voice-1" }

func (a *staticVoicesAdapter) DefaultFormat() string { return "mp3" }

func (a *staticVoicesAdapter) SupportsRateVolumePitch() bool { return false }

func (a *blockingListVoicesAdapter) Name() string { return a.name }

func (a *blockingListVoicesAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	_ = locale
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("missing context deadline")
	}
	select {
	case a.started <- a.name:
	default:
	}
	select {
	case a.budgets <- time.Until(deadline):
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (a *blockingListVoicesAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (a *blockingListVoicesAdapter) DefaultVoice() string { return "voice-1" }

func (a *blockingListVoicesAdapter) DefaultFormat() string { return "mp3" }

func (a *blockingListVoicesAdapter) SupportsRateVolumePitch() bool { return false }

func (a *nonCooperativeListVoicesAdapter) Name() string { return a.name }

func (a *nonCooperativeListVoicesAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	_ = ctx
	_ = locale
	defer close(a.finished)
	select {
	case a.started <- a.name:
	default:
	}
	<-a.release
	return nil, ctx.Err()
}

func (a *nonCooperativeListVoicesAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (a *nonCooperativeListVoicesAdapter) DefaultVoice() string { return "voice-1" }

func (a *nonCooperativeListVoicesAdapter) DefaultFormat() string { return "mp3" }

func (a *nonCooperativeListVoicesAdapter) SupportsRateVolumePitch() bool { return false }

func withTestRegistry(t *testing.T, entries map[string]ProviderFactory) {
	t.Helper()
	original := maps.Clone(registry)
	registry = maps.Clone(entries)
	t.Cleanup(func() {
		registry = original
	})
}

func TestVoicesRun_AllProvidersFailReturnsRuntimeError(t *testing.T) {
	sentinelAlpha := errors.New("alpha unavailable")
	sentinelBeta := errors.New("beta unavailable")
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &failingListVoicesAdapter{name: "alpha", err: sentinelAlpha}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &failingListVoicesAdapter{name: "beta", err: sentinelBeta}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "all"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var runtimeErr *RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("Run() error = %T, want *RuntimeError", err)
	}
	if !errors.Is(err, sentinelAlpha) || !errors.Is(err, sentinelBeta) {
		t.Fatalf("Run() error = %v, want joined failure chain for all providers", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on total failure", stdout.String())
	}
	if !strings.Contains(runtimeErr.Message, "alpha: alpha unavailable") {
		t.Fatalf("runtime error message = %q, want alpha failure", runtimeErr.Message)
	}
	if !strings.Contains(runtimeErr.Message, "beta: beta unavailable") {
		t.Fatalf("runtime error message = %q, want beta failure", runtimeErr.Message)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from alpha") {
		t.Fatalf("stderr = %q, want alpha warning", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from beta") {
		t.Fatalf("stderr = %q, want beta warning", stderr.String())
	}
}

func TestVoicesRun_ProviderFactoryErrorReturnsUsageError(t *testing.T) {
	sentinel := errors.New("invalid config")
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return nil, sentinel
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "alpha"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want preserved provider factory error chain", err)
	}
	if usageErr.Message != "invalid provider config" {
		t.Fatalf("usage error message = %q, want invalid provider config", usageErr.Message)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr for provider config error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on provider config error", stdout.String())
	}
}

func TestVoicesRun_PreservesInvalidProxyValidationMessage(t *testing.T) {
	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "edgetts", "--proxy", "://bad-proxy"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if !strings.Contains(usageErr.Message, "invalid proxy URL") {
		t.Fatalf("usage error message = %q, want invalid proxy URL context", usageErr.Message)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr for provider config error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on provider config error", stdout.String())
	}
}

func TestVoicesRun_NilAdapterFailurePreservesErrorChain(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return nil, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "alpha"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var runtimeErr *RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("Run() error = %T, want *RuntimeError", err)
	}
	if runtimeErr.Unwrap() == nil {
		t.Fatal("Run() runtime error unwrap = nil, want preserved failure chain")
	}
	if !strings.Contains(runtimeErr.Message, "alpha: provider adapter unavailable") {
		t.Fatalf("runtime error message = %q, want nil adapter failure", runtimeErr.Message)
	}
	if !strings.Contains(runtimeErr.Error(), "provider adapter unavailable") {
		t.Fatalf("Run() error = %q, want nil adapter cause in error string", runtimeErr.Error())
	}
	if !strings.Contains(stderr.String(), "Warning: provider alpha is unavailable") {
		t.Fatalf("stderr = %q, want nil adapter warning", stderr.String())
	}
}

func TestVoicesRun_SuccessUsesSharedProviderConfigAndSortsOutput(t *testing.T) {
	var seen []*ProviderConfig
	var seenMu sync.Mutex
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			copyCfg := *cfg
			seenMu.Lock()
			seen = append(seen, &copyCfg)
			seenMu.Unlock()
			return &staticVoicesAdapter{name: "alpha", voices: []tts.Voice{
				{Provider: "alpha", Language: "zh-CN", Gender: "Female", ID: "a2", Name: "Zulu"},
				{Provider: "alpha", Language: "en-US", Gender: "Male", ID: "a1", Name: "Alpha"},
			}}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			copyCfg := *cfg
			seenMu.Lock()
			seen = append(seen, &copyCfg)
			seenMu.Unlock()
			return &staticVoicesAdapter{name: "beta", voices: []tts.Voice{
				{Provider: "beta", Language: "en-GB", Gender: "Female", ID: "b1", Name: "Beta"},
			}}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{
		"--provider", "all",
		"--format", "text",
		"--proxy", "http://127.0.0.1:8080",
		"--http-timeout", "5s",
		"--max-attempts", "7",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	wantCfg := &ProviderConfig{Proxy: "http://127.0.0.1:8080", HTTPTimeout: 5 * time.Second, MaxAttempts: 7}
	if len(seen) != 2 {
		t.Fatalf("provider factories called %d times, want 2", len(seen))
	}
	for _, got := range seen {
		if !reflect.DeepEqual(got, wantCfg) {
			t.Fatalf("provider config = %+v, want %+v", got, wantCfg)
		}
	}

	wantOutput := strings.Join([]string{
		"alpha\ten-US\tMale\ta1\tAlpha",
		"alpha\tzh-CN\tFemale\ta2\tZulu",
		"beta\ten-GB\tFemale\tb1\tBeta",
		"",
	}, "\n")
	if stdout.String() != wantOutput {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty on success", stderr.String())
	}
}

func TestVoicesRun_AllProvidersShareTimeoutBudget(t *testing.T) {
	started := make(chan string, 2)
	budgets := make(chan time.Duration, 2)
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &blockingListVoicesAdapter{name: "alpha", started: started, budgets: budgets}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &blockingListVoicesAdapter{name: "beta", started: started, budgets: budgets}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := cmd.Run([]string{"--provider", "all", "--http-timeout", "25ms", "--max-attempts", "1"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var runtimeErr *RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("Run() error = %T, want *RuntimeError", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded in error chain", err)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case providerName := <-started:
			seen[providerName] = true
		default:
			t.Fatalf("started providers = %v, want both alpha and beta", seen)
		}
	}

	for i := 0; i < 2; i++ {
		remaining := <-budgets
		if remaining <= 0 || remaining > 50*time.Millisecond {
			t.Fatalf("remaining deadline budget = %v, want within (0, 50ms]", remaining)
		}
	}

	elapsed := time.Since(start)
	if elapsed > 75*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, want <= 75ms", elapsed)
	}
	if !strings.Contains(runtimeErr.Message, "alpha: context deadline exceeded") {
		t.Fatalf("runtime error message = %q, want alpha deadline failure", runtimeErr.Message)
	}
	if !strings.Contains(runtimeErr.Message, "beta: context deadline exceeded") {
		t.Fatalf("runtime error message = %q, want beta deadline failure", runtimeErr.Message)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from alpha") {
		t.Fatalf("stderr = %q, want alpha warning", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from beta") {
		t.Fatalf("stderr = %q, want beta warning", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestVoicesRun_AllProvidersShareScaledTimeoutBudgetWithMaxAttempts(t *testing.T) {
	started := make(chan string, 2)
	budgets := make(chan time.Duration, 2)
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &blockingListVoicesAdapter{name: "alpha", started: started, budgets: budgets}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &blockingListVoicesAdapter{name: "beta", started: started, budgets: budgets}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := cmd.Run([]string{"--provider", "all", "--http-timeout", "25ms", "--max-attempts", "3"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var runtimeErr *RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("Run() error = %T, want *RuntimeError", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded in error chain", err)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case providerName := <-started:
			seen[providerName] = true
		default:
			t.Fatalf("started providers = %v, want both alpha and beta", seen)
		}
	}

	for i := 0; i < 2; i++ {
		remaining := <-budgets
		if remaining <= 45*time.Millisecond || remaining > 100*time.Millisecond {
			t.Fatalf("remaining deadline budget = %v, want within (45ms, 100ms]", remaining)
		}
	}

	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond || elapsed > 125*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, want within [50ms, 125ms]", elapsed)
	}
	if !strings.Contains(runtimeErr.Message, "alpha: context deadline exceeded") {
		t.Fatalf("runtime error message = %q, want alpha deadline failure", runtimeErr.Message)
	}
	if !strings.Contains(runtimeErr.Message, "beta: context deadline exceeded") {
		t.Fatalf("runtime error message = %q, want beta deadline failure", runtimeErr.Message)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from alpha") {
		t.Fatalf("stderr = %q, want alpha warning", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from beta") {
		t.Fatalf("stderr = %q, want beta warning", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if strings.Contains(stderr.String(), "provider adapter unavailable") {
		t.Fatalf("stderr = %q, want timeout warnings only", stderr.String())
	}
	_ = runtimeErr
}

func TestVoicesRun_AllProvidersReturnsOnSharedTimeoutBudgetWhenProviderIgnoresCancel(t *testing.T) {
	started := make(chan string, 1)
	release := make(chan struct{})
	finished := make(chan struct{})
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &staticVoicesAdapter{name: "alpha", voices: []tts.Voice{{
				Provider: "alpha",
				Language: "en-US",
				Gender:   "Female",
				ID:       "a1",
				Name:     "Alpha",
			}}}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &nonCooperativeListVoicesAdapter{name: "beta", started: started, release: release, finished: finished}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := cmd.Run([]string{"--provider", "all", "--http-timeout", "25ms", "--max-attempts", "1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	select {
	case providerName := <-started:
		if providerName != "beta" {
			t.Fatalf("started provider = %q, want beta", providerName)
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("beta provider did not start")
	}

	elapsed := time.Since(start)
	if elapsed > 75*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, want <= 75ms", elapsed)
	}

	wantOutput := "alpha\ten-US\tFemale\ta1\tAlpha\n"
	if stdout.String() != wantOutput {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from beta: context deadline exceeded") {
		t.Fatalf("stderr = %q, want beta deadline warning", stderr.String())
	}

	close(release)
	select {
	case <-finished:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("non-cooperative provider goroutine did not exit after release")
	}
	release = nil
}

func TestVoicesRun_AllProvidersReturnsOnSharedTimeoutBudgetWhenMultipleProvidersIgnoreCancel(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	finishedBeta := make(chan struct{})
	finishedGamma := make(chan struct{})
	var releaseOnce sync.Once
	releaseProviders := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	waitForFinished := func(name string, finished <-chan struct{}) {
		t.Helper()
		select {
		case <-finished:
		case <-time.After(20 * time.Millisecond):
			t.Errorf("%s non-cooperative provider goroutine did not exit after release", name)
		}
	}
	t.Cleanup(func() {
		releaseProviders()
		waitForFinished("beta", finishedBeta)
		waitForFinished("gamma", finishedGamma)
	})
	withTestRegistry(t, map[string]ProviderFactory{
		"alpha": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &staticVoicesAdapter{name: "alpha", voices: []tts.Voice{{
				Provider: "alpha",
				Language: "en-US",
				Gender:   "Female",
				ID:       "a1",
				Name:     "Alpha",
			}}}, nil
		},
		"beta": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &nonCooperativeListVoicesAdapter{name: "beta", started: started, release: release, finished: finishedBeta}, nil
		},
		"gamma": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &nonCooperativeListVoicesAdapter{name: "gamma", started: started, release: release, finished: finishedGamma}, nil
		},
	})

	cmd := NewVoicesCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := cmd.Run([]string{"--provider", "all", "--http-timeout", "25ms", "--max-attempts", "1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	seen := map[string]bool{}
	deadline := time.After(20 * time.Millisecond)
	for len(seen) < 2 {
		select {
		case providerName := <-started:
			seen[providerName] = true
		case <-deadline:
			t.Fatalf("started providers = %v, want both beta and gamma", seen)
		}
	}

	elapsed := time.Since(start)
	if elapsed > 75*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, want <= 75ms", elapsed)
	}

	wantOutput := "alpha\ten-US\tFemale\ta1\tAlpha\n"
	if stdout.String() != wantOutput {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from beta: context deadline exceeded") {
		t.Fatalf("stderr = %q, want beta deadline warning", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Warning: failed to list voices from gamma: context deadline exceeded") {
		t.Fatalf("stderr = %q, want gamma deadline warning", stderr.String())
	}

	stdoutAfterReturn := stdout.String()
	stderrAfterReturn := stderr.String()

	releaseProviders()
	waitForFinished("beta", finishedBeta)
	waitForFinished("gamma", finishedGamma)

	if stdout.String() != stdoutAfterReturn {
		t.Fatalf("stdout changed after late goroutine completion: got %q, want %q", stdout.String(), stdoutAfterReturn)
	}
	if stderr.String() != stderrAfterReturn {
		t.Fatalf("stderr changed after late goroutine completion: got %q, want %q", stderr.String(), stderrAfterReturn)
	}
}
