package edgetts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
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

func cloneRegistryForDeterministicProbeTests(src *tts.FormatRegistry) *tts.FormatRegistry {
	registry := tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
		tts.WithProbeInterval(1*time.Millisecond),
	)
	declared := src.Declared()
	if len(declared) > 0 {
		registry.Register(declared...)
	}

	probeResults := make(map[string]bool)
	for _, format := range src.All() {
		if src.IsDeclared(format.ID) {
			continue
		}

		if _, ok := registry.Get(format.ID); !ok {
			continue
		}

		switch format.Status {
		case tts.FormatAvailable:
			probeResults[format.ID] = true
		case tts.FormatUnavailable:
			probeResults[format.ID] = false
		}
	}

	if len(probeResults) > 0 {
		registry.SetProber(&deterministicProbeAllProber{results: probeResults})
		for formatID := range probeResults {
			if _, err := registry.Probe(context.Background(), formatID); err != nil {
				panic("cloneRegistryForDeterministicProbeTests: failed to preserve probe state")
			}
		}
		registry.SetProber(nil)
	}
	return registry
}

func newDefaultFormatRegistryFixture() *tts.FormatRegistry {
	return defaultFormatRegistry.CloneDeclaredClean()
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
		{name: "const/opus-24khz", format: OutputFormatOpus_24khz, wantOK: true, wantFormat: "opus", wantRate: tts.SampleRate48kHz, wantCh: 1},
		{name: "const/pcm-24khz", format: OutputFormatPCM_24khz, wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 24000, wantCh: 1, wantLL: true},
		{name: "const/webm-24khz", format: OutputFormatWebM_24khz, wantOK: true, wantFormat: "webm", wantRate: tts.SampleRate48kHz, wantCh: 1},
		{name: "const/ogg-24khz", format: OutputFormatOgg_24khz, wantOK: true, wantFormat: "ogg", wantRate: tts.SampleRate48kHz, wantCh: 1},

		// ── Unverified: MP3 formats ──────────────────────────────────
		{name: "unverified/mp3-16khz-32k", format: "audio-16khz-32kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 32},
		{name: "unverified/mp3-16khz-64k", format: "audio-16khz-64kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 64},
		{name: "unverified/mp3-16khz-128k", format: "audio-16khz-128kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 16000, wantCh: 1, wantBR: 128},
		{name: "unverified/mp3-48khz-96k", format: "audio-48khz-96kbitrate-mono-mp3", wantOK: true, wantFormat: tts.AudioFormatMP3, wantRate: 48000, wantCh: 1, wantBR: 96},

		// ── Unverified: WebM ─────────────────────────────────────────
		{name: "unverified/webm-16khz", format: "webm-16khz-16bit-mono-opus", wantOK: true, wantFormat: "webm", wantRate: tts.SampleRate48kHz, wantCh: 1},

		// ── Unverified: Ogg ──────────────────────────────────────────
		{name: "unverified/ogg-16khz", format: "ogg-16khz-16bit-mono-opus", wantOK: true, wantFormat: "ogg", wantRate: tts.SampleRate48kHz, wantCh: 1},
		{name: "unverified/ogg-48khz", format: "ogg-48khz-16bit-mono-opus", wantOK: true, wantFormat: "ogg", wantRate: 48000, wantCh: 1},

		// ── Unverified: Opus raw ─────────────────────────────────────
		{name: "unverified/opus-16khz-32kbps", format: "audio-16khz-16bit-32kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: tts.SampleRate48kHz, wantCh: 1},
		{name: "unverified/opus-24khz-24kbps", format: "audio-24khz-16bit-24kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: tts.SampleRate48kHz, wantCh: 1},
		{name: "unverified/opus-24khz-48kbps", format: "audio-24khz-16bit-48kbps-mono-opus", wantOK: true, wantFormat: "opus", wantRate: tts.SampleRate48kHz, wantCh: 1},

		// ── Unverified: Raw PCM / special ────────────────────────────
		{name: "unverified/raw-8khz-alaw", format: "raw-8khz-8bit-mono-alaw", wantOK: true, wantFormat: "alaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/raw-8khz-mulaw", format: "raw-8khz-8bit-mono-mulaw", wantOK: true, wantFormat: "mulaw", wantRate: 8000, wantCh: 1},
		{name: "unverified/raw-8khz-pcm", format: "raw-8khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 8000, wantCh: 1, wantLL: true},
		{name: "unverified/raw-16khz-pcm", format: "raw-16khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 16000, wantCh: 1, wantLL: true},
		{name: "unverified/raw-16khz-truesilk", format: "raw-16khz-16bit-mono-truesilk", wantOK: true, wantFormat: "silk", wantRate: 16000, wantCh: 1},
		{name: "unverified/raw-22050hz-pcm", format: "raw-22050hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 22050, wantCh: 1, wantLL: true},
		{name: "unverified/raw-24khz-truesilk", format: "raw-24khz-16bit-mono-truesilk", wantOK: true, wantFormat: "silk", wantRate: 24000, wantCh: 1},
		{name: "unverified/raw-44100hz-pcm", format: "raw-44100hz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 44100, wantCh: 1, wantLL: true},
		{name: "unverified/raw-48khz-pcm", format: "raw-48khz-16bit-mono-pcm", wantOK: true, wantFormat: tts.AudioFormatPCM, wantRate: 48000, wantCh: 1, wantLL: true},

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

func TestParseOutputFormat_DistinguishesRawPCMAndRiffPCM(t *testing.T) {
	raw, ok := ParseOutputFormat("raw-24khz-16bit-mono-pcm")
	if !ok {
		t.Fatal("ParseOutputFormat(raw) returned ok=false")
	}
	if raw.Format != tts.AudioFormatPCM {
		t.Fatalf("raw format = %q, want %q", raw.Format, tts.AudioFormatPCM)
	}

	riff, ok := ParseOutputFormat("riff-24khz-16bit-mono-pcm")
	if !ok {
		t.Fatal("ParseOutputFormat(riff) returned ok=false")
	}
	if riff.Format != tts.AudioFormatWAV {
		t.Fatalf("riff format = %q, want %q", riff.Format, tts.AudioFormatWAV)
	}
}

func TestParseOutputFormat_NormalizesOpusSampleRateTo48kHz(t *testing.T) {
	formats := []string{
		OutputFormatOpus_24khz,
		OutputFormatWebM_24khz,
		OutputFormatOgg_24khz,
		"audio-16khz-16bit-32kbps-mono-opus",
		"webm-16khz-16bit-mono-opus",
		"ogg-16khz-16bit-mono-opus",
	}

	for _, formatID := range formats {
		t.Run(formatID, func(t *testing.T) {
			profile, ok := ParseOutputFormat(formatID)
			if !ok {
				t.Fatalf("ParseOutputFormat(%q) returned ok=false", formatID)
			}
			if profile.SampleRate != tts.SampleRate48kHz {
				t.Fatalf("SampleRate = %d, want %d", profile.SampleRate, tts.SampleRate48kHz)
			}
		})
	}
}

// TestDefaultFormatRegistry_Constants verifies all 9 documented constants are
// registered as FormatUnverified until explicitly probed, and that their
// profiles match ParseOutputFormat.
func TestDefaultFormatRegistry_Constants(t *testing.T) {
	registry := newDefaultFormatRegistryFixture()
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
			f, ok := registry.Get(id)
			if !ok {
				t.Fatalf("constant %q not found in registry", id)
			}
			if f.Status != tts.FormatUnverified {
				t.Errorf("status = %v; want FormatUnverified", f.Status)
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
	registry := newDefaultFormatRegistryFixture()
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
			f, ok := registry.Get(id)
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
	registry := newDefaultFormatRegistryFixture()
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

	all := registry.All()
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
	registry := newDefaultFormatRegistryFixture()
	// An unknown format string parseable by ParseOutputFormat should auto-register.
	id := "audio-96khz-512kbitrate-mono-mp3"
	f, ok := registry.Get(id)
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

// TestSupportedFormats_ExcludesUndeclaredAvailableFormats verifies that
// Provider.SupportedFormats only exposes declared formats that are currently
// available, even if the registry contains other available auto-probed IDs.
func TestSupportedFormats_ExcludesUndeclaredAvailableFormats(t *testing.T) {
	p := New()
	autoParsedID := "audio-96khz-512kbitrate-mono-mp3"
	registry := cloneRegistryForDeterministicProbeTests(p.FormatRegistry())
	p.WithFormatRegistry(registry)
	p.FormatRegistry().Register(tts.OutputFormat{ID: OutputFormatMP3_24khz_48k, Status: tts.FormatAvailable})
	p.FormatRegistry().SetProber(&deterministicProbeAllProber{results: map[string]bool{autoParsedID: true}})

	probed, err := p.FormatRegistry().Probe(context.Background(), autoParsedID)
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if probed.Status != tts.FormatAvailable {
		t.Fatalf("probed status = %v; want %v", probed.Status, tts.FormatAvailable)
	}

	supported := p.SupportedFormats()
	if len(supported) != 1 {
		t.Fatalf("SupportedFormats() returned %d formats; want 1 declared available format", len(supported))
	}
	if supported[0].ID != OutputFormatMP3_24khz_48k {
		t.Fatalf("SupportedFormats()[0] = %q; want %q", supported[0].ID, OutputFormatMP3_24khz_48k)
	}
}

func TestProvider_FormatRegistryAccessors(t *testing.T) {
	p := New()
	if p.FormatRegistry() == nil {
		t.Fatal("FormatRegistry() returned nil")
	}

	// SupportedFormats should be empty before any explicit probe.
	formats := p.SupportedFormats()
	if len(formats) != 0 {
		t.Errorf("SupportedFormats() returned %d formats; want 0 before probe", len(formats))
	}

	// Verify every constant is still present in the registry catalog.
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
		f, ok := p.FormatRegistry().Get(id)
		if !ok {
			t.Errorf("constant %q missing from FormatRegistry()", id)
			continue
		}
		if f.Status != tts.FormatUnverified {
			t.Errorf("constant %q status=%v; want FormatUnverified before probe", id, f.Status)
		}
	}

	// WithFormatRegistry should adopt a cloned registry with a provider-bound prober.
	custom := tts.NewFormatRegistry()
	p.WithFormatRegistry(custom)
	if p.FormatRegistry() == custom {
		t.Error("WithFormatRegistry should clone the provided registry to keep provider-owned prober state isolated")
	}
	if !p.FormatRegistry().HasProber() {
		t.Fatal("WithFormatRegistry should inject a prober into the adopted registry")
	}
}

// TestDefaultFormatRegistry_TotalCount verifies the clean catalog fixture has
// no leaked runtime state and contains only declared formats.
func TestDefaultFormatRegistry_TotalCount(t *testing.T) {
	registry := newDefaultFormatRegistryFixture()
	all := registry.All()
	declared := registry.Declared()

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

	if available != 0 {
		t.Errorf("available formats = %d; want 0 before probe", available)
	}
	if unverified != len(all) {
		t.Errorf("unverified formats = %d; want %d", unverified, len(all))
	}
	if len(all) != len(declared) {
		t.Errorf("registry size = %d; want %d declared formats in clean fixture", len(all), len(declared))
	}
}

func TestOutputFormatConstants_DefaultToUnverifiedWithoutProbe(t *testing.T) {
	registry := newDefaultFormatRegistryFixture()
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

	for _, formatID := range constants {
		registered, ok := registry.Get(formatID)
		if !ok {
			t.Fatalf("constant %q not registered", formatID)
		}
		if registered.Status != tts.FormatUnverified {
			t.Fatalf("constant %q status=%v; want FormatUnverified", formatID, registered.Status)
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

func splitFormatsByVerificationStatus(formats []tts.OutputFormat) []string {
	var unverifiedIDs []string

	for _, format := range formats {
		if format.Status == tts.FormatUnverified {
			unverifiedIDs = append(unverifiedIDs, format.ID)
		}
	}

	sort.Strings(unverifiedIDs)
	return unverifiedIDs
}

func buildAlternatingProbeResults(unverifiedIDs []string) map[string]bool {
	results := make(map[string]bool, len(unverifiedIDs))
	for index, id := range unverifiedIDs {
		results[id] = index%2 == 0
	}
	return results
}

func assertProbeAllCounts(t *testing.T, available, unavailable int, probeResults map[string]bool) {
	t.Helper()

	expectedAvailable := 0
	for _, ok := range probeResults {
		if ok {
			expectedAvailable++
		}
	}
	expectedUnavailable := len(probeResults) - expectedAvailable
	if available != expectedAvailable || unavailable != expectedUnavailable {
		t.Fatalf("ProbeAll() counts = (available=%d unavailable=%d); want (%d %d)", available, unavailable, expectedAvailable, expectedUnavailable)
	}
}

func assertProbeReportContract(t *testing.T, allBefore, allAfter []tts.OutputFormat) {
	t.Helper()

	if len(allAfter) != len(allBefore) {
		t.Fatalf("registry size changed after probe: before=%d after=%d", len(allBefore), len(allAfter))
	}

	reportRows := 0
	for _, format := range allAfter {
		if format.ID == "" {
			t.Fatal("report contract broken: empty format id")
		}
		if probeStatusLabel(format.Status) == "" {
			t.Fatalf("report contract broken: empty status label for %q", format.ID)
		}
		if format.Status == tts.FormatUnverified {
			t.Fatalf("format %q remained unverified after ProbeAll", format.ID)
		}
		if format.VerifiedAt.IsZero() {
			t.Fatalf("report contract broken: verified_at is zero for %q", format.ID)
		}
		reportRows++
	}

	if reportRows != len(allAfter) {
		t.Fatalf("report rows=%d; want %d", reportRows, len(allAfter))
	}
}

func documentedConstantIDs() map[string]struct{} {
	ids := make(map[string]struct{}, len(edgeTTSKnownFormats))
	for _, format := range edgeTTSKnownFormats {
		ids[format.id] = struct{}{}
	}
	return ids
}

func assertDocumentedConstantsProbeContract(t *testing.T, registry *tts.FormatRegistry, probeResults map[string]bool, constantIDs map[string]struct{}) {
	t.Helper()

	for id := range constantIDs {
		format, ok := registry.Get(id)
		if !ok {
			t.Fatalf("constant %q missing after ProbeAll", id)
		}
		expectedAvailable, ok := probeResults[id]
		if !ok {
			t.Fatalf("constant %q missing from deterministic probe result map", id)
		}
		expectedStatus := tts.FormatUnavailable
		if expectedAvailable {
			expectedStatus = tts.FormatAvailable
		}
		if format.Status != expectedStatus {
			t.Fatalf("constant %q status=%v; want %v", id, format.Status, expectedStatus)
		}
		if format.VerifiedAt.IsZero() {
			t.Fatalf("constant %q verified_at is zero after ProbeAll", id)
		}
	}
}

func TestFormatProbePipeline_DeterministicProbeAllAndReportContract(t *testing.T) {
	p := New()
	r := cloneRegistryForDeterministicProbeTests(p.FormatRegistry())

	allBefore := r.All()
	if len(allBefore) == 0 {
		t.Fatal("format registry should not be empty")
	}

	unverifiedIDs := splitFormatsByVerificationStatus(allBefore)
	if len(unverifiedIDs) == 0 {
		t.Fatal("expected unverified formats for probe pipeline test")
	}
	constantIDs := documentedConstantIDs()
	if len(constantIDs) == 0 {
		t.Fatal("expected documented constants for probe pipeline test")
	}
	for id := range constantIDs {
		format, ok := r.Get(id)
		if !ok {
			t.Fatalf("documented constant %q missing before ProbeAll", id)
		}
		if format.Status != tts.FormatUnverified {
			t.Fatalf("documented constant %q status=%v before ProbeAll; want %v", id, format.Status, tts.FormatUnverified)
		}
	}

	probeResults := buildAlternatingProbeResults(unverifiedIDs)
	prober := &deterministicProbeAllProber{results: probeResults}
	r.SetProber(prober)

	available, unavailable, err := r.ProbeAll(context.Background())
	if err != nil {
		t.Fatalf("ProbeAll() error: %v", err)
	}

	if len(prober.calls) != len(unverifiedIDs) {
		t.Fatalf("probe calls = %d; want %d", len(prober.calls), len(unverifiedIDs))
	}

	assertProbeAllCounts(t, available, unavailable, probeResults)
	assertProbeReportContract(t, allBefore, r.All())
	assertDocumentedConstantsProbeContract(t, r, probeResults, constantIDs)
}

func TestFormatProbePipeline_DeterministicMappingReadiness(t *testing.T) {
	p := New()
	p.WithFormatRegistry(cloneRegistryForDeterministicProbeTests(p.FormatRegistry()))
	r := p.FormatRegistry()

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
}

func TestResolveOutputFormat_RejectsUnavailableFormat(t *testing.T) {
	p := New()
	p.FormatRegistry().Register(tts.OutputFormat{
		ID:         OutputFormatMP3_24khz_48k,
		Profile:    tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48},
		Status:     tts.FormatUnavailable,
		VerifiedAt: time.Now(),
	})

	_, err := p.resolveOutputFormat(OutputFormatMP3_24khz_48k)
	if err == nil {
		t.Fatal("expected unavailable format to be rejected")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_DefaultPathHonorsRegistryState(t *testing.T) {
	t.Run("default format marked unavailable", func(t *testing.T) {
		p := New()
		p.FormatRegistry().Register(tts.OutputFormat{
			ID:         defaultOutputFormat,
			Profile:    tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48},
			Status:     tts.FormatUnavailable,
			VerifiedAt: time.Now(),
		})

		_, err := p.resolveOutputFormat("")
		if err == nil {
			t.Fatal("expected default format to be rejected when registry marks it unavailable")
		}
		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("expected *tts.Error, got %T", err)
		}
		if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
			t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
		}
	})

	t.Run("default format undeclared in active registry", func(t *testing.T) {
		p := New()
		r := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
		r.Register(tts.OutputFormat{ID: OutputFormatMP3_48khz_192k, Status: tts.FormatUnverified})
		p.WithFormatRegistry(r)

		_, err := p.resolveOutputFormat("")
		if err == nil {
			t.Fatal("expected undeclared default format to be rejected")
		}
		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("expected *tts.Error, got %T", err)
		}
		if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
			t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
		}
	})
}

func TestResolveOutputFormat_RejectsUndeclaredFormat(t *testing.T) {
	p := New()

	_, err := p.resolveOutputFormat("audio-96khz-999kbitrate-mono-mp3")
	if err == nil {
		t.Fatal("expected undeclared format to be rejected")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_AllowsDeclaredUnverifiedFormat(t *testing.T) {
	p := New()

	resolved, err := p.resolveOutputFormat(OutputFormatMP3_48khz_192k)
	if err != nil {
		t.Fatalf("resolveOutputFormat() error: %v", err)
	}
	if resolved != OutputFormatMP3_48khz_192k {
		t.Fatalf("resolved format = %q; want %q", resolved, OutputFormatMP3_48khz_192k)
	}
}

func TestResolveOutputFormat_AllowsExplicitDeclaredFormatFromRegistryHandoff(t *testing.T) {
	customID := "audio-96khz-512kbitrate-mono-mp3"
	profile, ok := ParseOutputFormat(customID)
	if !ok {
		t.Fatalf("ParseOutputFormat(%q) returned false", customID)
	}

	r := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
	r.Register(tts.OutputFormat{ID: customID, Profile: profile, Status: tts.FormatUnverified})

	p := New().WithFormatRegistry(r)
	resolved, err := p.resolveOutputFormat(customID)
	if err != nil {
		t.Fatalf("resolveOutputFormat() error: %v", err)
	}
	if resolved != customID {
		t.Fatalf("resolved format = %q; want %q", resolved, customID)
	}
}

func TestResolveOutputFormat_RejectsRegistryAutoParsedFormat(t *testing.T) {
	p := New()
	autoParsedID := "audio-96khz-512kbitrate-mono-mp3"

	if _, ok := p.FormatRegistry().Get(autoParsedID); !ok {
		t.Fatalf("expected %q to auto-parse into the registry", autoParsedID)
	}

	_, err := p.resolveOutputFormat(autoParsedID)
	if err == nil {
		t.Fatal("expected auto-parsed undeclared format to be rejected")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_RejectsAutoParsedFormatAfterRegistryHandoff(t *testing.T) {
	autoParsedID := "audio-96khz-512kbitrate-mono-mp3"
	r := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
	if _, ok := r.Get(autoParsedID); !ok {
		t.Fatalf("expected %q to auto-parse into source registry", autoParsedID)
	}

	p := New().WithFormatRegistry(r)
	_, err := p.resolveOutputFormat(autoParsedID)
	if err == nil {
		t.Fatal("expected auto-parsed format to remain undeclared after registry handoff")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_AllowsExpiredUnavailableDeclaredFormat(t *testing.T) {
	p := New()
	registry := tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
		tts.WithProbeTTL(time.Millisecond),
	)
	profile, ok := ParseOutputFormat(OutputFormatMP3_24khz_48k)
	if !ok {
		t.Fatalf("ParseOutputFormat(%q) returned false", OutputFormatMP3_24khz_48k)
	}
	registry.Register(tts.OutputFormat{
		ID:         OutputFormatMP3_24khz_48k,
		Profile:    profile,
		Status:     tts.FormatUnavailable,
		VerifiedAt: time.Now().Add(-time.Second),
	})
	p.formatRegistry = registry

	resolved, err := p.resolveOutputFormat(OutputFormatMP3_24khz_48k)
	if err != nil {
		t.Fatalf("resolveOutputFormat() error: %v", err)
	}
	if resolved != OutputFormatMP3_24khz_48k {
		t.Fatalf("resolved format = %q; want %q", resolved, OutputFormatMP3_24khz_48k)
	}
}

func TestResolveOutputFormat_RejectsUndeclaredFormatAfterSuccessfulProbe(t *testing.T) {
	p := New()
	probeFormat := "audio-96khz-512kbitrate-mono-mp3"

	p.connectHook = func(context.Context) (*websocket.Conn, error) {
		return newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
			if _, _, err := ws.Read(ctx); err != nil {
				t.Errorf("read config: %v", err)
				return
			}
			if _, _, err := ws.Read(ctx); err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageBinary, []byte{0x00, 0x00, 'o', 'k'}); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}

	probed, err := p.FormatRegistry().Probe(context.Background(), probeFormat)
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if probed.Status != tts.FormatAvailable {
		t.Fatalf("status = %v; want %v", probed.Status, tts.FormatAvailable)
	}

	_, err = p.resolveOutputFormat(probeFormat)
	if err == nil {
		t.Fatal("expected successfully probed undeclared format to remain rejected for synthesis")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_RejectsAutoProbedFormatAfterRegistryHandoff(t *testing.T) {
	probeFormat := "audio-96khz-512kbitrate-mono-mp3"
	sourceProvider := New()
	sourceProvider.connectHook = func(context.Context) (*websocket.Conn, error) {
		return newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
			if _, _, err := ws.Read(ctx); err != nil {
				t.Errorf("read config: %v", err)
				return
			}
			if _, _, err := ws.Read(ctx); err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageBinary, []byte{0x00, 0x00, 'o', 'k'}); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}

	if _, err := sourceProvider.FormatRegistry().Probe(context.Background(), probeFormat); err != nil {
		t.Fatalf("Probe() error: %v", err)
	}

	p := New().WithFormatRegistry(sourceProvider.FormatRegistry())
	_, err := p.resolveOutputFormat(probeFormat)
	if err == nil {
		t.Fatal("expected auto-probed format to remain undeclared after registry handoff")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
}

func TestResolveOutputFormat_AllowsLiveRegistryRegistrationAfterHandoff(t *testing.T) {
	p := New()
	formatID := "audio-96khz-512kbitrate-mono-mp3"
	profile, ok := ParseOutputFormat(formatID)
	if !ok {
		t.Fatalf("ParseOutputFormat(%q) returned false", formatID)
	}

	p.FormatRegistry().Register(tts.OutputFormat{ID: formatID, Profile: profile, Status: tts.FormatUnverified})

	resolved, err := p.resolveOutputFormat(formatID)
	if err != nil {
		t.Fatalf("resolveOutputFormat() error: %v", err)
	}
	if resolved != formatID {
		t.Fatalf("resolved format = %q; want %q", resolved, formatID)
	}
}

func TestDefaultRegistry_192kbpsProfile(t *testing.T) {
	p := New()
	f, ok := p.FormatRegistry().Get(OutputFormatMP3_48khz_192k)
	if !ok {
		t.Fatalf("192kbps format %q not found in registry", OutputFormatMP3_48khz_192k)
	}
	if f.Profile.Bitrate != 192 {
		t.Errorf("Profile.Bitrate = %d; want 192", f.Profile.Bitrate)
	}
	if f.Profile.SampleRate != tts.SampleRate48kHz {
		t.Errorf("Profile.SampleRate = %d; want %d", f.Profile.SampleRate, tts.SampleRate48kHz)
	}
	if f.Profile.Format != tts.AudioFormatMP3 {
		t.Errorf("Profile.Format = %q; want %q", f.Profile.Format, tts.AudioFormatMP3)
	}
	if f.Status != tts.FormatUnverified {
		t.Errorf("Status = %v; want FormatUnverified", f.Status)
	}
}

func TestWithFormatRegistry_ClonesSharedRegistryPerProvider(t *testing.T) {
	shared := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
	shared.Register(tts.OutputFormat{ID: OutputFormatMP3_24khz_48k, Status: tts.FormatUnverified})

	providerA := New().WithClientToken("token-a")
	providerA.WithFormatRegistry(shared)

	providerB := New().WithClientToken("token-b")
	providerB.WithFormatRegistry(providerA.FormatRegistry())

	if providerA.FormatRegistry() == providerB.FormatRegistry() {
		t.Fatal("providers should not share the same adopted format registry instance")
	}
	if providerA.FormatRegistry() == shared || providerB.FormatRegistry() == shared {
		t.Fatal("providers should not retain the caller's registry pointer")
	}
	if !providerA.FormatRegistry().HasProber() || !providerB.FormatRegistry().HasProber() {
		t.Fatal("each provider-owned registry should have a prober installed")
	}

	formatA, ok := providerA.FormatRegistry().Get(OutputFormatMP3_24khz_48k)
	if !ok {
		t.Fatalf("providerA registry missing format %q", OutputFormatMP3_24khz_48k)
	}
	formatB, ok := providerB.FormatRegistry().Get(OutputFormatMP3_24khz_48k)
	if !ok {
		t.Fatalf("providerB registry missing format %q", OutputFormatMP3_24khz_48k)
	}
	if formatA.Status != tts.FormatUnverified || formatB.Status != tts.FormatUnverified {
		t.Fatal("adopted registries should preserve declared formats while starting from clean probe state")
	}
}

func TestWithFormatRegistry_ClearsRuntimeProbeStateFromDeclaredFormats(t *testing.T) {
	formatID := OutputFormatMP3_24khz_48k
	source := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
	source.Register(tts.OutputFormat{
		ID:         formatID,
		Profile:    tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48},
		Status:     tts.FormatUnavailable,
		VerifiedAt: time.Now(),
	})
	if _, ok := source.Get("audio-96khz-512kbitrate-mono-mp3"); !ok {
		t.Fatal("expected auto-parse to seed an undeclared source entry")
	}

	p := New().WithFormatRegistry(source)

	adopted, ok := p.FormatRegistry().Get(formatID)
	if !ok {
		t.Fatalf("adopted registry missing declared format %q", formatID)
	}
	if adopted.Status != tts.FormatUnverified {
		t.Fatalf("adopted status = %v; want %v", adopted.Status, tts.FormatUnverified)
	}
	if !adopted.VerifiedAt.IsZero() {
		t.Fatal("adopted declared format should not retain probe timestamp")
	}
	if p.FormatRegistry().IsDeclared("audio-96khz-512kbitrate-mono-mp3") {
		t.Fatal("adopted registry should not treat undeclared runtime entries as declared")
	}

	resolved, err := p.resolveOutputFormat(formatID)
	if err != nil {
		t.Fatalf("resolveOutputFormat() error: %v", err)
	}
	if resolved != formatID {
		t.Fatalf("resolved format = %q; want %q", resolved, formatID)
	}
	if supported := p.SupportedFormats(); len(supported) != 0 {
		t.Fatalf("SupportedFormats() len = %d; want 0 before re-probe", len(supported))
	}
	if _, err := p.resolveOutputFormat("audio-96khz-512kbitrate-mono-mp3"); err == nil {
		t.Fatal("resolveOutputFormat() should still reject undeclared runtime-derived formats after handoff")
	}
}

func TestFormatRegistryProbe_AllowsUndeclaredParseableFormatThroughProviderProber(t *testing.T) {
	p := New()
	probeFormat := "audio-96khz-512kbitrate-mono-mp3"

	var configMessages []string
	p.connectHook = func(context.Context) (*websocket.Conn, error) {
		return newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
			_, configMessage, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read config: %v", err)
				return
			}
			configMessages = append(configMessages, string(configMessage))

			_, ssmlMessage, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}
			if !strings.Contains(string(ssmlMessage), ">test<") {
				t.Errorf("ssml message did not contain probe text: %s", string(ssmlMessage))
				return
			}

			if err := ws.Write(ctx, websocket.MessageBinary, []byte{0x00, 0x00, 'o', 'k'}); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}

	probed, err := p.FormatRegistry().Probe(context.Background(), probeFormat)
	if err != nil {
		t.Fatalf("Probe() error: %v", err)
	}
	if probed.Status != tts.FormatAvailable {
		t.Fatalf("status = %v; want %v", probed.Status, tts.FormatAvailable)
	}
	if probed.Profile.SampleRate != 96000 {
		t.Fatalf("SampleRate = %d; want 96000", probed.Profile.SampleRate)
	}
	if len(configMessages) != 1 {
		t.Fatalf("config message count = %d; want 1", len(configMessages))
	}
	if !strings.Contains(configMessages[0], `"outputFormat":"`+probeFormat+`"`) {
		t.Fatalf("config message did not use probed format: %s", configMessages[0])
	}
	stored, ok := p.FormatRegistry().Get(probeFormat)
	if !ok {
		t.Fatalf("expected probed format %q to be auto-registered", probeFormat)
	}
	if stored.Status != tts.FormatAvailable {
		t.Fatalf("stored status = %v; want %v", stored.Status, tts.FormatAvailable)
	}
}

func TestClassifyWebsocketReadError_ClassifiesUnsupportedOutputFormatCloseReason(t *testing.T) {
	err := classifyWebsocketReadError(&websocket.CloseError{
		Code:   websocket.StatusInvalidFramePayloadData,
		Reason: "Unsupported Edge output format: " + OutputFormatMP3_48khz_192k + ".",
	}, nil, time.Second, "websocket read error while receiving audio")

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedFormat {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeUnsupportedFormat)
	}
	if !strings.Contains(ttsErr.Message, OutputFormatMP3_48khz_192k) {
		t.Fatalf("message %q should include rejected format id", ttsErr.Message)
	}
	if tts.IsRetryableError(err) {
		t.Fatal("unsupported output format close reason should not remain retryable")
	}
}

func TestFormatRegistryProbeAll_ClassifiesServiceUnsupportedFormatsAsUnavailable(t *testing.T) {
	p := New()
	r := tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
		tts.WithProbeInterval(time.Millisecond),
	)
	r.Register(
		tts.OutputFormat{ID: OutputFormatMP3_24khz_48k, Status: tts.FormatUnverified},
		tts.OutputFormat{ID: OutputFormatMP3_48khz_192k, Status: tts.FormatUnverified},
	)
	p.WithFormatRegistry(r)

	attempts := map[string]int{}
	p.connectHook = func(context.Context) (*websocket.Conn, error) {
		return newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
			_, configMessage, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read config: %v", err)
				return
			}

			config := string(configMessage)
			formatID := ""
			switch {
			case strings.Contains(config, OutputFormatMP3_24khz_48k):
				formatID = OutputFormatMP3_24khz_48k
			case strings.Contains(config, OutputFormatMP3_48khz_192k):
				formatID = OutputFormatMP3_48khz_192k
			default:
				t.Errorf("unexpected config message: %s", config)
				return
			}
			attempts[formatID]++

			_, ssmlMessage, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}
			if !strings.Contains(string(ssmlMessage), ">test<") {
				t.Errorf("ssml message did not contain probe text: %s", string(ssmlMessage))
				return
			}

			if formatID == OutputFormatMP3_48khz_192k {
				closeErr := ws.Close(websocket.StatusInvalidFramePayloadData, "Unsupported Edge output format: "+formatID+".")
				if closeErr != nil {
					t.Errorf("close websocket: %v", closeErr)
				}
				return
			}

			if err := ws.Write(ctx, websocket.MessageBinary, []byte{0x00, 0x00, 'o', 'k'}); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}

	available, unavailable, err := p.FormatRegistry().ProbeAll(context.Background())
	if err != nil {
		t.Fatalf("ProbeAll() error: %v", err)
	}
	if available != 1 {
		t.Fatalf("available = %d; want 1", available)
	}
	if unavailable != 1 {
		t.Fatalf("unavailable = %d; want 1", unavailable)
	}
	if attempts[OutputFormatMP3_24khz_48k] != 1 {
		t.Fatalf("available format attempts = %d; want 1", attempts[OutputFormatMP3_24khz_48k])
	}
	if attempts[OutputFormatMP3_48khz_192k] != 1 {
		t.Fatalf("unsupported format attempts = %d; want 1", attempts[OutputFormatMP3_48khz_192k])
	}

	availableFormat, ok := p.FormatRegistry().Get(OutputFormatMP3_24khz_48k)
	if !ok {
		t.Fatalf("expected %q in registry", OutputFormatMP3_24khz_48k)
	}
	if availableFormat.Status != tts.FormatAvailable {
		t.Fatalf("status(%q) = %v; want %v", OutputFormatMP3_24khz_48k, availableFormat.Status, tts.FormatAvailable)
	}

	unsupportedFormat, ok := p.FormatRegistry().Get(OutputFormatMP3_48khz_192k)
	if !ok {
		t.Fatalf("expected %q in registry", OutputFormatMP3_48khz_192k)
	}
	if unsupportedFormat.Status != tts.FormatUnavailable {
		t.Fatalf("status(%q) = %v; want %v", OutputFormatMP3_48khz_192k, unsupportedFormat.Status, tts.FormatUnavailable)
	}
	if unsupportedFormat.VerifiedAt.IsZero() {
		t.Fatalf("verified_at for %q should be set", OutputFormatMP3_48khz_192k)
	}

	supported := p.SupportedFormats()
	if len(supported) != 1 || supported[0].ID != OutputFormatMP3_24khz_48k {
		t.Fatalf("SupportedFormats() = %+v; want only %q", supported, OutputFormatMP3_24khz_48k)
	}
}
