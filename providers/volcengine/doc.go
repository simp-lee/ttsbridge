// Package volcengine implements the Volcengine TTS provider behind the shared
// tts.Provider contract.
//
// The built-in provider is plain-text only and returns WAV audio. It does not
// support raw SSML, prosody parameters, boundary events, or streaming.
//
// Callers should inspect Capabilities and Voice metadata before requesting
// provider-specific behavior such as multilingual catalog filtering.
package volcengine
