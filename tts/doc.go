// Package tts defines the provider-neutral text-to-speech contract shared by
// all built-in providers.
//
// Callers discover voices through Voice and VoiceFilter, submit synthesis work
// through SynthesisRequest, and receive normalized audio metadata in
// SynthesisResult.
//
// ProviderCapabilities describes which optional features a provider actually
// supports. Callers should inspect that capability set before sending raw SSML,
// prosody parameters, boundary-event requests, streaming requests, or
// non-default output formats.
package tts
