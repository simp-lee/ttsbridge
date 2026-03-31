package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
)

func plainTextRequest(text, voice string) tts.SynthesisRequest {
	return tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      text,
		VoiceID:   voice,
	}
}

func TestProviderCapabilities_ExposeUnifiedContractConstraints(t *testing.T) {
	provider := New()
	caps := provider.Capabilities()

	if caps.RawSSML {
		t.Fatal("Capabilities().RawSSML = true, want false")
	}
	if caps.ProsodyParams {
		t.Fatal("Capabilities().ProsodyParams = true, want false")
	}
	if !caps.PlainTextOnly {
		t.Fatal("Capabilities().PlainTextOnly = false, want true")
	}
	if caps.BoundaryEvents {
		t.Fatal("Capabilities().BoundaryEvents = true, want false")
	}
	if caps.Streaming {
		t.Fatal("Capabilities().Streaming = true, want false")
	}
	if caps.ResolvedOutputFormat("") != tts.AudioFormatWAV {
		t.Fatalf("Capabilities().ResolvedOutputFormat(empty) = %q, want %q", caps.ResolvedOutputFormat(""), tts.AudioFormatWAV)
	}
	if !caps.SupportsFormat(tts.AudioFormatWAV) {
		t.Fatal("Capabilities().SupportsFormat(wav) = false, want true")
	}
	for _, format := range []string{tts.AudioFormatMP3, tts.AudioFormatPCM} {
		if caps.SupportsFormat(format) {
			t.Fatalf("Capabilities().SupportsFormat(%q) = true, want false", format)
		}
	}
}

func TestProviderCapabilities_PreQueryReturnsStableMatrix(t *testing.T) {
	provider := New()
	first := provider.Capabilities()
	if len(first.SupportedFormats) == 0 {
		t.Fatal("Capabilities().SupportedFormats is empty, want full matrix without synthesis")
	}

	first.SupportedFormats[0] = tts.AudioFormatMP3

	second := provider.Capabilities()
	if second.SupportsFormat(tts.AudioFormatMP3) {
		t.Fatal("Capabilities() reused caller-mutated SupportedFormats slice")
	}
	if !second.SupportsFormat(tts.AudioFormatWAV) {
		t.Fatal("Capabilities().SupportsFormat(wav) = false after mutation, want true")
	}
}

func TestSynthesizeRejectsProsodyInputMode(t *testing.T) {
	provider := New()
	_, err := provider.Synthesize(context.Background(), tts.SynthesisRequest{
		InputMode: tts.InputModePlainTextWithProsody,
		Text:      "hello",
		VoiceID:   "BV700_streaming",
		Prosody:   tts.ProsodyParams{Rate: 1.1},
	})
	if err == nil {
		t.Fatal("expected prosody request to fail for Volcengine")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
		t.Fatalf("error code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
	}
}

func TestSynthesizeRejectsRawSSMLInputMode(t *testing.T) {
	provider := New()
	_, err := provider.Synthesize(context.Background(), tts.SynthesisRequest{
		InputMode: tts.InputModeRawSSML,
		SSML:      "<speak>hello</speak>",
		VoiceID:   "BV700_streaming",
	})
	if err == nil {
		t.Fatal("expected raw SSML request to fail for Volcengine")
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
		t.Fatalf("error code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
	}
}

func TestSynthesizeStreamRejectsUnsupportedStreaming(t *testing.T) {
	provider := New()
	_, err := provider.SynthesizeStream(context.Background(), plainTextRequest("hello", "BV700_streaming"))
	if err == nil {
		t.Fatal("expected stream request to fail for Volcengine")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeUnsupportedCapability {
		t.Fatalf("error code = %q, want %q", ttsErr.Code, tts.ErrCodeUnsupportedCapability)
	}
	if ttsErr.Message != "streaming synthesis is not supported by provider" {
		t.Fatalf("error message = %q, want %q", ttsErr.Message, "streaming synthesis is not supported by provider")
	}
}

func TestIsAvailableRejectsInvalidWAVResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString([]byte("short"))
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxAttempts(1)
	if p.IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = true; want false for invalid WAV response")
	}
}

func TestIsAvailableRejectsMalformedPseudoWAV(t *testing.T) {
	tests := []struct {
		name  string
		chunk []byte
	}{
		{name: "header only", chunk: makeHeaderOnlyChunkForTest()},
		{name: "mismatched sizes", chunk: makeChunkWithMismatchedSizesForTest([]byte{1, 2, 3, 4}, 4, 0)},
		{name: "inconsistent byte rate", chunk: makeChunkWithInconsistentByteRateForTest([]byte{1, 2, 3, 4})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := translateResponseStub{}
				resp.BaseResp.StatusCode = 0
				resp.Audio.Data = base64.StdEncoding.EncodeToString(tt.chunk)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			p := New().WithBaseURL(server.URL).WithMaxAttempts(1)
			if p.IsAvailable(context.Background()) {
				t.Fatalf("IsAvailable() = true; want false for %s", tt.name)
			}
		})
	}
}

func TestSynthesizeRejectsMalformedPseudoWAV(t *testing.T) {
	tests := []struct {
		name  string
		chunk []byte
	}{
		{name: "header only", chunk: makeHeaderOnlyChunkForTest()},
		{name: "mismatched data size", chunk: makeChunkWithMismatchedSizesForTest([]byte{1, 2, 3, 4}, 0, 2)},
		{name: "inconsistent byte rate", chunk: makeChunkWithInconsistentByteRateForTest([]byte{1, 2, 3, 4})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := translateResponseStub{}
				resp.BaseResp.StatusCode = 0
				resp.Audio.Data = base64.StdEncoding.EncodeToString(tt.chunk)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			p := New().WithBaseURL(server.URL).WithMaxAttempts(1)
			_, err := p.Synthesize(context.Background(), plainTextRequest("hello", "BV700_streaming"))
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), "no valid wav chunk header found") {
				t.Fatalf("unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

func TestSynthesizePreservesProviderUnavailableAfterRetryExhaustion(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "upstream overloaded", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxAttempts(2)
	_, err := p.Synthesize(context.Background(), plainTextRequest("hello", "BV700_streaming"))
	if err == nil {
		t.Fatal("expected retry exhaustion error, got nil")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T: %v", err, err)
	}
	if ttsErr.Code != tts.ErrCodeProviderUnavail {
		t.Fatalf("Code = %s; want %s", ttsErr.Code, tts.ErrCodeProviderUnavail)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want 2", got)
	}
	if !strings.Contains(err.Error(), "retryable status 503") {
		t.Fatalf("error = %v; want final 503 context", err)
	}
}

func TestSynthesizeFailsFastOnInvalidProxyURL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(makeWAVChunkForTest([]byte{1, 2, 3, 4}))
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithProxy("://bad-proxy")
	_, err := p.Synthesize(context.Background(), plainTextRequest("hello", "BV700_streaming"))
	if err == nil {
		t.Fatal("expected invalid proxy error, got nil")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T: %v", err, err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("Code = %s; want %s", ttsErr.Code, tts.ErrCodeInvalidInput)
	}
	if !strings.Contains(err.Error(), "invalid proxy URL") {
		t.Fatalf("error = %v; want invalid proxy URL context", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("calls = %d; want 0 because request should fail before network", got)
	}
}

func TestFormatRegistry_IsolatedPerProviderInstance(t *testing.T) {
	providerA := New()
	providerB := New()

	const customFormatID = "custom-test-format"
	providerA.FormatRegistry().RegisterConstant(customFormatID, tts.VoiceAudioProfile{Format: tts.AudioFormatWAV, SampleRate: 16000, Channels: 1})

	if _, ok := providerA.FormatRegistry().Get(customFormatID); !ok {
		t.Fatalf("providerA registry missing %q", customFormatID)
	}
	if _, ok := providerB.FormatRegistry().Get(customFormatID); ok {
		t.Fatalf("providerB registry unexpectedly contains %q", customFormatID)
	}

	for _, format := range providerB.SupportedFormats() {
		if format.ID == customFormatID {
			t.Fatalf("providerB SupportedFormats() unexpectedly contains %q", customFormatID)
		}
	}
}

func TestSynthesizeFailsFastOnInvalidBaseURL(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(makeWAVChunkForTest([]byte{1, 2, 3, 4}))
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL("://bad-base").WithMaxAttempts(3)
	_, err := p.Synthesize(context.Background(), plainTextRequest("hello", "BV700_streaming"))
	if err == nil {
		t.Fatal("expected invalid base URL error, got nil")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T: %v", err, err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("Code = %s; want %s", ttsErr.Code, tts.ErrCodeInvalidInput)
	}
	if !strings.Contains(err.Error(), "invalid base URL") {
		t.Fatalf("error = %v; want invalid base URL context", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("calls = %d; want 0 because request should fail before network", got)
	}
}
