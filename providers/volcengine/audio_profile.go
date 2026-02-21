package volcengine

import "github.com/simp-lee/ttsbridge/tts"

// OutputFormatWAV_24khz is the only output format supported by Volcengine
// free TTS API. The API always returns WAV 24kHz 16-bit mono audio.
const OutputFormatWAV_24khz = "wav-24khz-16bit-mono"

// defaultFormatRegistry is the package-level FormatRegistry for Volcengine.
// Because the API returns a fixed format, no FormatProber is needed.
var defaultFormatRegistry *tts.FormatRegistry

// wavProfile is the fixed audio profile for all Volcengine voices.
var wavProfile = tts.VoiceAudioProfile{
	Format:     tts.AudioFormatWAV,
	SampleRate: 24000,
	Channels:   1,
	Lossless:   true,
}

func init() {
	defaultFormatRegistry = tts.NewFormatRegistry()
	defaultFormatRegistry.RegisterConstant(OutputFormatWAV_24khz, wavProfile)

	// 所有21种免费语音的音质都是固定的：WAV, 24000Hz, 单声道, 无损
	resolver := func(string) (tts.VoiceAudioProfile, bool) {
		return wavProfile, true
	}

	tts.RegisterVoiceProfileResolver(providerName, resolver)
}
