package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestSynthesizeReadInputText_FileWithinLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.maxInputBytes = 5

	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp input file: %v", err)
	}
	cmd.file = inputFile

	got, err := cmd.readInputText()
	if err != nil {
		t.Fatalf("readInputText() unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("readInputText() = %q, want %q", got, "hello")
	}
}

func TestSynthesizeReadInputText_FileExceedsLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.maxInputBytes = 4

	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp input file: %v", err)
	}
	cmd.file = inputFile

	_, err := cmd.readInputText()
	if err == nil {
		t.Fatal("readInputText() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("readInputText() error = %T, want *UsageError", err)
	}
	if !strings.Contains(usageErr.Message, "input exceeds --max-input-bytes limit") {
		t.Fatalf("usage error message = %q, want limit message", usageErr.Message)
	}
}

func TestSynthesizeReadInputText_StdinExceedsLimit(t *testing.T) {
	cmd := NewSynthesizeCmd()
	cmd.text = ""
	cmd.file = "-"
	cmd.maxInputBytes = 3

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("close read pipe: %v", err)
		}
	}()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	if _, err := w.Write([]byte("hello")); err != nil {
		if closeErr := w.Close(); closeErr != nil {
			t.Errorf("close write pipe after write failure: %v", closeErr)
		}
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}

	_, err = cmd.readInputText()
	if err == nil {
		t.Fatal("readInputText() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("readInputText() error = %T, want *UsageError", err)
	}
}

func TestSynthesizeRun_RejectsNegativeMaxInputBytes(t *testing.T) {
	cmd := NewSynthesizeCmd()
	err := cmd.Run([]string{"--text", "hello", "--max-input-bytes", "-1"}, os.Stdout, os.Stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if usageErr.Message != "--max-input-bytes must be >= 0" {
		t.Fatalf("usage error message = %q, want exact validation message", usageErr.Message)
	}
}

func TestSynthesizeRun_RejectsInvalidMaxAttemptsBeforeProviderCreation(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{})

	factoryCalled := false
	registry["fake"] = func(cfg *ProviderConfig) (ProviderAdapter, error) {
		factoryCalled = true
		return &testAdapter{name: "fake"}, nil
	}

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--max-attempts", "0"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if usageErr.Message != "--max-attempts must be >= 1" {
		t.Fatalf("usage error message = %q, want exact validation message", usageErr.Message)
	}
	if factoryCalled {
		t.Fatal("provider factory was called for invalid --max-attempts")
	}
}

func TestSynthesizeRun_RejectsInvalidHTTPTimeoutBeforeProviderCreation(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{})

	factoryCalled := false
	registry["fake"] = func(cfg *ProviderConfig) (ProviderAdapter, error) {
		factoryCalled = true
		return &testAdapter{name: "fake"}, nil
	}

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--http-timeout", "0s"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if usageErr.Message != "--http-timeout must be > 0" {
		t.Fatalf("usage error message = %q, want exact validation message", usageErr.Message)
	}
	if factoryCalled {
		t.Fatal("provider factory was called for invalid --http-timeout")
	}
}

func TestSynthesizeRun_RejectsInvalidProxyAsUsageError(t *testing.T) {
	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--text", "hello", "--proxy", "://bad-proxy"}, &stdout, &stderr)
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
}

func TestSynthesizeRun_PreservesProviderFactoryErrorChain(t *testing.T) {
	sentinel := errors.New("provider config rejected")
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return nil, sentinel
		},
	})

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "fake", "--text", "hello"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run() error = %T, want *UsageError", err)
	}
	if usageErr.Message != "invalid provider config" {
		t.Fatalf("usage error message = %q, want %q", usageErr.Message, "invalid provider config")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want preserved provider factory error chain", err)
	}
}

func TestSynthesizeRun_AutoOutputPathReservationIsAtomic(t *testing.T) {
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &testAdapter{name: "fake"}, nil
		},
	})

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("os.Chdir(%q) error: %v", tmpDir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	oldNow := now
	fixedTime := time.Date(2026, time.March, 25, 12, 34, 56, 0, time.UTC)
	now = func() time.Time { return fixedTime }
	t.Cleanup(func() {
		now = oldNow
	})

	cmd := NewSynthesizeCmd()
	cmd.provider = "fake"

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	texts := []string{"first", "second"}
	for _, text := range texts {
		text := text
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			command := NewSynthesizeCmd()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			errCh <- command.Run([]string{"--provider", "fake", "--text", text}, &stdout, &stderr)
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for runErr := range errCh {
		if runErr != nil {
			t.Fatalf("Run() unexpected error: %v", runErr)
		}
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error: %v", tmpDir, err)
	}
	if len(entries) != 2 {
		t.Fatalf("created %d files, want 2", len(entries))
	}

	wantNames := map[string]bool{
		"tts_fake_20260325_123456.000000000.mp3":   true,
		"tts_fake_20260325_123456.000000000_1.mp3": true,
	}
	wantContents := map[string]bool{"first": true, "second": true}

	for _, entry := range entries {
		if !wantNames[entry.Name()] {
			t.Fatalf("unexpected output file %q", entry.Name())
		}
		delete(wantNames, entry.Name())

		data, err := os.ReadFile(filepath.Join(tmpDir, entry.Name()))
		if err != nil {
			t.Fatalf("os.ReadFile(%q) error: %v", entry.Name(), err)
		}
		content := string(data)
		if !wantContents[content] {
			t.Fatalf("unexpected output content %q in %q", content, entry.Name())
		}
		delete(wantContents, content)
	}

	if len(wantNames) != 0 {
		t.Fatalf("missing output files: %v", wantNames)
	}
	if len(wantContents) != 0 {
		t.Fatalf("missing output contents: %v", wantContents)
	}
}

type blockingSynthesizeAdapter struct {
	name    string
	budgets chan time.Duration
}

func (a *blockingSynthesizeAdapter) Name() string { return a.name }

func (a *blockingSynthesizeAdapter) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return nil, errors.New("not implemented")
}

func (a *blockingSynthesizeAdapter) Synthesize(ctx context.Context, opts *SynthesizeRequest) ([]byte, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("missing context deadline")
	}
	select {
	case a.budgets <- time.Until(deadline):
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (a *blockingSynthesizeAdapter) DefaultVoice() string { return "voice-1" }

func (a *blockingSynthesizeAdapter) DefaultFormat() string { return "mp3" }

func (a *blockingSynthesizeAdapter) SupportsRateVolumePitch() bool { return true }

func TestSynthesizeRun_UsesSharedTimeoutBudget(t *testing.T) {
	budgets := make(chan time.Duration, 1)
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (ProviderAdapter, error) {
			_ = cfg
			return &blockingSynthesizeAdapter{name: "fake", budgets: budgets}, nil
		},
	})

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--http-timeout", "25ms", "--max-attempts", "2"}, &stdout, &stderr)
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

	remaining := <-budgets
	if remaining <= 0 || remaining > 75*time.Millisecond {
		t.Fatalf("remaining deadline budget = %v, want within (0, 75ms]", remaining)
	}

	elapsed := time.Since(start)
	if elapsed > 90*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, want <= 90ms", elapsed)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
