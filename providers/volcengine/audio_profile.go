package volcengine

import "github.com/simp-lee/ttsbridge/tts"

// OutputFormatWAV_24khz is the only output format supported by Volcengine
// free TTS API. The API always returns WAV 24kHz 16-bit mono audio.
const OutputFormatWAV_24khz = "wav-24khz-16bit-mono"

// defaultFormatRegistryTemplate is the package-level template used to seed a
// clean per-provider registry. Because the API returns a fixed format, no
// FormatProber is needed.
var defaultFormatRegistryTemplate *tts.FormatRegistry

// wavProfile is the fixed audio profile for all Volcengine voices.
var wavProfile = tts.VoiceAudioProfile{
	Format:     tts.AudioFormatWAV,
	SampleRate: 24000,
	Channels:   1,
	Lossless:   true,
}

func init() {
	defaultFormatRegistryTemplate = tts.NewFormatRegistry()
	defaultFormatRegistryTemplate.RegisterConstant(OutputFormatWAV_24khz, wavProfile)
}

func newDefaultFormatRegistry() *tts.FormatRegistry {
	return defaultFormatRegistryTemplate.CloneDeclaredClean()
}
