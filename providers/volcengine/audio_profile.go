package volcengine

import "github.com/simp-lee/ttsbridge/tts"

func init() {
	// 所有21种免费语音的音质都是固定的：WAV, 24000Hz, 单声道, 无损
	resolver := func(string) (tts.VoiceAudioProfile, bool) {
		return tts.VoiceAudioProfile{
			Format:     tts.AudioFormatWAV,
			SampleRate: 24000,
			Channels:   1,
			Lossless:   true,
		}, true
	}

	tts.RegisterVoiceProfileResolver(providerName, resolver)
}
