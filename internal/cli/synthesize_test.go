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
		if closeErr := r.Close(); closeErr != nil {
			t.Errorf("close read pipe: %v", closeErr)
		}
	}()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	if _, writeErr := w.Write([]byte("hello")); writeErr != nil {
		if closeErr := w.Close(); closeErr != nil {
			t.Errorf("close write pipe after write failure: %v", closeErr)
		}
		t.Fatalf("write stdin pipe: %v", writeErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close write pipe: %v", closeErr)
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
	registry["fake"] = ProviderRegistration{
		Factory: func(cfg *ProviderConfig) (tts.Provider, error) {
			factoryCalled = true
			return &testAdapter{name: "fake"}, nil
		},
		DefaultVoice: "voice-1",
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
	registry["fake"] = ProviderRegistration{
		Factory: func(cfg *ProviderConfig) (tts.Provider, error) {
			factoryCalled = true
			return &testAdapter{name: "fake"}, nil
		},
		DefaultVoice: "voice-1",
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
		"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
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
		"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
			_ = cfg
			return &testAdapter{name: "fake"}, nil
		},
	})

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("os.Chdir(%q) error: %v", tmpDir, chdirErr)
	}
	t.Cleanup(func() {
		if restoreErr := os.Chdir(oldWD); restoreErr != nil {
			t.Fatalf("restore working directory: %v", restoreErr)
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

type deadlineInspectingSynthesizeAdapter struct {
	name        string
	hasDeadline chan bool
}

type recordingSynthesizeAdapter struct {
	name         string
	capabilities tts.ProviderCapabilities
	lastRequest  tts.SynthesisRequest
	callCount    int
}

func (a *deadlineInspectingSynthesizeAdapter) Name() string { return a.name }

func (a *deadlineInspectingSynthesizeAdapter) Capabilities() tts.ProviderCapabilities {
	return testProviderCapabilities(true, tts.AudioFormatMP3)
}

func (a *deadlineInspectingSynthesizeAdapter) ListVoices(ctx context.Context, filter tts.VoiceFilter) ([]tts.Voice, error) {
	_ = ctx
	_ = filter
	return nil, errors.New("not implemented")
}

func (a *deadlineInspectingSynthesizeAdapter) Synthesize(ctx context.Context, request tts.SynthesisRequest) (*tts.SynthesisResult, error) {
	_, ok := ctx.Deadline()
	select {
	case a.hasDeadline <- ok:
	default:
	}
	return &tts.SynthesisResult{
		Audio:    []byte("audio"),
		Format:   tts.AudioFormatMP3,
		Provider: a.name,
		VoiceID:  request.VoiceID,
	}, nil
}

func (a *deadlineInspectingSynthesizeAdapter) SynthesizeStream(ctx context.Context, request tts.SynthesisRequest) (tts.AudioStream, error) {
	_ = ctx
	_ = request
	return nil, errors.New("not implemented")
}

func (a *recordingSynthesizeAdapter) Name() string { return a.name }

func (a *recordingSynthesizeAdapter) Capabilities() tts.ProviderCapabilities {
	if a.capabilities.PreferredAudioFormat == "" && len(a.capabilities.SupportedFormats) == 0 {
		return testProviderCapabilities(true, tts.AudioFormatMP3)
	}
	return a.capabilities.Clone()
}

func (a *recordingSynthesizeAdapter) ListVoices(ctx context.Context, filter tts.VoiceFilter) ([]tts.Voice, error) {
	_ = ctx
	_ = filter
	return nil, errors.New("not implemented")
}

func (a *recordingSynthesizeAdapter) Synthesize(ctx context.Context, request tts.SynthesisRequest) (*tts.SynthesisResult, error) {
	a.callCount++
	a.lastRequest = request
	return &tts.SynthesisResult{
		Audio:    []byte("audio"),
		Format:   tts.AudioFormatMP3,
		Provider: a.name,
		VoiceID:  request.VoiceID,
	}, nil
}

func (a *recordingSynthesizeAdapter) SynthesizeStream(ctx context.Context, request tts.SynthesisRequest) (tts.AudioStream, error) {
	_ = ctx
	_ = request
	return nil, errors.New("not implemented")
}

func TestSynthesizeRun_RejectsUnsupportedProsodyFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		flagValue string
	}{
		{name: "rate", flagName: "--rate", flagValue: "+10%"},
		{name: "volume", flagName: "--volume", flagValue: "+10%"},
		{name: "pitch", flagName: "--pitch", flagValue: "+10%"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingSynthesizeAdapter{
				name:         "fake",
				capabilities: testProviderCapabilities(false, tts.AudioFormatMP3),
			}
			withTestRegistry(t, map[string]ProviderFactory{
				"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
					_ = cfg
					return adapter, nil
				},
			})

			cmd := NewSynthesizeCmd()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--stdout", tt.flagName, tt.flagValue}, &stdout, &stderr)
			if err == nil {
				t.Fatal("Run() expected error, got nil")
			}
			var usageErr *UsageError
			if !errors.As(err, &usageErr) {
				t.Fatalf("Run() error = %T, want *UsageError", err)
			}
			if !strings.Contains(usageErr.Message, tt.flagName) {
				t.Fatalf("usage error message = %q, want unsupported flag %q", usageErr.Message, tt.flagName)
			}
			if adapter.callCount != 0 {
				t.Fatalf("Synthesize() call count = %d, want 0 when prosody is unsupported", adapter.callCount)
			}
		})
	}
}

func TestSynthesizeRun_NeutralProsodySpellingsRemainPlainText(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		flagValue string
	}{
		{name: "rate 0%", flagName: "--rate", flagValue: "0%"},
		{name: "rate +0.0%", flagName: "--rate", flagValue: "+0.0%"},
		{name: "rate -0%", flagName: "--rate", flagValue: "-0%"},
		{name: "volume 0%", flagName: "--volume", flagValue: "0%"},
		{name: "volume +0.0%", flagName: "--volume", flagValue: "+0.0%"},
		{name: "volume -0%", flagName: "--volume", flagValue: "-0%"},
		{name: "pitch 0%", flagName: "--pitch", flagValue: "0%"},
		{name: "pitch +0.0%", flagName: "--pitch", flagValue: "+0.0%"},
		{name: "pitch -0%", flagName: "--pitch", flagValue: "-0%"},
	}

	for _, supportsProsody := range []bool{false, true} {
		supportsProsody := supportsProsody
		providerMode := "unsupported"
		if supportsProsody {
			providerMode = "supported"
		}

		for _, tt := range tests {
			tt := tt
			t.Run(providerMode+"/"+tt.name, func(t *testing.T) {
				adapter := &recordingSynthesizeAdapter{
					name:         "fake",
					capabilities: testProviderCapabilities(supportsProsody, tts.AudioFormatMP3),
				}
				withTestRegistry(t, map[string]ProviderFactory{
					"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
						_ = cfg
						return adapter, nil
					},
				})

				cmd := NewSynthesizeCmd()
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--stdout", tt.flagName, tt.flagValue}, &stdout, &stderr)
				if err != nil {
					t.Fatalf("Run() unexpected error: %v", err)
				}
				if adapter.callCount != 1 {
					t.Fatalf("Synthesize() call count = %d, want 1", adapter.callCount)
				}
				if adapter.lastRequest.InputMode != tts.InputModePlainText {
					t.Fatalf("request.InputMode = %q, want %q", adapter.lastRequest.InputMode, tts.InputModePlainText)
				}
				if !adapter.lastRequest.Prosody.IsZero() {
					t.Fatalf("request.Prosody = %+v, want zero-value prosody", adapter.lastRequest.Prosody)
				}
				if stdout.String() != "audio" {
					t.Fatalf("stdout = %q, want synthesized audio bytes", stdout.String())
				}
				if stderr.Len() != 0 {
					t.Fatalf("stderr = %q, want empty", stderr.String())
				}
			})
		}
	}
}

func TestSynthesizeRun_ProsodyFlagsUsePlainTextWithProsody(t *testing.T) {
	adapter := &recordingSynthesizeAdapter{
		name:         "fake",
		capabilities: testProviderCapabilities(true, tts.AudioFormatMP3),
	}
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
			_ = cfg
			return adapter, nil
		},
	})

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{
		"--provider", "fake",
		"--text", "hello",
		"--voice", "voice-1",
		"--stdout",
		"--rate", "+25%",
		"--volume", "-20%",
		"--pitch", "+10%",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if adapter.callCount != 1 {
		t.Fatalf("Synthesize() call count = %d, want 1", adapter.callCount)
	}
	if adapter.lastRequest.InputMode != tts.InputModePlainTextWithProsody {
		t.Fatalf("request.InputMode = %q, want %q", adapter.lastRequest.InputMode, tts.InputModePlainTextWithProsody)
	}
	if adapter.lastRequest.Text != "hello" {
		t.Fatalf("request.Text = %q, want %q", adapter.lastRequest.Text, "hello")
	}
	if adapter.lastRequest.VoiceID != "voice-1" {
		t.Fatalf("request.VoiceID = %q, want %q", adapter.lastRequest.VoiceID, "voice-1")
	}
	wantProsody := tts.ProsodyParams{Rate: 1.25, Volume: 0.8, Pitch: 1.1}
	if adapter.lastRequest.Prosody != wantProsody {
		t.Fatalf("request.Prosody = %+v, want %+v", adapter.lastRequest.Prosody, wantProsody)
	}
	if stdout.String() != "audio" {
		t.Fatalf("stdout = %q, want synthesized audio bytes", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSynthesizeRun_VolumeMinus100PreservesExplicitMute(t *testing.T) {
	adapter := &recordingSynthesizeAdapter{
		name:         "fake",
		capabilities: testProviderCapabilities(true, tts.AudioFormatMP3),
	}
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
			_ = cfg
			return adapter, nil
		},
	})

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{
		"--provider", "fake",
		"--text", "hello",
		"--stdout",
		"--volume", "-100%",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if adapter.callCount != 1 {
		t.Fatalf("Synthesize() call count = %d, want 1", adapter.callCount)
	}
	if adapter.lastRequest.InputMode != tts.InputModePlainTextWithProsody {
		t.Fatalf("request.InputMode = %q, want %q", adapter.lastRequest.InputMode, tts.InputModePlainTextWithProsody)
	}
	if !adapter.lastRequest.Prosody.HasVolume() {
		t.Fatal("request.Prosody.HasVolume() = false, want true")
	}
	if adapter.lastRequest.Prosody.Volume != 0 {
		t.Fatalf("request.Prosody.Volume = %v, want 0", adapter.lastRequest.Prosody.Volume)
	}
	if adapter.lastRequest.Prosody.HasRate() {
		t.Fatal("request.Prosody.HasRate() = true, want false")
	}
	if adapter.lastRequest.Prosody.HasPitch() {
		t.Fatal("request.Prosody.HasPitch() = true, want false")
	}
	if stdout.String() != "audio" {
		t.Fatalf("stdout = %q, want synthesized audio bytes", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestSynthesizeRun_DoesNotImposeCommandDeadline(t *testing.T) {
	hasDeadline := make(chan bool, 1)
	withTestRegistry(t, map[string]ProviderFactory{
		"fake": func(cfg *ProviderConfig) (tts.Provider, error) {
			_ = cfg
			return &deadlineInspectingSynthesizeAdapter{name: "fake", hasDeadline: hasDeadline}, nil
		},
	})

	cmd := NewSynthesizeCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := cmd.Run([]string{"--provider", "fake", "--text", "hello", "--stdout", "--http-timeout", "25ms", "--max-attempts", "2"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if <-hasDeadline {
		t.Fatal("Synthesize() context unexpectedly had a command-level deadline")
	}
	if stdout.String() != "audio" {
		t.Fatalf("stdout = %q, want synthesized audio bytes", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
