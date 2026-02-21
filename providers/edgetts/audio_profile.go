package edgetts

import (
	"sync"

	"github.com/simp-lee/ttsbridge/tts"
)

// defaultFallbackProfile is the static fallback profile matching the default
// Edge TTS output format (audio-24khz-48kbitrate-mono-mp3).
var defaultFallbackProfile = tts.VoiceAudioProfile{
	Format:     tts.AudioFormatMP3,
	SampleRate: tts.SampleRate24kHz,
	Channels:   1,
	Bitrate:    48,
}

var stableResolverOnce sync.Once

func init() {
	ensureStableResolverRegistered()
}

// getOutputFormat returns the output format string the provider uses by default.
// Per-call overrides (SynthesizeOptions.OutputFormat) are handled at the call
// site and do not affect this method.
func (p *Provider) getOutputFormat() string {
	return defaultOutputFormat
}

// ensureStableResolverRegistered installs a package-level, non-instance
// resolver exactly once to avoid cross-instance overwrite races. Instance-level
// output format differences must be passed explicitly via profile overrides.
func ensureStableResolverRegistered() {
	stableResolverOnce.Do(func() {
		resolver := func(string) (tts.VoiceAudioProfile, bool) {
			if f, ok := defaultFormatRegistry.Get(defaultOutputFormat); ok {
				if f.Profile != (tts.VoiceAudioProfile{}) {
					return f.Profile, true
				}
			}

			if profile, ok := ParseOutputFormat(defaultOutputFormat); ok {
				return profile, true
			}

			return defaultFallbackProfile, true
		}

		tts.RegisterVoiceProfileResolver("edgetts", resolver)
		tts.RegisterVoiceProfileResolver(providerName, resolver)
	})
}
