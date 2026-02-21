//go:build probe

package edgetts

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

// TestEdgeTTSFormatProbe probes all registered Edge TTS output formats and
// prints a status report. Run with:
//
//	go test -v -tags probe -run TestEdgeTTSFormatProbe ./providers/edgetts/
//
// The test is guarded by the "probe" build tag so it never runs in CI.
func TestEdgeTTSFormatProbe(t *testing.T) {
	provider := New()
	registry := provider.FormatRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Snapshot all formats before probing.
	allFormats := registry.All()
	t.Logf("Registered formats: %d", len(allFormats))

	// Probe all unverified formats via the registry infrastructure
	// (interval between probes is controlled by WithProbeInterval).
	start := time.Now()
	available, unavailable, err := registry.ProbeAll(ctx)
	elapsed := time.Since(start)

	t.Logf("ProbeAll completed in %s — available: %d, unavailable: %d", elapsed.Round(time.Millisecond), available, unavailable)
	if err != nil {
		t.Logf("ProbeAll error (partial results may follow): %v", err)
	}

	// Build report table.
	allFormats = registry.All() // re-read after probing
	separator := strings.Repeat("-", 100)

	t.Log("")
	t.Logf("%-55s %-14s %-8s %-6s %-6s %s",
		"FORMAT", "STATUS", "CODEC", "RATE", "KBPS", "VERIFIED")
	t.Log(separator)

	for _, f := range allFormats {
		status := statusString(f.Status)
		codec := f.Profile.Format
		if codec == "" {
			codec = "?"
		}
		rate := "?"
		if f.Profile.SampleRate > 0 {
			rate = fmt.Sprintf("%dk", f.Profile.SampleRate/1000)
		}
		kbps := "-"
		if f.Profile.Bitrate > 0 {
			kbps = fmt.Sprintf("%d", f.Profile.Bitrate)
		}
		verified := "-"
		if !f.VerifiedAt.IsZero() {
			verified = f.VerifiedAt.Format("15:04:05")
		}

		t.Logf("%-55s %-14s %-8s %-6s %-6s %s",
			f.ID, status, codec, rate, kbps, verified)
	}

	t.Log(separator)
	t.Logf("Total: %d  |  Available: %d  |  Unavailable: %d  |  Elapsed: %s",
		len(allFormats), available, unavailable, elapsed.Round(time.Millisecond))
}

func statusString(s tts.FormatStatus) string {
	switch s {
	case tts.FormatAvailable:
		return "AVAILABLE"
	case tts.FormatUnavailable:
		return "UNAVAILABLE"
	default:
		return "UNVERIFIED"
	}
}
