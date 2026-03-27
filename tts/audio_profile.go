package tts

// VoiceAudioProfile describes the intrinsic audio properties of a TTS voice
// returned by a provider. The profile reflects the exact output of the TTS
// service before any post-processing or mixing is applied.
type VoiceAudioProfile struct {
	Format     string
	SampleRate int
	Channels   int
	Bitrate    int
	Lossless   bool
}
