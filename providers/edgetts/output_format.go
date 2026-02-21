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

func init() {
	defaultFormatRegistry = tts.NewFormatRegistry(
		tts.WithProfileParser(ParseOutputFormat),
	)

	// Register compile-time constants — these 9 formats are known-good and
	// marked FormatAvailable at init time. Their profiles are hand-verified to
	// match the output of ParseOutputFormat. No runtime probing required.
	for _, c := range []struct {
		id      string
		profile tts.VoiceAudioProfile
	}{
		{OutputFormatMP3_24khz_48k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48}},
		{OutputFormatMP3_24khz_96k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 96}},
		{OutputFormatMP3_24khz_160k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 160}},
		{OutputFormatMP3_48khz_192k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 192}},
		{OutputFormatMP3_48khz_320k, tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 320}},
		{OutputFormatOpus_24khz, tts.VoiceAudioProfile{Format: "opus", SampleRate: 24000, Channels: 1}},
		{OutputFormatPCM_24khz, tts.VoiceAudioProfile{Format: tts.AudioFormatWAV, SampleRate: 24000, Channels: 1, Lossless: true}},
		{OutputFormatWebM_24khz, tts.VoiceAudioProfile{Format: "webm", SampleRate: 24000, Channels: 1}},
		{OutputFormatOgg_24khz, tts.VoiceAudioProfile{Format: "ogg", SampleRate: 24000, Channels: 1}},
	} {
		defaultFormatRegistry.RegisterConstant(c.id, c.profile)
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
	profile := tts.VoiceAudioProfile{}

	// Parse sample rate: match "24khz", "48khz" (NNkhz) and "22050hz", "44100hz" (NNNNNhz)
	if m := sampleRateRe.FindStringSubmatch(format); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			if strings.Contains(m[0], "khz") {
				profile.SampleRate = n * 1000
			} else {
				// Direct Hz value (e.g. "22050hz", "44100hz", "16000hz")
				profile.SampleRate = n
			}
		}
	}
	if profile.SampleRate == 0 {
		return profile, false
	}

	// Parse channels
	if strings.Contains(format, "mono") {
		profile.Channels = 1
	} else if strings.Contains(format, "stereo") {
		profile.Channels = 2
	} else {
		profile.Channels = 1 // default mono
	}

	// Parse bitrate: look for pattern like "48kbitrate" or "192kbitrate"
	if m := bitrateRe.FindStringSubmatch(format); len(m) > 1 {
		if br, err := strconv.Atoi(m[1]); err == nil {
			profile.Bitrate = br
		}
	}

	// Parse format (check container/codec prefixes first, then codec suffixes)
	lowerFormat := strings.ToLower(format)
	switch {
	case strings.HasPrefix(lowerFormat, "riff-"):
		switch {
		case strings.HasSuffix(lowerFormat, "-alaw"):
			profile.Format = "alaw"
		case strings.HasSuffix(lowerFormat, "-mulaw"):
			profile.Format = "mulaw"
		default:
			profile.Format = tts.AudioFormatWAV
			profile.Lossless = true
		}
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
	case strings.HasPrefix(lowerFormat, "raw-") || strings.HasSuffix(lowerFormat, "-pcm"):
		profile.Format = tts.AudioFormatWAV
		profile.Lossless = true
	case strings.HasSuffix(lowerFormat, "-mp3"):
		profile.Format = tts.AudioFormatMP3
	case strings.HasSuffix(lowerFormat, "-opus"):
		profile.Format = "opus"
	default:
		profile.Format = tts.AudioFormatMP3
	}

	return profile, true
}

// parseOutputFormat is an unexported alias kept for backward compatibility
// within the package.
var parseOutputFormat = ParseOutputFormat
