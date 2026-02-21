package volcengine

import (
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestOutputOptions_ReturnsSingleOption(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	if len(options) != 1 {
		t.Fatalf("OutputOptions() returned %d options; want 1 (fixed WAV format)", len(options))
	}
}

func TestOutputOptions_IsLossless(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	opt := options[0]
	if !opt.Profile.Lossless {
		t.Error("Profile.Lossless = false; want true")
	}
	if opt.Profile.Format != tts.AudioFormatWAV {
		t.Errorf("Profile.Format = %q; want %q", opt.Profile.Format, tts.AudioFormatWAV)
	}
}

func TestOutputOptions_IsDefault(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	if !options[0].IsDefault {
		t.Error("the single output option should be marked as default")
	}
}

func TestOutputOptions_FieldsPopulated(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	opt := options[0]
	if opt.FormatID == "" {
		t.Error("FormatID is empty")
	}
	if opt.Label == "" {
		t.Error("Label is empty")
	}
	if opt.Description == "" {
		t.Error("Description is empty")
	}
	if opt.Profile.SampleRate != 24000 {
		t.Errorf("SampleRate = %d; want 24000", opt.Profile.SampleRate)
	}
	if opt.Profile.Channels != 1 {
		t.Errorf("Channels = %d; want 1", opt.Profile.Channels)
	}
}

func TestOutputOptions_FormatIDMatchesConstant(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	if options[0].FormatID != OutputFormatWAV_24khz {
		t.Errorf("FormatID = %q; want %q", options[0].FormatID, OutputFormatWAV_24khz)
	}
}

func TestOutputOptions_VerifiedFieldPopulated(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	if options[0].Verified == "" {
		t.Error("Verified field is empty; should document the verification source")
	}
}
