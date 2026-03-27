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
			_, err := p.Synthesize(context.Background(), &SynthesizeOptions{Text: "hello", Voice: "BV700_streaming"})
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
	_, err := p.Synthesize(context.Background(), &SynthesizeOptions{Text: "hello", Voice: "BV700_streaming"})
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
	_, err := p.Synthesize(context.Background(), &SynthesizeOptions{Text: "hello", Voice: "BV700_streaming"})
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
	_, err := p.Synthesize(context.Background(), &SynthesizeOptions{Text: "hello", Voice: "BV700_streaming"})
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
