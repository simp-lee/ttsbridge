package edgetts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/ttsbridge/tts"
)

// TestEdgeAudioStream_AlreadyClosed 测试已关闭的流
func TestEdgeAudioStream_AlreadyClosed(t *testing.T) {
	stream := &edgeAudioStream{
		ctx:        context.Background(),
		textChunks: []string{"test"},
		closed:     true,
	}

	_, err := stream.Read()
	if err != io.EOF {
		t.Errorf("Expected io.EOF for closed stream, got %v", err)
	}
}

// TestEdgeAudioStream_EmptyChunks 测试空文本块列表
func TestEdgeAudioStream_EmptyChunks(t *testing.T) {
	stream := &edgeAudioStream{
		ctx:        context.Background(),
		textChunks: []string{},
		chunkIndex: 0,
		opts:       &SynthesizeOptions{},
		provider:   &Provider{maxAttempts: 1, receiveTimeout: time.Second},
	}

	_, err := stream.Read()
	if err != io.EOF {
		t.Errorf("Expected io.EOF for empty chunks, got %v", err)
	}
	if !stream.closed {
		t.Error("Stream should be closed after EOF")
	}
}

// TestEdgeAudioStream_ContextCancellation 测试 context 取消
func TestEdgeAudioStream_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	stream := &edgeAudioStream{
		ctx:        ctx,
		textChunks: []string{"test"},
		chunkIndex: 0,
		opts:       &SynthesizeOptions{},
		provider:   &Provider{maxAttempts: 1, receiveTimeout: time.Second},
	}

	_, err := stream.Read()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
	if !stream.closed {
		t.Error("Stream should be closed after context cancellation")
	}
}

// TestEdgeAudioStream_Close 测试主动关闭流
func TestEdgeAudioStream_Close(t *testing.T) {
	stream := &edgeAudioStream{
		ctx:        context.Background(),
		textChunks: []string{"test"},
		conn:       nil, // 没有实际连接
	}

	err := stream.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
	if !stream.closed {
		t.Error("Stream should be marked as closed")
	}
	if stream.conn != nil {
		t.Error("Connection should be nil after close")
	}

	// 再次关闭应该是幂等的
	err = stream.Close()
	if err != nil {
		t.Errorf("Second Close() should succeed, got %v", err)
	}
}

// TestEdgeAudioStream_ResetConnection 测试连接重置
func TestEdgeAudioStream_ResetConnection(t *testing.T) {
	stream := &edgeAudioStream{
		conn: nil,
	}

	// 重置空连接应该安全
	stream.resetConnection()
	if stream.conn != nil {
		t.Error("Connection should remain nil")
	}
}

// TestEdgeAudioStream_ChunkIndexBoundary 测试 chunkIndex 边界条件
func TestEdgeAudioStream_ChunkIndexBoundary(t *testing.T) {
	tests := []struct {
		name         string
		textChunks   []string
		chunkIndex   int
		expectEOF    bool
		expectClosed bool
	}{
		{
			name:         "索引超出范围",
			textChunks:   []string{"chunk1"},
			chunkIndex:   1,
			expectEOF:    true,
			expectClosed: true,
		},
		{
			name:         "索引远超范围",
			textChunks:   []string{"chunk1"},
			chunkIndex:   10,
			expectEOF:    true,
			expectClosed: true,
		},
		{
			name:         "空chunks且索引为0",
			textChunks:   []string{},
			chunkIndex:   0,
			expectEOF:    true,
			expectClosed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &edgeAudioStream{
				ctx:        context.Background(),
				textChunks: tt.textChunks,
				chunkIndex: tt.chunkIndex,
				opts:       &SynthesizeOptions{},
				provider:   &Provider{maxAttempts: 1, receiveTimeout: time.Millisecond * 100},
			}

			_, err := stream.Read()

			if tt.expectEOF {
				if err != io.EOF {
					t.Errorf("Expected io.EOF, got %v", err)
				}
			}

			if tt.expectClosed != stream.closed {
				t.Errorf("Expected closed=%v, got %v", tt.expectClosed, stream.closed)
			}
		})
	}
}

// TestExtractAudioData 测试音频数据提取
func TestExtractAudioData(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "正常消息",
			input:    []byte{0x00, 0x02, 'I', 'D', 0x01, 0x02, 0x03},
			expected: []byte{0x01, 0x02, 0x03},
		},
		{
			name:     "空消息",
			input:    []byte{},
			expected: nil,
		},
		{
			name:     "只有1字节",
			input:    []byte{0x00},
			expected: nil,
		},
		{
			name:     "只有头部",
			input:    []byte{0x00, 0x02},
			expected: nil,
		},
		{
			name:     "头部长度为0",
			input:    []byte{0x00, 0x00, 0x01, 0x02},
			expected: []byte{0x01, 0x02},
		},
		{
			name:     "大头部长度",
			input:    []byte{0x00, 0x04, 'T', 'E', 'S', 'T', 0x01, 0x02},
			expected: []byte{0x01, 0x02},
		},
		{
			name:     "头部长度等于消息长度",
			input:    []byte{0x00, 0x02, 'I', 'D'},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAudioData(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("At index %d: expected %d, got %d", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

// mockNetError 用于测试网络错误
type mockNetError struct {
	timeout bool
}

func (e *mockNetError) Error() string   { return "mock network error" }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return false }

// TestClassifyWebsocketReadError 测试 WebSocket 读取错误分类
func TestClassifyWebsocketReadError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		parentCtxErr error
		expectedCode string
		expectRetry  bool
	}{
		{
			name:         "父Context取消",
			err:          context.Canceled,
			parentCtxErr: context.Canceled,
			expectedCode: "", // 应该直接返回父context错误
			expectRetry:  false,
		},
		{
			name:         "父Context超时",
			err:          context.DeadlineExceeded,
			parentCtxErr: context.DeadlineExceeded,
			expectedCode: "", // 应该直接返回父context错误
			expectRetry:  false,
		},
		{
			name:         "内部读取超时应映射为可重试超时错误",
			err:          context.DeadlineExceeded,
			parentCtxErr: nil,
			expectedCode: tts.ErrCodeTimeout,
			expectRetry:  true,
		},
		{
			name:         "网络超时错误",
			err:          &mockNetError{timeout: true},
			expectedCode: tts.ErrCodeTimeout,
			expectRetry:  true,
		},
		{
			name:         "普通错误",
			err:          errors.New("connection error"),
			expectedCode: tts.ErrCodeWebSocketError,
			expectRetry:  true,
		},
		{
			name:         "IO错误",
			err:          io.ErrUnexpectedEOF,
			expectedCode: tts.ErrCodeWebSocketError,
			expectRetry:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyWebsocketReadError(tt.err, tt.parentCtxErr, time.Second, "test message")

			if tt.expectedCode == "" {
				// 应该是父context错误
				if err != tt.parentCtxErr {
					t.Errorf("Expected parent context error, got %v", err)
				}
				if retryable := tts.IsRetryableError(err); retryable != tt.expectRetry {
					t.Errorf("Expected retryable=%v, got %v", tt.expectRetry, retryable)
				}
				return
			}

			var ttsErr *tts.Error
			if !errors.As(err, &ttsErr) {
				t.Fatalf("Expected tts.Error, got %T", err)
			}
			if ttsErr.Code != tt.expectedCode {
				t.Errorf("Expected code %s, got %s", tt.expectedCode, ttsErr.Code)
			}
			if ttsErr.Provider != providerName {
				t.Errorf("Expected provider %s, got %s", providerName, ttsErr.Provider)
			}
			if retryable := tts.IsRetryableError(err); retryable != tt.expectRetry {
				t.Errorf("Expected retryable=%v, got %v", tt.expectRetry, retryable)
			}
		})
	}
}

func TestEdgeAudioStream_HandleReadError(t *testing.T) {
	t.Run("boundary emitted should fail fast without retry", func(t *testing.T) {
		stream := &edgeAudioStream{boundaryEmitted: true}

		err := stream.handleReadError(errors.New("connection reset by peer"), time.Second)

		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("expected tts.Error, got %T", err)
		}
		if ttsErr.Code != tts.ErrCodeInternalError {
			t.Fatalf("expected code %s, got %s", tts.ErrCodeInternalError, ttsErr.Code)
		}
		if !contains(ttsErr.Message, "cannot retry chunk after boundary callback emission") {
			t.Fatalf("unexpected message: %s", ttsErr.Message)
		}
		if tts.IsRetryableError(err) {
			t.Fatalf("expected non-retryable error, got retryable: %v", err)
		}
	})

	t.Run("partial chunk emitted should fail fast without retry", func(t *testing.T) {
		stream := &edgeAudioStream{chunkBytesEmitted: 128}

		err := stream.handleReadError(errors.New("connection reset by peer"), time.Second)

		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("expected tts.Error, got %T", err)
		}
		if ttsErr.Code != tts.ErrCodeInternalError {
			t.Fatalf("expected code %s, got %s", tts.ErrCodeInternalError, ttsErr.Code)
		}
		if !contains(ttsErr.Message, "cannot resume stream after partial chunk emission") {
			t.Fatalf("unexpected message: %s", ttsErr.Message)
		}
		if tts.IsRetryableError(err) {
			t.Fatalf("expected non-retryable error, got retryable: %v", err)
		}
	})

	t.Run("no emitted bytes should keep websocket retry behavior", func(t *testing.T) {
		stream := &edgeAudioStream{}

		err := stream.handleReadError(errors.New("connection reset by peer"), time.Second)

		var ttsErr *tts.Error
		if !errors.As(err, &ttsErr) {
			t.Fatalf("expected tts.Error, got %T", err)
		}
		if ttsErr.Code != tts.ErrCodeWebSocketError {
			t.Fatalf("expected code %s, got %s", tts.ErrCodeWebSocketError, ttsErr.Code)
		}
		if !tts.IsRetryableError(err) {
			t.Fatalf("expected retryable websocket error, got: %v", err)
		}
	})
}

func TestParseAndCallbackMetadata_OffsetsRemainChunkLocal(t *testing.T) {
	provider := New()
	message := []byte("Path:audio.metadata\r\n\r\n{\"Metadata\":[{\"Type\":\"WordBoundary\",\"Data\":{\"Offset\":12345,\"Duration\":678,\"text\":{\"Text\":\"hello\"}}}]}")

	var events []tts.BoundaryEvent
	emitted := provider.parseAndCallbackMetadata(message, func(event tts.BoundaryEvent) {
		events = append(events, event)
	}, 2)

	if !emitted {
		t.Fatal("expected metadata callback to emit events")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d; want 1", len(events))
	}
	if events[0].OffsetMs != 1 {
		t.Fatalf("OffsetMs = %d; want 1", events[0].OffsetMs)
	}
	if events[0].ChunkIndex != 2 {
		t.Fatalf("ChunkIndex = %d; want 2", events[0].ChunkIndex)
	}
	if events[0].Text != "hello" {
		t.Fatalf("Text = %q; want hello", events[0].Text)
	}
}

func TestEdgeAudioStream_TurnEndWithoutAudioReturnsExplicitError(t *testing.T) {
	conn := newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
		if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
			t.Errorf("write turn.end: %v", err)
		}
	})

	stream := &edgeAudioStream{
		ctx:        context.Background(),
		textChunks: []string{"chunk1"},
		chunkIndex: 0,
		opts:       &SynthesizeOptions{},
		provider:   &Provider{maxAttempts: 1, receiveTimeout: time.Second},
		conn:       conn,
	}

	_, err := stream.Read()
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeNoAudioReceived {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeNoAudioReceived)
	}
	if !stream.closed {
		t.Fatal("stream should be closed after explicit no-audio failure")
	}
}

func TestEdgeAudioStream_MetadataEmissionReturnsExplicitFailure(t *testing.T) {
	conn := newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
		metadata := "Path:audio.metadata\r\n\r\n{\"Metadata\":[{\"Type\":\"WordBoundary\",\"Data\":{\"Offset\":20000,\"Duration\":5000,\"text\":{\"Text\":\"hello\"}}}]}"
		if err := ws.Write(ctx, websocket.MessageText, []byte(metadata)); err != nil {
			t.Errorf("write metadata: %v", err)
			return
		}
		_ = ws.Close(websocket.StatusInternalError, "boom")
	})

	callbackCount := 0
	stream := &edgeAudioStream{
		ctx:        context.Background(),
		textChunks: []string{"chunk1"},
		chunkIndex: 0,
		opts: &SynthesizeOptions{BoundaryCallback: func(tts.BoundaryEvent) {
			callbackCount++
		}},
		provider: &Provider{maxAttempts: 2, receiveTimeout: time.Second},
		conn:     conn,
	}

	_, err := stream.Read()
	var ttsErr *tts.Error
	if !errors.As(err, &ttsErr) {
		t.Fatalf("expected tts.Error, got %T", err)
	}
	if ttsErr.Code != tts.ErrCodeInternalError {
		t.Fatalf("code = %s; want %s", ttsErr.Code, tts.ErrCodeInternalError)
	}
	if callbackCount != 1 {
		t.Fatalf("callback count = %d; want 1", callbackCount)
	}
	if tts.IsRetryableError(err) {
		t.Fatalf("expected non-retryable error, got %v", err)
	}
}

func TestEdgeAudioStream_DoesNotReplayCommittedChunkAfterNextChunkInitRetry(t *testing.T) {
	ctx := context.Background()
	provider := New().WithMaxAttempts(2)
	provider.receiveTimeout = time.Second

	connectCalls := 0
	provider.connectHook = func(context.Context) (*websocket.Conn, error) {
		connectCalls++
		if connectCalls == 2 {
			return nil, &tts.Error{Code: tts.ErrCodeWebSocketError, Message: "transient connect failure", Provider: providerName}
		}

		return newTestWebsocketConn(t, func(ctx context.Context, ws *websocket.Conn) {
			_, config, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read config: %v", err)
				return
			}
			_ = config

			_, ssml, err := ws.Read(ctx)
			if err != nil {
				t.Errorf("read ssml: %v", err)
				return
			}

			payload := []byte("A")
			if strings.Contains(string(ssml), ">chunk-1<") {
				payload = []byte("B")
			}

			message := append([]byte{0x00, 0x00}, payload...)
			if err := ws.Write(ctx, websocket.MessageBinary, message); err != nil {
				t.Errorf("write audio: %v", err)
				return
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte("Path:turn.end\r\n\r\n")); err != nil {
				t.Errorf("write turn.end: %v", err)
			}
		}), nil
	}

	stream := &edgeAudioStream{
		ctx:        ctx,
		textChunks: []string{"chunk-0", "chunk-1"},
		opts: &SynthesizeOptions{
			Voice: defaultVoice,
		},
		provider: provider,
	}

	first, err := stream.Read()
	if err != nil {
		t.Fatalf("first Read() error: %v", err)
	}
	if string(first) != "A" {
		t.Fatalf("first Read() = %q; want %q", string(first), "A")
	}

	second, err := stream.Read()
	if err != nil {
		t.Fatalf("second Read() error: %v", err)
	}
	if string(second) != "B" {
		t.Fatalf("second Read() = %q; want %q", string(second), "B")
	}
	if connectCalls != 3 {
		t.Fatalf("connect call count = %d; want 3", connectCalls)
	}

	_, err = stream.Read()
	if err != io.EOF {
		t.Fatalf("final Read() error = %v; want io.EOF", err)
	}
}

// TestBuildSSML 测试 SSML 构建
func TestBuildSSML(t *testing.T) {
	tests := []struct {
		name     string
		opts     *SynthesizeOptions
		contains []string
	}{
		{
			name: "基本选项",
			opts: &SynthesizeOptions{
				Text:   "Hello World",
				Voice:  "en-US-JennyNeural",
				Rate:   1.0,
				Volume: 1.0,
				Pitch:  1.0,
			},
			contains: []string{
				"<speak",
				"en-US-JennyNeural",
				"Hello World",
				"rate='+0%'",
				"volume='+0%'",
				"pitch='+0Hz'",
			},
		},
		{
			name: "空Voice使用默认",
			opts: &SynthesizeOptions{
				Text: "测试",
			},
			contains: []string{
				defaultVoice,
				"测试",
			},
		},
		{
			name: "调整语速和音量",
			opts: &SynthesizeOptions{
				Text:   "Fast and loud",
				Voice:  "en-US-GuyNeural",
				Rate:   1.5,
				Volume: 1.2,
				Pitch:  0.8,
			},
			contains: []string{
				"rate='+50%'",
				"volume='+20%'",
				"pitch='-20%'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSSML(tt.opts)
			for _, substr := range tt.contains {
				if !contains(result, substr) {
					t.Errorf("Expected SSML to contain '%s', but it doesn't.\nSSML: %s", substr, result)
				}
			}
		})
	}
}

// contains 辅助函数检查字符串包含关系
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexInString(s, substr) >= 0)
}

func indexInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func newTestWebsocketConn(t *testing.T, handler func(context.Context, *websocket.Conn)) *websocket.Conn {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket accept failed: %v", err)
			return
		}
		defer closeNowIgnoreError(ws)
		handler(r.Context(), ws)
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	t.Cleanup(func() {
		closeNowIgnoreError(conn)
	})
	return conn
}
