package edgetts

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/simp-lee/ttsbridge/tts"
)

// Common Edge TTS output formats
const (
	OutputFormatMP3_24khz_48k  = "audio-24khz-48kbitrate-mono-mp3"  // Default, 48kbps
	OutputFormatMP3_24khz_96k  = "audio-24khz-96kbitrate-mono-mp3"  // 96kbps
	OutputFormatMP3_24khz_160k = "audio-24khz-160kbitrate-mono-mp3" // 160kbps
	OutputFormatMP3_48khz_192k = "audio-48khz-192kbitrate-mono-mp3" // 192kbps
	OutputFormatMP3_48khz_320k = "audio-48khz-320kbitrate-mono-mp3" // 320kbps (highest quality MP3)
	OutputFormatOpus_24khz     = "audio-24khz-16bit-mono-opus"      // Opus
	OutputFormatPCM_24khz      = "raw-24khz-16bit-mono-pcm"         // Raw PCM
	OutputFormatWebM_24khz     = "webm-24khz-16bit-mono-opus"       // WebM container
	OutputFormatOgg_24khz      = "ogg-24khz-16bit-mono-opus"        // Ogg container
)

// defaultFormatRegistry is the package-level FormatRegistry shared by all
// Provider instances. It is initialised in init() with all known Edge TTS
// output formats.
var defaultFormatRegistry *tts.FormatRegistry

var edgeTTSKnownFormats = []struct {
	id      string
	profile tts.VoiceAudioProfile
}{
	{OutputFormatMP3_24khz_48k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48}},
	{OutputFormatMP3_24khz_96k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 96}},
	{OutputFormatMP3_24khz_160k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 160}},
	{OutputFormatMP3_48khz_192k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 192}},
	{OutputFormatMP3_48khz_320k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 320}},
	{OutputFormatOpus_24khz, tts.VoiceAudioProfile{Format: "opus", SampleRate: tts.SampleRate48kHz, Channels: 1}},
	{OutputFormatPCM_24khz, tts.VoiceAudioProfile{Format: tts.AudioFormatPCM, SampleRate: 24000, Channels: 1, Lossless: true}},
	{OutputFormatWebM_24khz, tts.VoiceAudioProfile{Format: "webm", SampleRate: tts.SampleRate48kHz, Channels: 1}},
	{OutputFormatOgg_24khz, tts.VoiceAudioProfile{Format: "ogg", SampleRate: tts.SampleRate48kHz, Channels: 1}},
}

func init() {
	defaultFormatRegistry = tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
	)

	// Register the documented catalog as FormatUnverified.
	// Edge runtime availability must be established by an explicit probe in the
	// current environment before these IDs are reported by SupportedFormats().
	for _, c := range edgeTTSKnownFormats {
		defaultFormatRegistry.Register(tts.OutputFormat{
			ID:      c.id,
			Profile: c.profile,
			Status:  tts.FormatUnverified,
		})
	}

	// Register additional Azure-documented formats as FormatUnverified.
	// Profile is pre-populated via ParseOutputFormat so callers always see
	// valid audio characteristics even before probing.
	//
	// 探测结果 (2026-02-21): 以下 28 个格式全部探测为 UNAVAILABLE。
	// Edge TTS 实际仅支持上面 9 个 RegisterConstant 的格式。
	// 如需重新验证，运行: go test -v -tags probe -run TestEdgeTTSFormatProbe ./providers/edgetts/
	unverifiedIDs := []string{
		// MP3 formats
		"audio-16khz-32kbitrate-mono-mp3",
		"audio-16khz-64kbitrate-mono-mp3",
		"audio-16khz-128kbitrate-mono-mp3",
		"audio-48khz-96kbitrate-mono-mp3",

		// WebM formats
		"webm-16khz-16bit-mono-opus",

		// Ogg formats
		"ogg-16khz-16bit-mono-opus",
		"ogg-48khz-16bit-mono-opus",

		// Opus raw formats
		"audio-16khz-16bit-32kbps-mono-opus",
		"audio-24khz-16bit-24kbps-mono-opus",
		"audio-24khz-16bit-48kbps-mono-opus",

		// Raw PCM formats
		"raw-8khz-8bit-mono-alaw",
		"raw-8khz-8bit-mono-mulaw",
		"raw-8khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-pcm",
		"raw-16khz-16bit-mono-truesilk",
		"raw-22050hz-16bit-mono-pcm",
		"raw-24khz-16bit-mono-truesilk",
		"raw-44100hz-16bit-mono-pcm",
		"raw-48khz-16bit-mono-pcm",

		// RIFF/WAV formats
		"riff-8khz-8bit-mono-alaw",
		"riff-8khz-8bit-mono-mulaw",
		"riff-8khz-16bit-mono-pcm",
		"riff-16khz-16bit-mono-pcm",
		"riff-22050hz-16bit-mono-pcm",
		"riff-24khz-16bit-mono-pcm",
		"riff-44100hz-16bit-mono-pcm",
		"riff-48khz-16bit-mono-pcm",

		// Other formats
		"amr-wb-16000hz",
		"g722-16khz-64kbps",
	}
	for _, id := range unverifiedIDs {
		f := tts.OutputFormat{ID: id, Status: tts.FormatUnverified}
		if profile, ok := ParseOutputFormat(id); ok {
			f.Profile = profile
		}
		defaultFormatRegistry.Register(f)
	}
}

var (
	// sampleRateRe matches "24khz", "48khz", "22050hz", "44100hz", etc.
	sampleRateRe = regexp.MustCompile(`(\d+)k?hz`)
	bitrateRe    = regexp.MustCompile(`(\d+)kbitrate`)
)

func parseSampleRate(format string) int {
	m := sampleRateRe.FindStringSubmatch(format)
	if len(m) <= 1 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	if strings.Contains(m[0], "khz") {
		return n * 1000
	}
	return n
}

func parseChannels(format string) int {
	if strings.Contains(format, "stereo") {
		return 2
	}
	return 1
}

func parseBitrate(format string) int {
	m := bitrateRe.FindStringSubmatch(format)
	if len(m) <= 1 {
		return 0
	}
	bitrate, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return bitrate
}

func normalizeProfileSampleRate(profile *tts.VoiceAudioProfile, lowerFormat string) {
	if strings.HasSuffix(lowerFormat, "-opus") {
		// Opus streams are decoded on a fixed 48kHz clock even when the format ID
		// advertises a lower nominal bandwidth such as 16kHz or 24kHz.
		profile.SampleRate = tts.SampleRate48kHz
	}
}

func resolveProfileFormat(profile *tts.VoiceAudioProfile, lowerFormat string) {
	switch {
	case strings.HasPrefix(lowerFormat, "riff-"):
		resolveRiffFormat(profile, lowerFormat)
	case strings.HasPrefix(lowerFormat, "ogg-"):
		profile.Format = "ogg"
	case strings.HasPrefix(lowerFormat, "webm-"):
		profile.Format = "webm"
	case strings.HasPrefix(lowerFormat, "amr-wb-"):
		profile.Format = "amr-wb"
	case strings.HasPrefix(lowerFormat, "g722-"):
		profile.Format = "g722"
	case strings.HasSuffix(lowerFormat, "-truesilk"):
		profile.Format = "silk"
	case strings.HasSuffix(lowerFormat, "-alaw"):
		profile.Format = "alaw"
	case strings.HasSuffix(lowerFormat, "-mulaw"):
		profile.Format = "mulaw"
	case strings.HasPrefix(lowerFormat, "raw-") && strings.HasSuffix(lowerFormat, "-pcm"):
		profile.Format = tts.AudioFormatPCM
		profile.Lossless = true
	case strings.HasSuffix(lowerFormat, "-pcm"):
		profile.Format = tts.AudioFormatWAV
		profile.Lossless = true
	case strings.HasSuffix(lowerFormat, "-mp3"):
		profile.Format = tts.AudioFormatMP3
	case strings.HasSuffix(lowerFormat, "-opus"):
		profile.Format = "opus"
	default:
		profile.Format = tts.AudioFormatMP3
	}
}

func resolveRiffFormat(profile *tts.VoiceAudioProfile, lowerFormat string) {
	switch {
	case strings.HasSuffix(lowerFormat, "-alaw"):
		profile.Format = "alaw"
	case strings.HasSuffix(lowerFormat, "-mulaw"):
		profile.Format = "mulaw"
	default:
		profile.Format = tts.AudioFormatWAV
		profile.Lossless = true
	}
}

// ParseOutputFormat parses an Edge TTS output format string into a VoiceAudioProfile.
// Format examples:
//
//	"audio-24khz-48kbitrate-mono-mp3"
//	"audio-48khz-192kbitrate-mono-mp3"
//	"audio-24khz-16bit-mono-opus"
//	"audio-48khz-96kbitrate-mono-mp3"
//	"webm-24khz-16bit-mono-opus"
//	"ogg-24khz-16bit-mono-opus"
//	"raw-24khz-16bit-mono-pcm"
//	"riff-24khz-16bit-mono-pcm"
//	"raw-16khz-16bit-mono-truesilk"
//	"amr-wb-16000hz"
//	"g722-16khz-64kbps"
func ParseOutputFormat(format string) (tts.VoiceAudioProfile, bool) {
	profile := tts.VoiceAudioProfile{
		SampleRate: parseSampleRate(format),
		Channels:   parseChannels(format),
		Bitrate:    parseBitrate(format),
	}
	if profile.SampleRate == 0 {
		return profile, false
	}
	lowerFormat := strings.ToLower(format)
	resolveProfileFormat(&profile, lowerFormat)
	normalizeProfileSampleRate(&profile, lowerFormat)

	return profile, true
}
