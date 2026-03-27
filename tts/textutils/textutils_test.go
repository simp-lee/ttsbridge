package textutils

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitByByteLength(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		opts    *SplitOptions
		wantMin int
		wantMax int
	}{
		{
			name:    "短文本无需切割",
			text:    "Hello, world!",
			opts:    &SplitOptions{MaxBytes: 100},
			wantMin: 1,
			wantMax: 1,
		},
		{
			name:    "长英文文本",
			text:    strings.Repeat("Hello world. ", 500),
			opts:    &SplitOptions{MaxBytes: 100},
			wantMin: 60,
			wantMax: 75,
		},
		{
			name:    "长中文文本",
			text:    strings.Repeat("你好世界。", 200),
			opts:    &SplitOptions{MaxBytes: 100},
			wantMin: 28,
			wantMax: 36,
		},
		{
			name: "HTML实体保护",
			text: strings.Repeat("Tom &amp; Jerry. ", 100),
			opts: &SplitOptions{
				MaxBytes:             50,
				PreserveHTMLEntities: true,
			},
			wantMin: 30,
			wantMax: 55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitByByteLength(tt.text, tt.opts)

			if len(chunks) < tt.wantMin || len(chunks) > tt.wantMax {
				t.Errorf("got %d chunks, want between %d and %d", len(chunks), tt.wantMin, tt.wantMax)
			}

			// 验证每个块的大小
			for i, chunk := range chunks {
				if len(chunk) > tt.opts.MaxBytes {
					t.Errorf("chunk %d exceeds max bytes: %d > %d", i, len(chunk), tt.opts.MaxBytes)
				}

				// 验证 UTF-8 有效性
				if !utf8.ValidString(chunk) {
					t.Errorf("chunk %d contains invalid UTF-8", i)
				}
			}

			if joined := strings.Join(chunks, ""); joined != strings.TrimSpace(tt.text) {
				t.Errorf("joined chunks = %q, want %q", joined, strings.TrimSpace(tt.text))
			}
		})
	}
}

func TestSplitByByteLengthPreserveEntities(t *testing.T) {
	text := strings.Repeat("Tom &amp; Jerry ", 10) + "Tom &amp; Jerry"
	chunks := SplitByByteLength(text, &SplitOptions{
		MaxBytes:             60,
		PreserveHTMLEntities: true,
	})

	if len(chunks) == 0 {
		t.Fatalf("expected non-empty chunks")
	}

	for i, chunk := range chunks {
		if !utf8.ValidString(chunk) {
			t.Fatalf("chunk %d is not valid UTF-8", i)
		}
		if strings.Count(chunk, "&") > 0 {
			if strings.Count(chunk, "&") != strings.Count(chunk, "&amp;") {
				t.Fatalf("chunk %d splits HTML entity: %q", i, chunk)
			}
		}
	}
}

func TestSplitByByteLength_TinyMaxBytesKeepsUTF8Intact(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want string
	}{
		{name: "single chinese rune", text: "你好吗", max: 1, want: "你"},
		{name: "single emoji", text: "🙂abc", max: 2, want: "🙂"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitByByteLength(tt.text, &SplitOptions{MaxBytes: tt.max})
			if len(chunks) == 0 {
				t.Fatal("expected non-empty chunks")
			}
			if chunks[0] != tt.want {
				t.Fatalf("first chunk = %q, want %q", chunks[0], tt.want)
			}
			if len(chunks[0]) <= tt.max {
				t.Fatalf("first chunk should exceed max bytes when no smaller safe split exists: len=%d max=%d", len(chunks[0]), tt.max)
			}
			for i, chunk := range chunks {
				if !utf8.ValidString(chunk) {
					t.Fatalf("chunk %d contains invalid UTF-8: %q", i, chunk)
				}
			}
		})
	}
}

func TestSplitByByteLength_TinyMaxBytesKeepsHTMLEntitiesIntact(t *testing.T) {
	text := "&amp;hello"
	chunks := SplitByByteLength(text, &SplitOptions{
		MaxBytes:             1,
		PreserveHTMLEntities: true,
	})

	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks")
	}
	if chunks[0] != "&amp;" {
		t.Fatalf("first chunk = %q, want %q", chunks[0], "&amp;")
	}
	if len(chunks[0]) <= 1 {
		t.Fatalf("first chunk should exceed max bytes when preserving an entity, len=%d", len(chunks[0]))
	}
	for i, chunk := range chunks {
		if !utf8.ValidString(chunk) {
			t.Fatalf("chunk %d contains invalid UTF-8: %q", i, chunk)
		}
		if strings.Contains(chunk, "&") && !strings.Contains(chunk, ";") {
			t.Fatalf("chunk %d contains a broken HTML entity: %q", i, chunk)
		}
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name string
		text string
		opts *CleanOptions
		want string
	}{
		{
			name: "无需清理",
			text: "Hello, world!",
			opts: nil,
			want: "Hello, world!",
		},
		{
			name: "转义HTML",
			text: "Tom & Jerry",
			opts: &CleanOptions{EscapeHTML: true},
			want: "Tom &amp; Jerry",
		},
		{
			name: "移除控制字符",
			text: "Hello\vWorld",
			opts: &CleanOptions{
				RemoveControlChars: true,
				ControlCharRanges: [][2]int{
					{0, 8}, {11, 12}, {14, 31},
				},
			},
			want: "Hello World",
		},
		{
			name: "去除空白",
			text: "  Hello World  ",
			opts: &CleanOptions{TrimSpaces: true},
			want: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanText(tt.text, tt.opts)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrepareSSMLText(t *testing.T) {
	got := PrepareSSMLText("  Tom & Jerry\v\n")
	want := "Tom &amp; Jerry"

	if got != want {
		t.Fatalf("PrepareSSMLText() = %q, want %q", got, want)
	}

	if strings.ContainsAny(got, "\v\n") {
		t.Fatalf("expected control characters to be removed, got %q", got)
	}
}

func TestCleanTextDefaultControlRanges(t *testing.T) {
	dirty := "Hello\vWorld\x06"
	cleaned := CleanText(dirty, &CleanOptions{RemoveControlChars: true})

	if strings.ContainsAny(cleaned, "\v\x06") {
		t.Fatalf("expected control chars to be removed, got %q", cleaned)
	}
	if strings.Count(cleaned, " ") != 2 {
		t.Fatalf("expected replacement spaces for control chars, got %q", cleaned)
	}
}

func BenchmarkSplitByByteLength(b *testing.B) {
	text := strings.Repeat("这是一段测试文本。", 1000)
	opts := &SplitOptions{MaxBytes: 1024}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SplitByByteLength(text, opts)
	}
}

func TestSplitByByteLengthNewlineAndSpace(t *testing.T) {
	// 测试换行符优先于空格
	text := "Hello\nWorld This is a test"
	chunks := SplitByByteLength(text, &SplitOptions{MaxBytes: 10})

	// 第一个块应该在换行符处切割
	if len(chunks) > 0 && !strings.Contains(chunks[0], "Hello") {
		t.Errorf("expected first chunk to contain 'Hello', got %q", chunks[0])
	}
	if strings.Join(chunks, "") != strings.TrimSpace(text) {
		t.Errorf("joined chunks = %q, want %q", strings.Join(chunks, ""), strings.TrimSpace(text))
	}
}

func TestHTMLEntityProtection(t *testing.T) {
	// 测试 HTML 实体不被切断
	tests := []struct {
		name string
		text string
		max  int
	}{
		{
			name: "实体在切割点之前",
			text: "Tom &amp; Jerry is good",
			max:  15,
		},
		{
			name: "实体跨越切割点",
			text: "Hello &amp;test",
			max:  10, // 切割点在 &amp; 中间
		},
		{
			name: "多个实体",
			text: "&lt;tag&gt; &amp; more",
			max:  12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitByByteLength(tt.text, &SplitOptions{
				MaxBytes:             tt.max,
				PreserveHTMLEntities: true,
			})

			// 验证每个块都不包含不完整的实体
			for i, chunk := range chunks {
				// 如果块中有 &，必须有对应的 ;
				ampCount := strings.Count(chunk, "&")

				// 简单检查：每个 & 应该有对应的 ;
				if ampCount > 0 {
					// 验证没有不完整的实体
					lastAmp := strings.LastIndex(chunk, "&")
					if lastAmp >= 0 {
						afterAmp := chunk[lastAmp:]
						if !strings.Contains(afterAmp, ";") {
							t.Errorf("chunk %d has unterminated entity: %q", i, chunk)
						}
					}
				}
			}
		})
	}
}

func BenchmarkCleanText(b *testing.B) {
	text := strings.Repeat("测试文本 & <content>", 1000)
	opts := &CleanOptions{
		RemoveControlChars: true,
		ControlCharRanges: [][2]int{
			{0, 8}, {11, 12}, {14, 31},
		},
		EscapeHTML: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CleanText(text, opts)
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		text string
		opts *SplitOptions
	}{
		{
			name: "空文本",
			text: "",
			opts: &SplitOptions{MaxBytes: 100},
		},
		{
			name: "纯空白文本",
			text: "   \t\n   ",
			opts: &SplitOptions{MaxBytes: 100},
		},
		{
			name: "单个字符",
			text: "a",
			opts: &SplitOptions{MaxBytes: 1},
		},
		{
			name: "只有HTML实体",
			text: "&amp;&lt;&gt;&quot;",
			opts: &SplitOptions{MaxBytes: 10, PreserveHTMLEntities: true},
		},
		{
			name: "中文字符边界",
			text: "你好世界",
			opts: &SplitOptions{MaxBytes: 4}, // 恰好一个中文字符(3字节)+1字节
		},
		{
			name: "不完整的HTML实体在末尾",
			text: "Hello &amp",
			opts: &SplitOptions{MaxBytes: 11, PreserveHTMLEntities: true},
		},
		{
			name: "极长的单个词（无分隔符）",
			text: strings.Repeat("a", 1000),
			opts: &SplitOptions{MaxBytes: 100},
		},
		{
			name: "混合多字节字符",
			text: "Hello世界Test测试",
			opts: &SplitOptions{MaxBytes: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitByByteLength(tt.text, tt.opts)

			// 验证每个块的大小和 UTF-8 有效性
			for i, chunk := range chunks {
				if len(chunk) > tt.opts.MaxBytes {
					t.Errorf("chunk %d exceeds max bytes: %d > %d, chunk=%q", i, len(chunk), tt.opts.MaxBytes, chunk)
				}
				if !utf8.ValidString(chunk) {
					t.Errorf("chunk %d contains invalid UTF-8: %q", i, chunk)
				}
			}

			// 合并后应该精确还原原文本（仅忽略最外层空白）
			if len(chunks) > 0 {
				joined := strings.Join(chunks, "")
				original := strings.TrimSpace(tt.text)
				if joined != original {
					t.Errorf("joined chunks don't match original: got %q, want %q", joined, original)
				}
			}
		})
	}
}

func TestSplitByByteLength_PreservesBoundaryWhitespace(t *testing.T) {
	text := "Alpha beta\nGamma delta"
	chunks := SplitByByteLength(text, &SplitOptions{MaxBytes: 12})

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	if joined := strings.Join(chunks, ""); joined != strings.TrimSpace(text) {
		t.Fatalf("joined chunks = %q, want %q", joined, strings.TrimSpace(text))
	}
}

func TestSeparatorPriority(t *testing.T) {
	// 测试分隔符优先级：换行 > 空格 > 标点
	// 注意：edge-tts 只用换行和空格，我们添加标点作为更自然的备选
	tests := []struct {
		name     string
		text     string
		maxBytes int
		wantSep  string // 期望在此分隔符处切割
	}{
		{
			name:     "换行符优先于空格",
			text:     "Hello\nWorld This is a test",
			maxBytes: 15,
			wantSep:  "\n",
		},
		{
			name:     "空格优先于标点",
			text:     "HelloWorld。Test String",
			maxBytes: 20,
			wantSep:  " ",
		},
		{
			name:     "标点符号作为备选",
			text:     "你好世界。测试文本",
			maxBytes: 15,
			wantSep:  "。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitByByteLength(tt.text, &SplitOptions{MaxBytes: tt.maxBytes})
			if len(chunks) == 0 {
				t.Fatal("expected chunks")
			}

			// 检查第一个块是否包含期望的分隔符之前的部分
			firstChunk := chunks[0]
			if !strings.Contains(tt.text[:len(firstChunk)+5], tt.wantSep) {
				t.Errorf("expected split near %q, but got chunk: %q", tt.wantSep, firstChunk)
			}
		})
	}
}

func TestTextProcessingOrder(t *testing.T) {
	// 测试文本处理顺序：先清理控制字符，再转义HTML
	// 这个顺序很重要，参考 edge-tts
	text := "Tom & Jerry\vTest"

	// 使用 PrepareSSMLText（应该先清理控制字符，再转义）
	result := PrepareSSMLText(text)

	// 验证：
	// 1. 控制字符 \v 被替换为空格
	if strings.Contains(result, "\v") {
		t.Error("control character \\v should be removed")
	}

	// 2. HTML 字符 & 被转义为 &amp;
	if !strings.Contains(result, "&amp;") {
		t.Error("& should be escaped to &amp;")
	}

	// 3. 验证结果格式正确
	expected := "Tom &amp; Jerry Test"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestControlCharacterReplacement(t *testing.T) {
	// 测试控制字符被替换为空格，而不是直接删除
	// 这样可以保持词与词之间的分隔
	text := "Hello\vWorld\x06Test"
	cleaned := CleanText(text, &CleanOptions{RemoveControlChars: true})

	// 验证控制字符被替换为空格
	if !strings.Contains(cleaned, " ") {
		t.Error("control characters should be replaced with spaces")
	}

	// 验证没有控制字符残留
	for _, r := range cleaned {
		code := int(r)
		if (code >= 0 && code <= 8) || (code >= 11 && code <= 12) || (code >= 14 && code <= 31) {
			t.Errorf("found control character: %d", code)
		}
	}
}

func TestCompleteWorkflow(t *testing.T) {
	// 测试完整的文本处理工作流（模拟 edge-tts 的处理）
	// 输入：包含控制字符、HTML特殊字符、需要切割的长文本
	originalText := "Tom & Jerry\vtest. " + strings.Repeat("Hello world! ", 100)

	// 步骤1: 准备 SSML 文本（清理控制字符 + 转义 HTML）
	preparedText := PrepareSSMLText(originalText)

	// 验证：控制字符被移除
	if strings.Contains(preparedText, "\v") {
		t.Error("control characters should be removed")
	}

	// 验证：HTML 被转义
	if !strings.Contains(preparedText, "&amp;") {
		t.Error("HTML characters should be escaped")
	}

	// 步骤2: 切割长文本
	chunks := SplitByByteLength(preparedText, &SplitOptions{
		MaxBytes:             200,
		PreserveHTMLEntities: true,
	})

	// 验证：生成了多个块
	if len(chunks) <= 1 {
		t.Error("long text should be split into multiple chunks")
	}

	// 验证：每个块都不超过最大字节数
	for i, chunk := range chunks {
		if len(chunk) > 200 {
			t.Errorf("chunk %d exceeds max bytes: %d", i, len(chunk))
		}

		// 验证：每个块都是有效的 UTF-8
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d contains invalid UTF-8", i)
		}

		// 验证：HTML 实体没有被破坏
		if strings.Contains(chunk, "&") && !strings.Contains(chunk, "&amp;") {
			// 如果有 &，应该是完整的 &amp;
			t.Errorf("chunk %d may have broken HTML entities: %q", i, chunk)
		}
	}
}
