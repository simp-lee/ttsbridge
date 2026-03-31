// Package edgetts implements the Edge TTS provider behind the shared
// tts.Provider contract.
//
// The built-in provider accepts plain text and plain text with prosody,
// supports boundary events and streaming, and exposes MP3 as its caller-facing
// output format.
//
// Raw SSML is rejected by the free Edge endpoint, so callers should inspect
// Capabilities before requesting unsupported input modes or formats.
package edgetts
