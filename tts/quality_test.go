package tts

import "testing"

func TestOutputOption_Fields(t *testing.T) {
	opt := OutputOption{
		FormatID:    "audio-48khz-192kbitrate-mono-mp3",
		Label:       "MP3 48kHz 192kbps",
		Description: "High quality",
		Profile: VoiceAudioProfile{
			Format:     AudioFormatMP3,
			SampleRate: 48000,
			Channels:   1,
			Bitrate:    192,
		},
		IsDefault: false,
		Verified:  "实际探测验证 (2026-02-21)",
	}

	if opt.FormatID == "" {
		t.Error("FormatID should not be empty")
	}
	if opt.Label == "" {
		t.Error("Label should not be empty")
	}
	if opt.Profile.Format != AudioFormatMP3 {
		t.Errorf("Profile.Format = %q; want %q", opt.Profile.Format, AudioFormatMP3)
	}
	if opt.Profile.SampleRate != 48000 {
		t.Errorf("Profile.SampleRate = %d; want 48000", opt.Profile.SampleRate)
	}
	if opt.Profile.Channels != 1 {
		t.Errorf("Profile.Channels = %d; want 1", opt.Profile.Channels)
	}
	if opt.Profile.Bitrate != 192 {
		t.Errorf("Profile.Bitrate = %d; want 192", opt.Profile.Bitrate)
	}
	if opt.Verified == "" {
		t.Error("Verified should not be empty")
	}
}

func TestOutputOption_DefaultFlag(t *testing.T) {
	opts := []OutputOption{
		{FormatID: "fmt-a", IsDefault: true},
		{FormatID: "fmt-b", IsDefault: false},
	}

	defaults := 0
	for _, o := range opts {
		if o.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("expected exactly 1 default; got %d", defaults)
	}
}
