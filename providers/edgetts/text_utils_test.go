package edgetts

import (
	"strings"
	"testing"
)

func TestSplitTextByByteLength(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		maxBytes   int
		wantChunks int
	}{
		{
			name:       "短文本",
			text:       "Hello, world!",
			maxBytes:   100,
			wantChunks: 1,
		},
		{
			name:       "需要分割的长文本",
			text:       strings.Repeat("Hello world. ", 500),
			maxBytes:   100,
			wantChunks: 65, // 约 65 个块
		},
		{
			name:       "中文文本",
			text:       strings.Repeat("你好世界。", 200),
			maxBytes:   100,
			wantChunks: 30, // 约 30 个块
		},
		{
			name:       "混合文本",
			text:       "Hello 世界 " + strings.Repeat("test ", 100),
			maxBytes:   50,
			wantChunks: 11, // 约 11 个块
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitTextByByteLength(tt.text, tt.maxBytes)

			if len(chunks) < 1 {
				t.Error("Should produce at least one chunk")
			}

			// 验证每个块的大小
			for i, chunk := range chunks {
				if len(chunk) > tt.maxBytes {
					t.Errorf("Chunk %d exceeds max bytes: %d > %d", i, len(chunk), tt.maxBytes)
				}

				// 验证 UTF-8 有效性
				if !isValidUTF8([]byte(chunk)) {
					t.Errorf("Chunk %d contains invalid UTF-8", i)
				}
			}

			// 验证重新合并后的文本
			rejoined := strings.Join(chunks, "")
			original := strings.ReplaceAll(tt.text, " ", "")
			rejoined = strings.ReplaceAll(rejoined, " ", "")

			if len(rejoined) < len(original)*9/10 { // 允许 10% 的差异（空格修剪）
				t.Errorf("Lost too much content after splitting")
			}
		})
	}
}

func TestRemoveIncompatibleCharacters(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "无特殊字符",
			input: "Hello, world!",
			want:  "Hello, world!",
		},
		{
			name:  "包含垂直制表符",
			input: "Hello\vWorld",
			want:  "Hello World",
		},
		{
			name:  "包含多种控制字符",
			input: "Test\x00\x08\x0b\x0c\x1fEnd",
			want:  "Test     End",
		},
		{
			name:  "正常换行和制表符",
			input: "Line1\nLine2\tTab",
			want:  "Line1\nLine2\tTab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveIncompatibleCharacters(tt.input)
			if got != tt.want {
				t.Errorf("RemoveIncompatibleCharacters() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapeSSMLText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "无需转义",
			input: "Hello, world!",
			want:  "Hello, world!",
		},
		{
			name:  "HTML 实体",
			input: "Tom & Jerry",
			want:  "Tom &amp; Jerry",
		},
		{
			name:  "XML 特殊字符",
			input: "<tag>content</tag>",
			want:  "&lt;tag&gt;content&lt;/tag&gt;",
		},
		{
			name:  "引号",
			input: `He said "hello"`,
			want:  `He said &#34;hello&#34;`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeSSMLText(tt.input)
			if got != tt.want {
				t.Errorf("EscapeSSMLText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdjustForHTMLEntity(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		splitAt int
		want    int
	}{
		{
			name:    "完整实体",
			text:    "Hello &amp; world",
			splitAt: 15,
			want:    15,
		},
		{
			name:    "不完整实体",
			text:    "Hello &amp; world",
			splitAt: 10, // 在 &amp 中间
			want:    6,  // 移到 & 之前
		},
		{
			name:    "无实体",
			text:    "Hello world",
			splitAt: 8,
			want:    8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustForHTMLEntity([]byte(tt.text), tt.splitAt)
			if got != tt.want {
				t.Errorf("adjustForHTMLEntity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func isValidUTF8(data []byte) bool {
	for len(data) > 0 {
		r, size := decodeRune(data)
		if r == -1 {
			return false
		}
		data = data[size:]
	}
	return true
}

func decodeRune(data []byte) (rune, int) {
	if len(data) == 0 {
		return -1, 0
	}

	// Simple UTF-8 validation
	b := data[0]
	if b < 0x80 {
		return rune(b), 1
	}

	if b < 0xC0 {
		return -1, 0
	}

	if b < 0xE0 {
		if len(data) < 2 {
			return -1, 0
		}
		return rune(b), 2
	}

	if b < 0xF0 {
		if len(data) < 3 {
			return -1, 0
		}
		return rune(b), 3
	}

	if len(data) < 4 {
		return -1, 0
	}
	return rune(b), 4
}
