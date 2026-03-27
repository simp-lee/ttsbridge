package edgetts

import (
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

// TestDynamicResolver_192kbpsProfile verifies that when the default registry is
// used, querying the 192kbps constant through the registry yields a 192kbps
// profile (not the static 48kbps fallback).
func TestDynamicResolver_192kbpsProfile(t *testing.T) {
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
