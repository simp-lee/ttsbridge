package edgetts

import (
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestOutputOptions_ReturnsNonEmpty(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	if len(options) == 0 {
		t.Fatal("OutputOptions() returned empty list")
	}
}

func TestOutputOptions_ExpectedFormats(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	expectedFormats := []string{
		OutputFormatMP3_24khz_48k,
		OutputFormatMP3_24khz_96k,
		OutputFormatMP3_48khz_192k,
		OutputFormatMP3_48khz_320k,
		OutputFormatPCM_24khz,
	}

	formatSet := make(map[string]bool)
	for _, opt := range options {
		formatSet[opt.FormatID] = true
	}

	for _, f := range expectedFormats {
		if !formatSet[f] {
			t.Errorf("OutputOptions() missing format %q", f)
		}
	}
}

func TestOutputOptions_FieldsPopulated(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	for _, opt := range options {
		if opt.FormatID == "" {
			t.Errorf("FormatID is empty for label %q", opt.Label)
		}
		if opt.Label == "" {
			t.Errorf("Label is empty for format %q", opt.FormatID)
		}
		if opt.Description == "" {
			t.Errorf("Description is empty for format %q", opt.FormatID)
		}
		if opt.Profile.Format == "" {
			t.Errorf("Profile.Format is empty for format %q", opt.FormatID)
		}
		if opt.Profile.SampleRate == 0 {
			t.Errorf("Profile.SampleRate is zero for format %q", opt.FormatID)
		}
		if opt.Profile.Channels == 0 {
			t.Errorf("Profile.Channels is zero for format %q", opt.FormatID)
		}
	}
}

func TestOutputOptions_HasExactlyOneDefault(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	defaults := 0
	for _, opt := range options {
		if opt.IsDefault {
			defaults++
		}
	}

	if defaults != 1 {
		t.Errorf("OutputOptions() has %d defaults; want exactly 1", defaults)
	}
}

func TestOutputOptions_FormatIDsAreVerified(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	for _, opt := range options {
		f, ok := provider.FormatRegistry().Get(opt.FormatID)
		if !ok {
			t.Errorf("FormatID %q not found in FormatRegistry", opt.FormatID)
			continue
		}
		if f.Status != tts.FormatUnverified {
			t.Errorf("FormatID %q status is %v; want FormatUnverified before probe", opt.FormatID, f.Status)
		}
	}
}

func TestOutputOptions_ProfilesMatchRegistry(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	for _, opt := range options {
		f, ok := provider.FormatRegistry().Get(opt.FormatID)
		if !ok {
			continue
		}
		if opt.Profile != f.Profile {
			t.Errorf("format %q: Profile mismatch\n  option:   %+v\n  registry: %+v",
				opt.FormatID, opt.Profile, f.Profile)
		}
	}
}

func TestOutputOptions_BitrateNonDecreasing(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	prevBitrate := 0
	for _, opt := range options {
		if opt.Profile.Lossless {
			continue // skip lossless — bitrate not comparable
		}
		if opt.Profile.Bitrate < prevBitrate {
			t.Errorf("format %q bitrate %d < previous %d; options should be in non-decreasing bitrate order",
				opt.FormatID, opt.Profile.Bitrate, prevBitrate)
		}
		prevBitrate = opt.Profile.Bitrate
	}
}

func TestOutputOptions_FormatIDsUsableAsOutputFormat(t *testing.T) {
	// Verify FormatIDs are the same constants users pass to SynthesizeOptions.OutputFormat
	provider := New()
	options := provider.OutputOptions()

	knownFormats := map[string]bool{
		OutputFormatMP3_24khz_48k:  true,
		OutputFormatMP3_24khz_96k:  true,
		OutputFormatMP3_48khz_192k: true,
		OutputFormatMP3_48khz_320k: true,
		OutputFormatPCM_24khz:      true,
	}

	for _, opt := range options {
		if !knownFormats[opt.FormatID] {
			t.Errorf("FormatID %q is not a known OutputFormat constant", opt.FormatID)
		}
	}
}

func TestOutputOptions_VerifiedFieldPopulated(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	for _, opt := range options {
		if opt.Verified == "" {
			t.Errorf("format %q: Verified field is empty; all OutputOptions must document their verification source", opt.FormatID)
		}
	}
}

// TestOutputOptions_SubsetOfRegistryCatalog 验证 OutputOptions 中的每个格式 ID
// 都存在于 FormatRegistry，并且在显式 probe 前保持未验证状态。
func TestOutputOptions_SubsetOfRegistryCatalog(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()
	registry := provider.FormatRegistry()

	for _, opt := range options {
		f, ok := registry.Get(opt.FormatID)
		if !ok {
			t.Errorf("OutputOptions format %q not found in FormatRegistry — registry may have changed", opt.FormatID)
			continue
		}
		if f.Status != tts.FormatUnverified {
			t.Errorf("OutputOptions format %q has registry status %v; want FormatUnverified before probe",
				opt.FormatID, f.Status)
		}
	}
}

func TestOutputOptions_RawPCMUsesPCMMetadata(t *testing.T) {
	provider := New()
	options := provider.OutputOptions()

	for _, opt := range options {
		if opt.FormatID != OutputFormatPCM_24khz {
			continue
		}
		if opt.Profile.Format != tts.AudioFormatPCM {
			t.Fatalf("Profile.Format = %q, want %q", opt.Profile.Format, tts.AudioFormatPCM)
		}
		return
	}

	t.Fatal("raw PCM output option not found")
}
