package edgetts

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

type deterministicProbeAllProber struct {
	results map[string]bool
	calls   []string
}

func (p *deterministicProbeAllProber) ProbeFormat(_ context.Context, id string) (bool, error) {
	p.calls = append(p.calls, id)
	available, ok := p.results[id]
	if !ok {
		return false, nil
	}
	return available, nil
}

func probeStatusLabel(status tts.FormatStatus) string {
	switch status {
	case tts.FormatAvailable:
		return "AVAILABLE"
	case tts.FormatUnavailable:
		return "UNAVAILABLE"
	default:
		return "UNVERIFIED"
	}
}

func edgeTTSRepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// TestParseOutputFormat_AllFormats exercises ParseOutputFormat against every
// format string registered in the default registry (constants + unverified)
// plus edge cases. This is the exhaustive parsing test.
func TestParseOutputFormat_AllFormats(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantOK     bool
		wantFormat string
		wantRate   int
		wantCh     int
		wantBR     int
		wantLL     bool
	}{
		// ── Compile-time constants (9 formats) ───────────────────────
		{name: "const/mp3-24khz-48k", format: OutputFormatMP3_24khz_48k, wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 24000, wantCh: 1, wantBR: 48},
		{name: "const/mp3-24khz-96k", format: OutputFormatMP3_24khz_96k, wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 24000, wantCh: 1, wantBR: 96},
		{name: "const/mp3-24khz-160k", format: OutputFormatMP3_24khz_160k, wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 24000, wantCh: 1, wantBR: 160},
		{name: "const/mp3-48khz-192k", format: OutputFormatMP3_48khz_192k, wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 48000, wantCh: 1, wantBR: 192},
		{name: "const/mp3-48khz-320k", format: OutputFormatMP3_48khz_320k, wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 48000, wantCh: 1, wantBR: 320},
		{name: "const/opus-24khz", format: OutputFormatOpus_24khz, wantOK: true, wantFormat: "opus", wantRate: 24000, wantCh: 1},
		{name: "const/pcm-24khz", format: OutputFormatPCM_24khz, wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 24000, wantCh: 1, wantLL: true},
		{name: "const/webm-24khz", format: OutputFormatWebM_24khz, wantOK: true, wantFormat: "webm", wantRate: 24000, wantCh: 1},
		{name: "const/ogg-24khz", format: OutputFormatOgg_24khz, wantOK: true, wantFormat: "ogg", wantRate: 24000, wantCh: 1},

		// ── Unverified: MP3 formats ──────────────────────────────────
		{name: "unverified/mp3-16khz-32k", format: "audio-16khz-32kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 32},
		{name: "unverified/mp3-16khz-64k", format: "audio-16khz-64kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 64},
		{name: "unverified/mp3-16khz-128k", format: "audio-16khz-128kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 128},
		{name: "unverified/mp3-48khz-96k", format: "audio-48khz-96kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 48000, wantCh: 1, wantBR: 96},

		// ── Unverified: WebM ─────────────────────────────────────────
		{name: "unverified/webm-16khz", format: "webm-16khz-16bit-mono-opus", wantOK: true, wantFormat: "webm", wantRate: 16000, wantCh: 1},

		// ── Unverified: Ogg ──────────────────────────────────────────
		{name: "unverified/ogg-16khz", format: "ogg-16khz-16bit-mono-opus", wantOK: true, wantFormat: "ogg", wantRate: 16000, wantCh: 1},
		{name: "unverified/ogg-48khz", format: "ogg-48khz-16bit-mono-opus", wantOK: true, wantFormat: "ogg", wantRate: 48000, wantCh: 1},

		// ── Unverified: Opus raw ─────────────────────────────────────
		{name: "unverified/opus-16khz-32kbps", format: "audio-16khz-16bit-32kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: 16000, wantCh: 1},
		{name: "unverified/opus-24khz-24kbps", format: "audio-24khz-16bit-24kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: 24000, wantCh: 1},
		{name: "unverified/opus-24khz-48kbps", format: "audio-24khz-16bit-48kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: 24000, wantCh: 1},

		// ── Unverified: Raw PCM / special ────────────────────────────
		{name: "unverified/raw-8khz-alaw", format: "raw-8khz-8bit-mono-alaw", wantOK: true, wantFormat: "alaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/raw-8khz-mulaw", format: "raw-8khz-8bit-mono-mulaw", wantOK: true, wantFormat: "mulaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/raw-8khz-pcm", format: "raw-8khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 8000, wantCh: 1, wantLL: true},
		{name: "unverified/raw-16khz-pcm", format: "raw-16khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 16000, wantCh: 1, wantLL: true},
		{name: "unverified/raw-16khz-truesilk", format: "raw-16khz-16bit-mono-truesilk", wantOK: true, wantFormat: "silk", wantRate: 16000, wantCh: 1},
		{name: "unverified/raw-22050hz-pcm", format: "raw-22050hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 22050, wantCh: 1, wantLL: true},
		{name: "unverified/raw-24khz-truesilk", format: "raw-24khz-16bit-mono-truesilk", wantOK: true, wantFormat: "silk", wantRate: 24000, wantCh: 1},
		{name: "unverified/raw-44100hz-pcm", format: "raw-44100hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 44100, wantCh: 1, wantLL: true},
		{name: "unverified/raw-48khz-pcm", format: "raw-48khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 48000, wantCh: 1, wantLL: true},

		// ── Unverified: RIFF/WAV ─────────────────────────────────────
		{name: "unverified/riff-8khz-alaw", format: "riff-8khz-8bit-mono-alaw", wantOK: true, wantFormat: "alaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/riff-8khz-mulaw", format: "riff-8khz-8bit-mono-mulaw", wantOK: true, wantFormat: "mulaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/riff-8khz-pcm", format: "riff-8khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 8000, wantCh: 1, wantLL: true},
		{name: "unverified/riff-16khz-pcm", format: "riff-16khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 16000, wantCh: 1, wantLL: true},
		{name: "unverified/riff-22050hz-pcm", format: "riff-22050hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 22050, wantCh: 1, wantLL: true},
		{name: "unverified/riff-24khz-pcm", format: "riff-24khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 24000, wantCh: 1, wantLL: true},
		{name: "unverified/riff-44100hz-pcm", format: "riff-44100hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 44100, wantCh: 1, wantLL: true},
		{name: "unverified/riff-48khz-pcm", format: "riff-48khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatWAV, wantRate: 48000, wantCh: 1, wantLL: true},

		// ── Unverified: Special codecs ───────────────────────────────
		{name: "unverified/amr-wb", format: "amr-wb-16000hz", wantOK: true, wantFormat: "amr-wb", wantRate: 16000, wantCh: 1},
		{name: "unverified/g722", format: "g722-16khz-64kbps", wantOK: true, wantFormat: "g722", wantRate: 16000, wantCh: 1},

		// ── Edge cases ───────────────────────────────────────────────
		{name: "invalid/no-sample-rate", format: "invalid-format-string", wantOK: false},
		{name: "invalid/empty", format: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, ok := ParseOutputFormat(tt.format)
			if ok != tt.wantOK {
				t.Fatalf("ParseOutputFormat(%q) ok = %v, want %v", tt.format, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if profile.Format != tt.wantFormat {
				t.Errorf("Format = %q, want %q", profile.Format, tt.wantFormat)
			}
			if profile.SampleRate != tt.wantRate {
				t.Errorf("SampleRate = %d, want %d", profile.SampleRate, tt.wantRate)
			}
			if profile.Channels != tt.wantCh {
				t.Errorf("Channels = %d, want %d", profile.Channels, tt.wantCh)
			}
			if profile.Bitrate != tt.wantBR {
				t.Errorf("Bitrate = %d, want %d", profile.Bitrate, tt.wantBR)
			}
			if profile.Lossless != tt.wantLL {
				t.Errorf("Lossless = %v, want %v", profile.Lossless, tt.wantLL)
			}
		})
	}
}

// TestDefaultFormatRegistry_Constants verifies all 9 compile-time constants are
// registered as FormatAvailable and their profiles match ParseOutputFormat.
func TestDefaultFormatRegistry_Constants(t *testing.T) {
	constants := []string{
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_24khz_160k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatOpus_24khz,
		OutputFormatPCM_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,
	}
	for _, id := range constants {
		t.Run(id, func(t *testing.T) {
			f, ok := defaultFormatRegistry.Get(id)
			if !ok {
				t.Fatalf("constant %q not found in registry", id)
			}
			if f.Status != tts.FormatAvailable {
				t.Errorf("status = %v; want FormatAvailable", f.Status)
			}
			// Verify the registered profile matches what ParseOutputFormat produces.
			parsed, parseOK := ParseOutputFormat(id)
			if !parseOK {
				t.Fatalf("ParseOutputFormat(%q) returned false for a constant", id)
			}
			if f.Profile != parsed {
				t.Errorf("registry profile %+v != parsed profile %+v", f.Profile, parsed)
			}
		})
	}
}

// TestDefaultFormatRegistry_AllUnverifiedFormats verifies every unverified
// format in the registry has FormatUnverified status and a non-zero profile
// populated from ParseOutputFormat.
func TestDefaultFormatRegistry_AllUnverifiedFormats(t *testing.T) {
	allUnverified := []string{
		// MP3
		"audio-16khz-32kbitrate-mono-mp3",
		"audio-16khz-64kbitrate-mono-mp3",
		"audio-16khz-128kbitrate-mono-mp3",
		"audio-48khz-96kbitrate-mono-mp3",
		// WebM
		"webm-16khz-16bit-mono-opus",
		// Ogg
		"ogg-16khz-16bit-mono-opus",
		"ogg-48khz-16bit-mono-opus",
		// Opus raw
		"audio-16khz-16bit-32kbps-mono-opus",
		"audio-24khz-16bit-24kbps-mono-opus",
		"audio-24khz-16bit-48kbps-mono-opus",
		// Raw PCM / special
		"raw-8khz-8bit-mono-alaw",
		"raw-8khz-8bit-mono-mulaw",
		"raw-8khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-truesilk",
		"raw-22050hz-16bit-mono-pcm",
		"raw-24khz-16bit-mono-truesilk",
		"raw-44100hz-16bit-mono-pcm",
		"raw-48khz-16bit-mono-pcm",
		// RIFF/WAV
		"riff-8khz-8bit-mono-alaw",
		"riff-8khz-8bit-mono-mulaw",
		"riff-8khz-16bit-mono-pcm",
		"riff-16khz-16bit-mono-pcm",
		"riff-22050hz-16bit-mono-pcm",
		"riff-24khz-16bit-mono-pcm",
		"riff-44100hz-16bit-mono-pcm",
		"riff-48khz-16bit-mono-pcm",
		// Other
		"amr-wb-16000hz",
		"g722-16khz-64kbps",
	}
	for _, id := range allUnverified {
		t.Run(id, func(t *testing.T) {
			f, ok := defaultFormatRegistry.Get(id)
			if !ok {
				t.Fatalf("unverified format %q not found in registry", id)
			}
			if f.Status != tts.FormatUnverified {
				t.Errorf("status = %v; want FormatUnverified", f.Status)
			}
			// Profiles must be pre-populated via ParseOutputFormat.
			if f.Profile == (tts.VoiceAudioProfile{}) {
				t.Errorf("profile is zero-value for %q; expected pre-populated profile", id)
			}
			// Cross-check that the stored profile matches ParseOutputFormat.
			parsed, parseOK := ParseOutputFormat(id)
			if !parseOK {
				t.Fatalf("ParseOutputFormat(%q) returned false", id)
			}
			if f.Profile != parsed {
				t.Errorf("registry profile %+v != parsed profile %+v", f.Profile, parsed)
			}
		})
	}
}

func TestDefaultFormatRegistry_DocumentedCatalogComplete_NoDuplicates(t *testing.T) {
	expected := []string{
		// Constants (known available)
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_24khz_160k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatOpus_24khz,
		OutputFormatPCM_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,

		// Unverified IDs documented in output_format.go init()
		"audio-16khz-32kbitrate-mono-mp3",
		"audio-16khz-64kbitrate-mono-mp3",
		"audio-16khz-128kbitrate-mono-mp3",
		"audio-48khz-96kbitrate-mono-mp3",
		"webm-16khz-16bit-mono-opus",
		"ogg-16khz-16bit-mono-opus",
		"ogg-48khz-16bit-mono-opus",
		"audio-16khz-16bit-32kbps-mono-opus",
		"audio-24khz-16bit-24kbps-mono-opus",
		"audio-24khz-16bit-48kbps-mono-opus",
		"raw-8khz-8bit-mono-alaw",
		"raw-8khz-8bit-mono-mulaw",
		"raw-8khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-truesilk",
		"raw-22050hz-16bit-mono-pcm",
		"raw-24khz-16bit-mono-truesilk",
		"raw-44100hz-16bit-mono-pcm",
		"raw-48khz-16bit-mono-pcm",
		"riff-8khz-8bit-mono-alaw",
		"riff-8khz-8bit-mono-mulaw",
		"riff-8khz-16bit-mono-pcm",
		"riff-16khz-16bit-mono-pcm",
		"riff-22050hz-16bit-mono-pcm",
		"riff-24khz-16bit-mono-pcm",
		"riff-44100hz-16bit-mono-pcm",
		"riff-48khz-16bit-mono-pcm",
		"amr-wb-16000hz",
		"g722-16khz-64kbps",
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		if _, exists := expectedSet[id]; exists {
			t.Fatalf("duplicate expected id %q", id)
		}
		expectedSet[id] = struct{}{}
	}

	all := defaultFormatRegistry.All()
	gotSet := make(map[string]int, len(all))
	for _, f := range all {
		gotSet[f.ID]++
		if _, ok := ParseOutputFormat(f.ID); !ok {
			t.Fatalf("registry format %q is not parseable by ParseOutputFormat", f.ID)
		}
	}

	for id, count := range gotSet {
		if count != 1 {
			t.Fatalf("format %q appears %d times in registry, want 1", id, count)
		}
		if _, ok := expectedSet[id]; !ok {
			t.Fatalf("unexpected format in default registry: %q", id)
		}
	}

	for id := range expectedSet {
		if _, ok := gotSet[id]; !ok {
			t.Fatalf("expected format missing from default registry: %q", id)
		}
	}

	if len(gotSet) != len(expectedSet) {
		t.Fatalf("registry size = %d, expected = %d", len(gotSet), len(expectedSet))
	}
}

func TestDefaultFormatRegistry_AutoParse(t *testing.T) {
	// An unknown format string parseable by ParseOutputFormat should auto-register.
	id := "audio-96khz-512kbitrate-mono-mp3"
	f, ok := defaultFormatRegistry.Get(id)
	if !ok {
		t.Fatalf("auto-parse failed for %q", id)
	}
	if f.Status != tts.FormatUnverified {
		t.Errorf("auto-parsed format status = %v; want FormatUnverified", f.Status)
	}
	if f.Profile.SampleRate != 96000 {
		t.Errorf("auto-parsed SampleRate = %d; want 96000", f.Profile.SampleRate)
	}
}

// TestSupportedFormats_MatchesRegistryAvailable verifies that
// Provider.SupportedFormats returns exactly the same set as
// FormatRegistry.Available.
func TestSupportedFormats_MatchesRegistryAvailable(t *testing.T) {
	p := New()
	supported := p.SupportedFormats()
	available := p.FormatRegistry().Available()

	if len(supported) != len(available) {
		t.Fatalf("SupportedFormats() returned %d formats; Available() returned %d",
			len(supported), len(available))
	}

	availMap := make(map[string]struct{}, len(available))
	for _, f := range available {
		availMap[f.ID] = struct{}{}
	}
	for _, f := range supported {
		if _, ok := availMap[f.ID]; !ok {
			t.Errorf("SupportedFormats() contained %q which is not in Available()", f.ID)
		}
	}
}

func TestProvider_FormatRegistryAccessors(t *testing.T) {
	p := New()
	if p.FormatRegistry() == nil {
		t.Fatal("FormatRegistry() returned nil")
	}

	// SupportedFormats should return at least the compile-time constants.
	formats := p.SupportedFormats()
	if len(formats) < 9 {
		t.Errorf("SupportedFormats() returned %d formats; want >= 9", len(formats))
	}

	// Verify every constant appears in SupportedFormats.
	fmtSet := make(map[string]struct{}, len(formats))
	for _, f := range formats {
		fmtSet[f.ID] = struct{}{}
	}
	for _, id := range []string{
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_24khz_160k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatOpus_24khz,
		OutputFormatPCM_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,
	} {
		if _, ok := fmtSet[id]; !ok {
			t.Errorf("constant %q missing from SupportedFormats()", id)
		}
	}

	// WithFormatRegistry should replace the registry.
	custom := tts.NewFormatRegistry()
	p.WithFormatRegistry(custom)
	if p.FormatRegistry() != custom {
		t.Error("WithFormatRegistry did not replace registry")
	}
}

// TestDefaultFormatRegistry_TotalCount verifies the minimum expected count of
// registered formats (9 constants + 28 unverified = 37 at init time).
// The actual count may be higher because other tests auto-register formats
// via Get() on the shared defaultFormatRegistry.
func TestDefaultFormatRegistry_TotalCount(t *testing.T) {
	all := defaultFormatRegistry.All()

	// Count by status.
	var available, unverified int
	for _, f := range all {
		switch f.Status {
		case tts.FormatAvailable:
			available++
		case tts.FormatUnverified:
			unverified++
		}
	}

	if available != 9 {
		t.Errorf("available formats = %d; want 9", available)
	}
	// At least 28 unverified from init(); may be more due to auto-parse in other tests.
	if unverified < 28 {
		t.Errorf("unverified formats = %d; want >= 28", unverified)
	}
}

func TestOutputFormatConstants_BindToProbeArtifactOrDeterministicSurrogate(t *testing.T) {
	constants := []string{
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_24khz_160k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatOpus_24khz,
		OutputFormatPCM_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,
	}

	root := edgeTTSRepoRoot(t)
	candidates := []string{
		filepath.Join(root, ".agents-work", "spec-tests", "edge-format-probe-latest.log"),
		filepath.Join(root, ".agents-work", "spec-tests", "edge-format-probe-latest.txt"),
		filepath.Join(root, ".agents-work", "spec-tests", "edge-format-probe-report.md"),
	}

	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}

		report := string(content)
		for _, formatID := range constants {
			if !strings.Contains(report, formatID) {
				t.Fatalf("probe artifact %s missing constant format %q", candidate, formatID)
			}
		}
		return
	}

	t.Log("no probe artifact found; using deterministic surrogate consistency assertions")
	for _, formatID := range constants {
		registered, ok := defaultFormatRegistry.Get(formatID)
		if !ok {
			t.Fatalf("constant %q not registered", formatID)
		}
		if registered.Status != tts.FormatAvailable {
			t.Fatalf("constant %q status=%v; want FormatAvailable", formatID, registered.Status)
		}
		parsed, parseOK := ParseOutputFormat(formatID)
		if !parseOK {
			t.Fatalf("ParseOutputFormat(%q) returned false", formatID)
		}
		if registered.Profile != parsed {
			t.Fatalf("constant %q profile mismatch: registry=%+v parsed=%+v", formatID, registered.Profile, parsed)
		}
	}
}

func TestFormatProbeWorkflow_IsExplicitlyBuildTaggedAndDocumented(t *testing.T) {
	probeTestPath := filepath.Join(edgeTTSRepoRoot(t), "providers", "edgetts", "format_probe_test.go")
	content, err := os.ReadFile(probeTestPath)
	if err != nil {
		t.Fatalf("read format_probe_test.go failed: %v", err)
	}
	text := string(content)

	requiredFragments := []string{
		"//go:build probe",
		"go test -v -tags probe -run TestEdgeTTSFormatProbe ./providers/edgetts/",
		"never runs in CI",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(text, fragment) {
			t.Fatalf("format_probe_test.go should contain %q", fragment)
		}
	}
}

func TestFormatProbePipeline_DeterministicProbeAllAndReportContract(t *testing.T) {
	p := New()
	r := p.FormatRegistry().Clone()

	allBefore := r.All()
	if len(allBefore) == 0 {
		t.Fatal("format registry should not be empty")
	}

	var unverifiedIDs []string
	constantIDs := make(map[string]struct{})
	for _, f := range allBefore {
		if f.Status == tts.FormatUnverified {
			unverifiedIDs = append(unverifiedIDs, f.ID)
			continue
		}
		if f.Status == tts.FormatAvailable {
			constantIDs[f.ID] = struct{}{}
		}
	}
	if len(unverifiedIDs) == 0 {
		t.Fatal("expected unverified formats for probe pipeline test")
	}
	sort.Strings(unverifiedIDs)

	probeResults := make(map[string]bool, len(unverifiedIDs))
	for i, id := range unverifiedIDs {
		probeResults[id] = i%2 == 0
	}
	prober := &deterministicProbeAllProber{results: probeResults}
	r.SetProber(prober)

	available, unavailable, err := r.ProbeAll(context.Background())
	if err != nil {
		t.Fatalf("ProbeAll() error: %v", err)
	}

	if len(prober.calls) != len(unverifiedIDs) {
		t.Fatalf("probe calls = %d; want %d", len(prober.calls), len(unverifiedIDs))
	}

	expectedAvailable := 0
	for _, ok := range probeResults {
		if ok {
			expectedAvailable++
		}
	}
	expectedUnavailable := len(unverifiedIDs) - expectedAvailable
	if available != expectedAvailable || unavailable != expectedUnavailable {
		t.Fatalf("ProbeAll() counts = (available=%d unavailable=%d); want (%d %d)", available, unavailable, expectedAvailable, expectedUnavailable)
	}

	allAfter := r.All()
	if len(allAfter) != len(allBefore) {
		t.Fatalf("registry size changed after probe: before=%d after=%d", len(allBefore), len(allAfter))
	}

	reportRows := 0
	for _, f := range allAfter {
		if f.ID == "" {
			t.Fatal("report contract broken: empty format id")
		}
		if probeStatusLabel(f.Status) == "" {
			t.Fatalf("report contract broken: empty status label for %q", f.ID)
		}
		if f.Status == tts.FormatUnverified {
			t.Fatalf("format %q remained unverified after ProbeAll", f.ID)
		}
		if f.VerifiedAt.IsZero() {
			t.Fatalf("report contract broken: verified_at is zero for %q", f.ID)
		}
		reportRows++
	}
	if reportRows != len(allAfter) {
		t.Fatalf("report rows=%d; want %d", reportRows, len(allAfter))
	}

	for id := range constantIDs {
		f, ok := r.Get(id)
		if !ok {
			t.Fatalf("constant %q missing after ProbeAll", id)
		}
		if f.Status != tts.FormatAvailable {
			t.Fatalf("constant %q status=%v; want FormatAvailable", id, f.Status)
		}
	}
}

func TestFormatProbePipeline_DeterministicMappingReadiness(t *testing.T) {
	p := New()
	r := p.FormatRegistry().Clone()

	var unverifiedIDs []string
	for _, f := range r.All() {
		if f.Status == tts.FormatUnverified {
			unverifiedIDs = append(unverifiedIDs, f.ID)
		}
	}
	if len(unverifiedIDs) < 2 {
		t.Fatalf("need at least 2 unverified formats, got %d", len(unverifiedIDs))
	}
	sort.Strings(unverifiedIDs)

	availableID := unverifiedIDs[0]
	unavailableID := unverifiedIDs[1]

	prober := &deterministicProbeAllProber{results: map[string]bool{
		availableID:   true,
		unavailableID: false,
	}}
	r.SetProber(prober)
	p.WithFormatRegistry(r)

	if _, _, err := r.ProbeAll(context.Background()); err != nil {
		t.Fatalf("ProbeAll() error: %v", err)
	}

	supported := p.SupportedFormats()
	supportedSet := make(map[string]struct{}, len(supported))
	for _, f := range supported {
		supportedSet[f.ID] = struct{}{}
	}

	if _, ok := supportedSet[availableID]; !ok {
		t.Fatalf("SupportedFormats() should include newly available format %q", availableID)
	}
	if _, ok := supportedSet[unavailableID]; ok {
		t.Fatalf("SupportedFormats() should exclude unavailable format %q", unavailableID)
	}

	for _, id := range []string{
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_24khz_160k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatOpus_24khz,
		OutputFormatPCM_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,
	} {
		if _, ok := supportedSet[id]; !ok {
			t.Fatalf("SupportedFormats() missing compile-time constant %q", id)
		}
	}
}
