package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type translateResponseStub struct {
	BaseResp struct {
		StatusCode int `json:"status_code"`
	} `json:"base_resp"`
	Audio struct {
		Data string `json:"data"`
	} `json:"audio"`
}

func makeWAVChunkForTest(pcmData []byte) []byte {
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	header[16] = 16
	header[20] = 1
	header[22] = 1
	header[24] = 0x80
	header[25] = 0x3E
	header[34] = 16
	copy(header[36:40], []byte("data"))

	dataSize := len(pcmData)
	fileSize := dataSize + 36
	header[4] = byte(fileSize & 0xff)
	header[5] = byte((fileSize >> 8) & 0xff)
	header[6] = byte((fileSize >> 16) & 0xff)
	header[7] = byte((fileSize >> 24) & 0xff)
	header[40] = byte(dataSize & 0xff)
	header[41] = byte((dataSize >> 8) & 0xff)
	header[42] = byte((dataSize >> 16) & 0xff)
	header[43] = byte((dataSize >> 24) & 0xff)

	return append(header, pcmData...)
}

func makeInvalidHeaderChunkForTest() []byte {
	chunk := make([]byte, 44)
	copy(chunk[0:4], []byte("NOPE"))
	copy(chunk[8:12], []byte("WAVE"))
	copy(chunk[12:16], []byte("fmt "))
	copy(chunk[36:40], []byte("data"))
	return chunk
}

func TestSynthesizeMultiChunkAllInvalidChunksReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString([]byte("short"))
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abcdef",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeMultiChunkFirstInvalidThenValidReturnsErrorWithoutOuterRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		if current == 1 {
			resp.Audio.Data = base64.StdEncoding.EncodeToString([]byte("short"))
		} else {
			chunk := makeWAVChunkForTest([]byte{1, 2, 3, 4, 5, 6, 7, 8})
			resp.Audio.Data = base64.StdEncoding.EncodeToString(chunk)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1).WithMaxAttempts(3)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abcdef",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 request without outer retry, got %d", got)
	}
}

func TestSynthesizeMultiChunkAllInvalidHeadersReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(makeInvalidHeaderChunkForTest())
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abcdef",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeMultiChunkInvalidHeaderThenValidReturnsErrorWithoutOuterRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		if current == 1 {
			resp.Audio.Data = base64.StdEncoding.EncodeToString(makeInvalidHeaderChunkForTest())
		} else {
			chunk := makeWAVChunkForTest([]byte{1, 2, 3, 4, 5, 6, 7, 8})
			resp.Audio.Data = base64.StdEncoding.EncodeToString(chunk)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1).WithMaxAttempts(3)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abcdef",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 request without outer retry, got %d", got)
	}
}

func TestSynthesizeSingleChunkInvalidHeaderReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(makeInvalidHeaderChunkForTest())
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abc",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeSingleChunkInvalidHeaderThenValidReturnsErrorWithoutOuterRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		if current == 1 {
			resp.Audio.Data = base64.StdEncoding.EncodeToString(makeInvalidHeaderChunkForTest())
		} else {
			resp.Audio.Data = base64.StdEncoding.EncodeToString(makeWAVChunkForTest([]byte{1, 2, 3, 4}))
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxAttempts(3)
	audioData, err := p.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  "abc",
		Voice: "BV700_streaming",
	})
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(audioData))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 request without outer retry, got %d", got)
	}
}
