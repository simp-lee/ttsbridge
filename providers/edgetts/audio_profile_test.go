package edgetts

import (
	"context"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestGetOutputFormat(t *testing.T) {
	p := New()
	got := p.getOutputFormat()
	if got != defaultOutputFormat {
		t.Errorf("getOutputFormat() = %q, want %q", got, defaultOutputFormat)
	}
}

func TestDynamicResolver_FormatRegistryHit(t *testing.T) {
	// New() registers a dynamic resolver; the FormatRegistry should contain
	// the default format as a constant with a known profile.
	_ = New()

	profile, ok := tts.LookupVoiceAudioProfile(providerName, "zh-CN-XiaoxiaoNeural")
	if !ok {
		t.Fatal("LookupVoiceAudioProfile returned false")
	}
	if profile.Format != tts.AudioFormatMP3 {
		t.Errorf("Format = %q, want %q", profile.Format, tts.AudioFormatMP3)
	}
	if profile.SampleRate != tts.SampleRate24kHz {
		t.Errorf("SampleRate = %d, want %d", profile.SampleRate, tts.SampleRate24kHz)
	}
	if profile.Bitrate != 48 {
		t.Errorf("Bitrate = %d, want 48", profile.Bitrate)
	}
	if profile.Channels != 1 {
		t.Errorf("Channels = %d, want 1", profile.Channels)
	}
}

func TestDynamicResolver_FallbackWithoutNew(t *testing.T) {
	// init() registers the static fallback; verify it works for any voiceID.
	profile, ok := tts.LookupVoiceAudioProfile("edgetts", "nonexistent-voice")
	if !ok {
		t.Fatal("static fallback resolver returned false")
	}
	if profile != defaultFallbackProfile {
		t.Errorf("profile = %+v, want %+v", profile, defaultFallbackProfile)
	}
}

func TestDynamicResolver_CustomFormatRegistry(t *testing.T) {
	// Provider with a custom FormatRegistry containing a high-quality MP3.
	// The global resolver should remain stable and not be overwritten.
	customReg := tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
	)
	customReg.RegisterConstant(OutputFormatMP3_48khz_192k, tts.VoiceAudioProfile{
		Format:     tts.AudioFormatMP3,
		SampleRate: tts.SampleRate48kHz,
		Channels:   1,
		Bitrate:    192,
	})
	// Also register the default format with a custom bitrate to verify registry is used.
	customReg.RegisterConstant(defaultOutputFormat, tts.VoiceAudioProfile{
		Format:     tts.AudioFormatMP3,
		SampleRate: tts.SampleRate24kHz,
		Channels:   1,
		Bitrate:    48,
	})

	_ = New().WithFormatRegistry(customReg)

	profile, ok := tts.LookupVoiceAudioProfile(providerName, "any-voice")
	if !ok {
		t.Fatal("resolver returned false")
	}
	// Should use stable package-level default profile from default registry.
	if profile.Bitrate != 48 {
		t.Errorf("Bitrate = %d, want 48", profile.Bitrate)
	}
}

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
	if f.Status != tts.FormatAvailable {
		t.Errorf("Status = %v; want FormatAvailable", f.Status)
	}
}

func TestDynamicResolver_SuggestedCodecFromCache(t *testing.T) {
	// Set up a provider with a VoiceCache pre-populated with voices.
	p := New()

	voices := []tts.Voice{
		{
			ID:       "en-US-TestNeural",
			Name:     "Test",
			Language: "en-US",
			Gender:   tts.GenderFemale,
			Provider: providerName,
			Extra: &VoiceExtra{
				ShortName:      "en-US-TestNeural",
				SuggestedCodec: "audio/ogg",
			},
		},
	}

	fetcher := func(ctx context.Context) ([]tts.Voice, error) {
		return voices, nil
	}
	p.voiceCache = tts.NewVoiceCache(fetcher)

	// Populate the cache by doing a Get
	_, err := p.voiceCache.Get(context.Background(), "")
	if err != nil {
		t.Fatalf("cache Get failed: %v", err)
	}

	// Global resolver is package-level and stable; instance cache state does not
	// alter global lookup behavior.
	profile, ok := tts.LookupVoiceAudioProfile(providerName, "en-US-TestNeural")
	if !ok {
		t.Fatal("resolver returned false")
	}
	// Should still get the default format profile (FormatRegistry hits first)
	if profile.Format != tts.AudioFormatMP3 {
		t.Errorf("Format = %q, want %q", profile.Format, tts.AudioFormatMP3)
	}
}

func TestStableResolver_MultipleProvidersDoNotOverwriteGlobal(t *testing.T) {
	_ = New()

	customReg := tts.NewFormatRegistry(tts.WithProfileParser(ParseOutputFormat))
	customReg.RegisterConstant(defaultOutputFormat, tts.VoiceAudioProfile{
		Format:     tts.AudioFormatMP3,
		SampleRate: tts.SampleRate24kHz,
		Channels:   1,
		Bitrate:    192,
	})

	_ = New().WithFormatRegistry(customReg)

	profile, ok := tts.LookupVoiceAudioProfile(providerName, "zh-CN-XiaoxiaoNeural")
	if !ok {
		t.Fatal("LookupVoiceAudioProfile returned false")
	}
	if profile.Bitrate != 48 {
		t.Fatalf("Bitrate = %d, want stable global 48 (no instance overwrite)", profile.Bitrate)
	}
}
