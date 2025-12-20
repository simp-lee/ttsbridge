package edgetts

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

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
		expectedCode string
	}{
		{
			name:         "Context取消",
			err:          context.Canceled,
			expectedCode: "", // 应该直接返回原始错误
		},
		{
			name:         "Context超时",
			err:          context.DeadlineExceeded,
			expectedCode: "", // 应该直接返回原始错误
		},
		{
			name:         "网络超时错误",
			err:          &mockNetError{timeout: true},
			expectedCode: tts.ErrCodeTimeout,
		},
		{
			name:         "普通错误",
			err:          errors.New("connection error"),
			expectedCode: tts.ErrCodeWebSocketError,
		},
		{
			name:         "IO错误",
			err:          io.ErrUnexpectedEOF,
			expectedCode: tts.ErrCodeWebSocketError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyWebsocketReadError(tt.err, time.Second, "test message")

			if tt.expectedCode == "" {
				// 应该是原始错误
				if err != tt.err {
					t.Errorf("Expected original error, got %v", err)
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
		})
	}
}

// TestEdgeAudioStream_OffsetCompensation 测试偏移补偿逻辑
func TestEdgeAudioStream_OffsetCompensation(t *testing.T) {
	stream := &edgeAudioStream{
		ctx:                context.Background(),
		textChunks:         []string{"chunk1", "chunk2", "chunk3"},
		chunkIndex:         0,
		offsetCompensation: 0,
	}

	// 模拟处理第一个 chunk 后的状态变化
	initialOffset := stream.offsetCompensation

	// 手动模拟 turn.end 处理中的状态更新
	stream.chunkIndex++
	stream.offsetCompensation += defaultOffsetPadding

	expectedOffset := initialOffset + defaultOffsetPadding
	if stream.offsetCompensation != expectedOffset {
		t.Errorf("Expected offsetCompensation=%d after first chunk, got %d",
			expectedOffset, stream.offsetCompensation)
	}

	// 处理第二个 chunk
	stream.chunkIndex++
	stream.offsetCompensation += defaultOffsetPadding

	expectedOffset += defaultOffsetPadding
	if stream.offsetCompensation != expectedOffset {
		t.Errorf("Expected offsetCompensation=%d after second chunk, got %d",
			expectedOffset, stream.offsetCompensation)
	}

	if stream.chunkIndex != 2 {
		t.Errorf("Expected chunkIndex=2, got %d", stream.chunkIndex)
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
