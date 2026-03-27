package edgetts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/ttsbridge/tts"
)

// TestFormatProsody 测试 rate/volume 格式化函数
func TestFormatProsody(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"默认值 (1.0)", 1.0, "+0%"},
		{"零值", 0, "+0%"},
		{"提高 50%", 1.5, "+50%"},
		{"提高 100% (翻倍)", 2.0, "+100%"},
		{"降低 50%", 0.5, "-50%"},
		{"降低 20%", 0.8, "-20%"},
		{"提高 33%", 1.33, "+33%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProsody(tt.value)
			if result != tt.expected {
				t.Errorf("formatProsody(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

type trackingReadCloser struct {
	reader io.Reader
	closed atomic.Bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

type failOnLargeReadCloser struct {
	remaining int64
	closed    atomic.Bool
}

func (r *failOnLargeReadCloser) Read(p []byte) (int, error) {
	if int64(len(p)) > r.remaining {
		return 0, io.ErrShortBuffer
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

func (r *failOnLargeReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

// TestFormatPitch 测试 pitch 格式化函数
func TestFormatPitch(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"默认值 (1.0)", 1.0, "+0Hz"},
		{"零值", 0, "+0Hz"},
		{"提高 50%", 1.5, "+50%"},
		{"降低 50%", 0.5, "-50%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPitch(tt.value)
			if result != tt.expected {
				t.Errorf("formatPitch(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestProviderSynthesize_NilOptions(t *testing.T) {
	provider := New()

	_, err := provider.Synthesize(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesizeStream_NilOptions(t *testing.T) {
	provider := New()

	_, err := provider.SynthesizeStream(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesize_EmptyAfterNormalization(t *testing.T) {
	provider := New()

	_, err := provider.Synthesize(context.Background(), &SynthesizeOptions{
		Text:  " \t\n ",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err == nil {
		t.Fatal("expected error for empty normalized text")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func TestProviderSynthesizeStream_EmptyAfterNormalization(t *testing.T) {
	provider := New()

	_, err := provider.SynthesizeStream(context.Background(), &SynthesizeOptions{
		Text:  "\n\t ",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err == nil {
		t.Fatal("expected error for empty normalized text")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("expected *tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("expected code %s, got %s", tts.ErrCodeInvalidInput, ttsErr.Code)
	}
}

func nilContextForTest() context.Context {
	var ctx context.Context
	return ctx
}

func installSuccessfulSynthesisHook(t *testing.T, provider *Provider) {
	t.Helper()

	provider.connectHook = func(ctx context.Context) (*websocket.Conn, error) {
		if ctx == nil {
			t.Fatal("connectHook received nil context")
		}
		return newTestWebsocketConn(t, func(serverCtx context.Context, ws *websocket.Conn) {
			if _, _, err := ws.Read(serverCtx); err != nil {
				t.Errorf("read config: %v", err)
				return
			}
			if _, _, err := ws.Read(serverCtx); err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}
			if err := ws.Write(serverCtx, websocket.MessageBinary, []byte{0x00, 0x00, 'o', 'k'}); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(serverCtx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}
}

func TestProviderSynthesize_NilContextUsesBackground(t *testing.T) {
	provider := New()
	installSuccessfulSynthesisHook(t, provider)

	audio, err := provider.Synthesize(nilContextForTest(), &SynthesizeOptions{
		Text:  "hello",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err != nil {
		t.Fatalf("Synthesize(nil, ...) error: %v", err)
	}
	if string(audio) != "ok" {
		t.Fatalf("audio = %q; want %q", string(audio), "ok")
	}
}

func TestProviderSynthesizeStream_NilContextUsesBackground(t *testing.T) {
	provider := New()
	installSuccessfulSynthesisHook(t, provider)

	stream, err := provider.SynthesizeStream(nilContextForTest(), &SynthesizeOptions{
		Text:  "hello",
		Voice: "en-US-AvaMultilingualNeural",
	})
	if err != nil {
		t.Fatalf("SynthesizeStream(nil, ...) error: %v", err)
	}
	t.Cleanup(func() {
		_ = stream.Close()
	})

	chunk, err := stream.Read()
	if err != nil {
		t.Fatalf("stream.Read() error: %v", err)
	}
	if string(chunk) != "ok" {
		t.Fatalf("stream chunk = %q; want %q", string(chunk), "ok")
	}

	_, err = stream.Read()
	if err != io.EOF {
		t.Fatalf("final stream.Read() error = %v; want io.EOF", err)
	}
}

func TestProviderIsAvailable_NilContextUsesBackground(t *testing.T) {
	provider := New()
	provider.connectHook = func(ctx context.Context) (*websocket.Conn, error) {
		if ctx == nil {
			t.Fatal("connectHook received nil context")
		}
		return newTestWebsocketConn(t, func(context.Context, *websocket.Conn) {}), nil
	}

	if !provider.IsAvailable(nilContextForTest()) {
		t.Fatal("IsAvailable(nil) = false; want true")
	}
}

func TestSplitPreparedText_AccountsForSSMLWrapperOverhead(t *testing.T) {
	voice := strings.Repeat("v", 256)
	budget := defaultMaxSSMLBytes - ssmlWrapperBytes(&SynthesizeOptions{Voice: voice, Rate: 1.25})
	if budget <= 0 {
		t.Fatalf("budget = %d; want positive", budget)
	}

	opts := &SynthesizeOptions{
		Text:  strings.Repeat("a", budget+1),
		Voice: voice,
		Rate:  1.25,
	}

	chunks, err := splitPreparedText(opts)
	if err != nil {
		t.Fatalf("splitPreparedText() error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("splitPreparedText() chunks = %d; want 2", len(chunks))
	}

	for i, chunk := range chunks {
		chunkOpts := *opts
		chunkOpts.Text = chunk
		if got := len([]byte(buildSSML(&chunkOpts))); got > defaultMaxSSMLBytes {
			t.Fatalf("chunk %d SSML size = %d; want <= %d", i, got, defaultMaxSSMLBytes)
		}
	}
}

func TestWithProxy_ClearResetsHTTPTransport(t *testing.T) {
	p := New().WithProxy("http://127.0.0.1:8080")
	if p.client.Transport == nil {
		t.Fatal("expected proxy transport to be configured")
	}

	p.WithProxy("")
	if p.proxyURL != "" {
		t.Fatalf("proxyURL = %q; want empty", p.proxyURL)
	}
	if p.client.Transport != nil {
		t.Fatalf("client transport = %#v; want nil", p.client.Transport)
	}
}

func TestWithProxy_InvalidProxyReturnsRuntimeConfigError(t *testing.T) {
	p := New().WithProxy("://bad-proxy")

	_, err := p.ListVoices(context.Background(), "")
	if err == nil {
		t.Fatal("expected invalid proxy error, got nil")
	}

	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("error = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeInvalidInput {
		t.Fatalf("Code = %s; want %s", ttsErr.Code, tts.ErrCodeInvalidInput)
	}
	if !strings.Contains(err.Error(), "invalid proxy URL") {
		t.Fatalf("error = %v; want invalid proxy URL context", err)
	}
	if p.IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = true; want false for invalid proxy config")
	}

	p.WithProxy("")
	if p.proxyErr != nil {
		t.Fatalf("proxyErr = %v; want nil after clearing proxy", p.proxyErr)
	}
}

// --- VoiceCache integration tests ---

// mockVoiceEntries returns a small set of voiceListEntry for unit testing.
func mockVoiceEntries() []voiceListEntry {
	return []voiceListEntry{
		{
			Name:      "Edge Voice",
			ShortName: "en-US-TestNeural",
			Gender:    "Female",
			Locale:    "en-US",
			Status:    "GA",
		},
		{
			Name:      "Chinese Voice",
			ShortName: "zh-CN-XiaoxiaoNeural",
			Gender:    "Female",
			Locale:    "zh-CN",
			Status:    "GA",
		},
	}
}

func TestWithVoiceCache_ListVoicesUsesCache(t *testing.T) {
	var fetchCount atomic.Int32

	p := New()
	p.voiceCache = tts.NewVoiceCache(func(ctx context.Context) ([]tts.Voice, error) {
		fetchCount.Add(1)
		return filterAndConvertVoices(mockVoiceEntries(), ""), nil
	})

	ctx := context.Background()

	// First call: should trigger fetcher
	voices, err := p.ListVoices(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected 1 fetch call, got %d", fetchCount.Load())
	}

	// Second call: should hit cache, no additional fetch
	voices2, err := p.ListVoices(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices2) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices2))
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected still 1 fetch call, got %d", fetchCount.Load())
	}
}

func TestWithVoiceCache_ListVoicesFiltersLocale(t *testing.T) {
	p := New()
	p.voiceCache = tts.NewVoiceCache(func(ctx context.Context) ([]tts.Voice, error) {
		return filterAndConvertVoices(mockVoiceEntries(), ""), nil
	})

	ctx := context.Background()
	voices, err := p.ListVoices(ctx, "zh-CN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 1 {
		t.Fatalf("expected 1 voice for zh-CN, got %d", len(voices))
	}
	if voices[0].ID != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("expected zh-CN-XiaoxiaoNeural, got %s", voices[0].ID)
	}
}

func TestWithVoiceCache_BuilderReturnsSelf(t *testing.T) {
	p := New().WithVoiceCache()
	if p.voiceCache == nil {
		t.Fatal("expected voiceCache to be set after WithVoiceCache()")
	}
}

func TestWithVoiceCache_DefaultTTLIs24Hours(t *testing.T) {
	p := New().WithVoiceCache()
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})

	if p.voiceCache == nil {
		t.Fatal("expected voiceCache to be initialized")
	}

	ttlField := reflect.ValueOf(p.voiceCache).Elem().FieldByName("ttl")
	if !ttlField.IsValid() {
		t.Fatal("voiceCache ttl field not found")
	}
	if ttlField.Kind() != reflect.Int64 {
		t.Fatalf("voiceCache ttl kind = %v; want Int64", ttlField.Kind())
	}

	if got := time.Duration(ttlField.Int()); got != 24*time.Hour {
		t.Fatalf("WithVoiceCache default ttl = %v; want %v", got, 24*time.Hour)
	}
}

func TestClose_Idempotent(t *testing.T) {
	p := New().WithVoiceCache()
	if err := p.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

func TestClose_NilCache(t *testing.T) {
	p := New()
	if err := p.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestConnect_ForbiddenWithGenericDateStaysAuthFailed(t *testing.T) {
	provider := New()
	originalDial := websocketDial
	websocketDial = func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
		return nil, &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Date": []string{"Wed, 25 Mar 2026 10:00:00 GMT"},
			},
			Body: io.NopCloser(strings.NewReader("Forbidden")),
		}, io.ErrUnexpectedEOF
	}
	t.Cleanup(func() {
		websocketDial = originalDial
	})

	_, err := provider.connect(context.Background())
	if err == nil {
		t.Fatal("connect() error = nil, want auth failure")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("connect() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeAuthFailed {
		t.Fatalf("connect() code = %s, want %s", ttsErr.Code, tts.ErrCodeAuthFailed)
	}
}

func TestConnect_ForbiddenWithMalformedClockSkewDateStaysAuthFailed(t *testing.T) {
	provider := New()
	originalDial := websocketDial
	websocketDial = func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
		return nil, &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Date": []string{"not-a-date"},
			},
			Body: io.NopCloser(strings.NewReader("Request timestamp expired")),
		}, io.ErrUnexpectedEOF
	}
	t.Cleanup(func() {
		websocketDial = originalDial
	})

	_, err := provider.connect(context.Background())
	if err == nil {
		t.Fatal("connect() error = nil, want auth failure")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("connect() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeAuthFailed {
		t.Fatalf("connect() code = %s, want %s", ttsErr.Code, tts.ErrCodeAuthFailed)
	}
	if ttsErr.Err == nil || !strings.Contains(ttsErr.Err.Error(), "failed to parse server date") {
		t.Fatalf("connect() wrapped err = %v, want parse failure detail", ttsErr.Err)
	}
}

func TestConnect_NonForbiddenHandshakeFailureClosesBodyWithoutSurfacingPayload(t *testing.T) {
	provider := New()
	body := &trackingReadCloser{reader: strings.NewReader("sensitive upstream payload")}
	originalDial := websocketDial
	websocketDial = func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
		return nil, &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       body,
		}, io.ErrUnexpectedEOF
	}
	t.Cleanup(func() {
		websocketDial = originalDial
	})

	_, err := provider.connect(context.Background())
	if err == nil {
		t.Fatal("connect() error = nil, want websocket failure")
	}
	if !body.closed.Load() {
		t.Fatal("expected non-403 handshake response body to be closed")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("connect() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeWebSocketError {
		t.Fatalf("connect() code = %s; want %s", ttsErr.Code, tts.ErrCodeWebSocketError)
	}
	if strings.Contains(err.Error(), "sensitive upstream payload") {
		t.Fatal("connect() error should not surface raw handshake response payload")
	}
}

func TestConnect_ForbiddenHandshakeFailureReadsBoundedBodyPrefix(t *testing.T) {
	provider := New()
	body := &failOnLargeReadCloser{remaining: handshakeErrorBodyReadLimit}
	originalDial := websocketDial
	websocketDial = func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
		return nil, &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"Date": []string{"Wed, 25 Mar 2026 10:00:00 GMT"},
			},
			Body: body,
		}, io.ErrUnexpectedEOF
	}
	t.Cleanup(func() {
		websocketDial = originalDial
	})

	_, err := provider.connect(context.Background())
	if err == nil {
		t.Fatal("connect() error = nil, want auth failure")
	}
	if !body.closed.Load() {
		t.Fatal("expected forbidden handshake response body to be closed")
	}

	ttsErr, ok := err.(*tts.Error)
	if !ok {
		t.Fatalf("connect() error type = %T, want *tts.Error", err)
	}
	if ttsErr.Code != tts.ErrCodeAuthFailed {
		t.Fatalf("connect() code = %s, want %s", ttsErr.Code, tts.ErrCodeAuthFailed)
	}
}

func TestFormatRegistryProbe_NilContextUsesBackgroundThroughProvider(t *testing.T) {
	provider := New()
	installSuccessfulSynthesisHook(t, provider)

	probed, err := provider.FormatRegistry().Probe(nilContextForTest(), OutputFormatMP3_24khz_48k)
	if err != nil {
		t.Fatalf("Probe(nil, ...) error: %v", err)
	}
	if probed.Status != tts.FormatAvailable {
		t.Fatalf("status = %v; want %v", probed.Status, tts.FormatAvailable)
	}
}
