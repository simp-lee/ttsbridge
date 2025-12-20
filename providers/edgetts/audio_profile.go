package edgetts

import "github.com/simp-lee/ttsbridge/tts"

func init() {
	profile := tts.VoiceAudioProfile{
		Format:     tts.AudioFormatMP3,
		SampleRate: tts.SampleRate24kHz,
		Channels:   1,
		Bitrate:    48,
		Lossless:   false,
	}

	resolver := func(string) (tts.VoiceAudioProfile, bool) {
		return profile, true
	}

	tts.RegisterVoiceProfileResolver("edgetts", resolver)
	tts.RegisterVoiceProfileResolver(providerName, resolver)
}
