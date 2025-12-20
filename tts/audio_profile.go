package tts

import (
	"strings"
	"sync"
)

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

// VoiceProfileResolver resolves the audio profile for a given provider voice.
// Implementations should return the profile that most closely matches the
// provider's actual audio output. Returning false indicates that the profile is
// unknown.
type VoiceProfileResolver func(voiceID string) (VoiceAudioProfile, bool)

var (
	voiceProfileRegistryMu sync.RWMutex
	voiceProfileRegistry   = make(map[string]VoiceProfileResolver)
)

// RegisterVoiceProfileResolver registers a resolver for a provider. The most
// recent registration wins. Calls with empty provider names or nil resolvers are
// ignored.
func RegisterVoiceProfileResolver(provider string, resolver VoiceProfileResolver) {
	if provider == "" || resolver == nil {
		return
	}

	key := strings.ToLower(provider)

	voiceProfileRegistryMu.Lock()
	voiceProfileRegistry[key] = resolver
	voiceProfileRegistryMu.Unlock()
}

// LookupVoiceAudioProfile retrieves the registered audio profile for the given
// provider voice. The lookup is case-insensitive on provider names. If no
// resolver is registered or the resolver reports the voice as unknown, the
// second return value is false.
func LookupVoiceAudioProfile(provider, voiceID string) (VoiceAudioProfile, bool) {
	if provider == "" {
		return VoiceAudioProfile{}, false
	}

	key := strings.ToLower(provider)

	voiceProfileRegistryMu.RLock()
	resolver := voiceProfileRegistry[key]
	voiceProfileRegistryMu.RUnlock()

	if resolver == nil {
		return VoiceAudioProfile{}, false
	}

	return resolver(voiceID)
}
