package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/simp-lee/ttsbridge/tts"
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
	return makeWAVChunkWithProfileForTest(24000, 1, 16, pcmData)
}

func makeWAVChunkWithProfileForTest(sampleRate uint32, channels uint16, bitsPerSample uint16, pcmData []byte) []byte {
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	bytesPerSample := uint16(0)
	if bitsPerSample >= 8 {
		bytesPerSample = bitsPerSample / 8
	}
	blockAlign := channels * bytesPerSample
	binary.LittleEndian.PutUint32(header[28:32], sampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], []byte("data"))

	dataSize := len(pcmData)
	fileSize := dataSize + 36
	binary.LittleEndian.PutUint32(header[4:8], uint32(fileSize))
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

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

func makeHeaderOnlyChunkForTest() []byte {
	return makeWAVChunkForTest(nil)
}

func makeChunkWithMismatchedSizesForTest(pcmData []byte, riffSizeDelta, dataSizeDelta int) []byte {
	chunk := makeWAVChunkForTest(pcmData)
	riffSize := int(binary.LittleEndian.Uint32(chunk[4:8])) + riffSizeDelta
	dataSize := int(binary.LittleEndian.Uint32(chunk[40:44])) + dataSizeDelta
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(riffSize))
	binary.LittleEndian.PutUint32(chunk[40:44], uint32(dataSize))
	return chunk
}

func makeChunkWithInconsistentByteRateForTest(pcmData []byte) []byte {
	chunk := makeWAVChunkForTest(pcmData)
	binary.LittleEndian.PutUint32(chunk[28:32], binary.LittleEndian.Uint32(chunk[28:32])+1)
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abcdef", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeMultiChunkRebuildsWAVHeaderAndConcatenatesPCM(t *testing.T) {
	chunks := [][]byte{
		makeWAVChunkForTest([]byte{1, 2, 3, 4}),
		makeWAVChunkForTest([]byte{5, 6, 7, 8, 9, 10}),
	}
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := int(atomic.AddInt32(&calls, 1) - 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(chunks[current])
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1).WithMaxAttempts(1)
	result, err := p.Synthesize(context.Background(), plainTextRequest("ab", "BV700_streaming"))
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}

	wantPCM := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := result.Audio[44:]; string(got) != string(wantPCM) {
		t.Fatalf("PCM payload = %v, want %v", got, wantPCM)
	}
	if got := binary.LittleEndian.Uint32(result.Audio[4:8]); got != uint32(len(result.Audio)-8) {
		t.Fatalf("RIFF size = %d, want %d", got, len(result.Audio)-8)
	}
	if got := binary.LittleEndian.Uint32(result.Audio[40:44]); got != uint32(len(wantPCM)) {
		t.Fatalf("data size = %d, want %d", got, len(wantPCM))
	}
	expectedDuration, err := tts.InferDuration(result.Audio, tts.VoiceAudioProfile{Format: tts.AudioFormatWAV})
	if err != nil {
		t.Fatalf("InferDuration() error: %v", err)
	}
	if result.Format != tts.AudioFormatWAV {
		t.Fatalf("result.Format = %q, want %q", result.Format, tts.AudioFormatWAV)
	}
	if result.SampleRate != 24000 {
		t.Fatalf("result.SampleRate = %d, want %d", result.SampleRate, 24000)
	}
	if result.Duration != expectedDuration {
		t.Fatalf("result.Duration = %v, want %v", result.Duration, expectedDuration)
	}
	if result.Provider != "volcengine" {
		t.Fatalf("result.Provider = %q, want %q", result.Provider, "volcengine")
	}
	if result.VoiceID != "BV700_streaming" {
		t.Fatalf("result.VoiceID = %q, want %q", result.VoiceID, "BV700_streaming")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want 2", got)
	}
}

func TestSynthesizeMultiChunkRejectsMismatchedWAVProfile(t *testing.T) {
	chunks := [][]byte{
		makeWAVChunkForTest([]byte{1, 2, 3, 4}),
		makeWAVChunkWithProfileForTest(16000, 1, 16, []byte{5, 6, 7, 8}),
	}
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := int(atomic.AddInt32(&calls, 1) - 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(chunks[current])
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1).WithMaxAttempts(1)
	result, err := p.Synthesize(context.Background(), plainTextRequest("ab", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
	}
	if !strings.Contains(err.Error(), "wav profile mismatch with first chunk") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d; want 2", got)
	}
}

func TestSynthesizeSingleChunkUsesReturnedWAVMetadata(t *testing.T) {
	chunk := makeWAVChunkWithProfileForTest(16000, 1, 16, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		resp := translateResponseStub{}
		resp.BaseResp.StatusCode = 0
		resp.Audio.Data = base64.StdEncoding.EncodeToString(chunk)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxAttempts(1)
	result, err := p.Synthesize(context.Background(), plainTextRequest("hello", "BV700_streaming"))
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}

	expectedDuration, err := tts.InferDuration(result.Audio, tts.VoiceAudioProfile{Format: tts.AudioFormatWAV})
	if err != nil {
		t.Fatalf("InferDuration() error: %v", err)
	}
	if got := binary.LittleEndian.Uint32(result.Audio[24:28]); got != 16000 {
		t.Fatalf("wav header sample rate = %d, want %d", got, 16000)
	}
	if result.Format != tts.AudioFormatWAV {
		t.Fatalf("result.Format = %q, want %q", result.Format, tts.AudioFormatWAV)
	}
	if result.SampleRate != 16000 {
		t.Fatalf("result.SampleRate = %d, want %d", result.SampleRate, 16000)
	}
	if result.Duration != expectedDuration {
		t.Fatalf("result.Duration = %v, want %v", result.Duration, expectedDuration)
	}
	if result.Provider != "volcengine" {
		t.Fatalf("result.Provider = %q, want %q", result.Provider, "volcengine")
	}
	if result.VoiceID != "BV700_streaming" {
		t.Fatalf("result.VoiceID = %q, want %q", result.VoiceID, "BV700_streaming")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d; want 1", got)
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abcdef", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abcdef", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abcdef", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abc", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
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
	result, err := p.Synthesize(context.Background(), plainTextRequest("abc", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
	}
	if !strings.Contains(err.Error(), "no valid wav chunk header found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 request without outer retry, got %d", got)
	}
}

func TestSynthesizeMultiChunkFailureIncludesChunkContext(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&calls, 1)
		if current == 1 {
			resp := translateResponseStub{}
			resp.BaseResp.StatusCode = 0
			resp.Audio.Data = base64.StdEncoding.EncodeToString(makeWAVChunkForTest([]byte{1, 2, 3, 4}))
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "upstream overloaded", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := New().WithBaseURL(server.URL).WithMaxTextBytes(1).WithMaxAttempts(1)
	result, err := p.Synthesize(context.Background(), plainTextRequest("ab", "BV700_streaming"))
	if err == nil {
		t.Fatalf("expected error, got nil with audio len=%d", len(result.Audio))
	}
	if !strings.Contains(err.Error(), "chunk 2/2 failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T: %v", err, err)
	}
	if ttsErr.Code != tts.ErrCodeProviderUnavail {
		t.Fatalf("Code = %s; want %s", ttsErr.Code, tts.ErrCodeProviderUnavail)
	}
}
