// Package textutils 提供文本处理工具，供 TTS Provider 使用
package textutils

import (
	"bytes"
	"html"
	"strings"
	"unicode/utf8"
)

// SplitOptions 文本切割选项
type SplitOptions struct {
	// MaxBytes 每个块的最大字节数
	MaxBytes int
	// PreserveHTMLEntities 是否保护 HTML 实体（SSML 格式需要）
	PreserveHTMLEntities bool
	// Separators 切割分隔符优先级（可选，为空时使用默认值）
	Separators []string
}

// 分隔符优先级（从高到低）
// edge-tts 只使用换行和空格，但从实际 TTS 应用来看，在标点处切割更自然
// 因此我们添加标点作为低优先级的备选分隔符，优先级：换行 > 空格 > 标点
var defaultSeparators = []string{
	"\n",                                             // 最高优先级：换行符
	" ",                                              // 次优先级：空格
	"。", "！", "？", ".", "!", "?", "；", ";", "，", ",", // 标点符号（低优先级备选）
}

var defaultControlCharRanges = [][2]int{
	{0, 8},
	{11, 12},
	{14, 31},
}

// SplitByByteLength 按字节长度智能切割文本
//
// 该函数按照以下策略切割文本，确保：
// 1. 每个块不超过 MaxBytes 字节
// 2. 优先在自然边界切割（换行符 > 空格 > 标点符号）
// 3. 不破坏 UTF-8 多字节字符
// 4. 不破坏 HTML/XML 实体（如 &amp;）当 PreserveHTMLEntities 为 true 时
// 5. 自动去除每个块首尾的空白字符
//
// 参数：
//   - text: 待切割的文本
//   - opts: 切割选项，包括最大字节数、是否保护 HTML 实体、自定义分隔符等
//
// 返回值：
//   - []string: 切割后的文本块数组，如果输入为空或仅包含空白，返回 nil 或空数组
func SplitByByteLength(text string, opts *SplitOptions) []string {
	if opts == nil {
		opts = &SplitOptions{}
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	if opts.MaxBytes <= 0 {
		return []string{trimmed}
	}

	data := []byte(trimmed)
	if len(data) <= opts.MaxBytes {
		return []string{trimmed}
	}

	separatorBytes := prepareSeparatorBytes(opts.Separators)

	var chunks []string

	for len(data) > opts.MaxBytes {
		splitAt := locateSplitPoint(data, opts.MaxBytes, separatorBytes, opts.PreserveHTMLEntities)

		// 防止无限循环：如果切割点无效，至少前进一个字节
		// 参考 edge-tts: text = text[split_at if split_at > 0 else 1:]
		if splitAt <= 0 {
			splitAt = 1
		}

		if chunk := bytes.TrimSpace(data[:splitAt]); len(chunk) > 0 {
			chunks = append(chunks, string(chunk))
		}

		data = data[splitAt:]
	}

	if remaining := bytes.TrimSpace(data); len(remaining) > 0 {
		chunks = append(chunks, string(remaining))
	}

	return chunks
}

// CleanOptions 文本清理选项
type CleanOptions struct {
	// RemoveControlChars 是否移除控制字符
	RemoveControlChars bool
	// ControlCharRanges 控制字符范围 [[start, end], ...]
	ControlCharRanges [][2]int
	// EscapeHTML 是否转义 HTML/XML 特殊字符
	EscapeHTML bool
	// TrimSpaces 是否去除首尾空白
	TrimSpaces bool
}

// PrepareSSMLText 使用默认清理策略为 SSML 准备文本
// 1. 移除常见控制字符
// 2. 转义 HTML/XML 特殊字符
// 3. 去除首尾空白
func PrepareSSMLText(text string) string {
	return CleanText(text, &CleanOptions{
		RemoveControlChars: true,
		EscapeHTML:         true,
		TrimSpaces:         true,
	})
}

// CleanText 清理文本，为 TTS 合成做准备
//
// 该函数可以执行以下清理操作：
// 1. 移除或替换控制字符（如垂直制表符、换页符等）
// 2. 转义 HTML/XML 特殊字符（&, <, >, ", '）
// 3. 去除首尾空白字符
//
// 参数：
//   - text: 待清理的文本
//   - opts: 清理选项，如果为 nil 则不做任何处理
//
// 返回值：
//   - string: 清理后的文本
func CleanText(text string, opts *CleanOptions) string {
	if opts == nil {
		return text
	}

	result := text

	if opts.RemoveControlChars {
		ranges := opts.ControlCharRanges
		if len(ranges) == 0 {
			ranges = defaultControlCharRanges
		}
		result = removeControlChars(result, ranges)
	}

	if opts.EscapeHTML {
		result = html.EscapeString(result)
	}

	if opts.TrimSpaces {
		result = strings.TrimSpace(result)
	}

	return result
}

// removeControlChars 移除指定范围的控制字符
func removeControlChars(text string, ranges [][2]int) string {
	var builder strings.Builder
	builder.Grow(len(text))

	for _, r := range text {
		// 检查字符是否在控制字符范围内
		isControl := false
		for _, rng := range ranges {
			if int(r) >= rng[0] && int(r) <= rng[1] {
				isControl = true
				break
			}
		}

		if isControl {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func prepareSeparatorBytes(custom []string) [][]byte {
	separators := custom
	if len(separators) == 0 {
		separators = defaultSeparators
	}

	result := make([][]byte, 0, len(separators))
	for _, sep := range separators {
		if sep != "" {
			result = append(result, []byte(sep))
		}
	}

	return result
}

func locateSplitPoint(data []byte, maxBytes int, separators [][]byte, preserveHTML bool) int {
	limit := min(maxBytes, len(data))

	// 1. 尝试在分隔符处切割（按优先级查找）
	splitAt := lastSeparatorIndex(data, limit, separators)

	// 2. 如果找不到分隔符，寻找安全的 UTF-8 边界
	if splitAt < 0 {
		splitAt = findSafeUTF8SplitPoint(data[:limit])
	}

	// 3. 调整切割点以避免破坏 HTML 实体
	if preserveHTML && splitAt > 0 {
		splitAt = adjustForHTMLEntity(data, splitAt)
	}

	return splitAt
}

func lastSeparatorIndex(data []byte, limit int, separators [][]byte) int {
	segment := data[:limit]

	// 按优先级逐个查找分隔符（换行 > 空格 > 标点符号）
	// 注意：edge-tts 只用换行和空格，我们额外添加标点作为更优雅的切割点
	for _, sep := range separators {
		idx := bytes.LastIndex(segment, sep)
		if idx >= 0 {
			// 返回分隔符之后的位置
			return idx + len(sep)
		}
	}

	return -1
}

// findSafeUTF8SplitPoint 在给定的数据段中找到最后一个完整的 UTF-8 字符之后的位置
// 这避免了在多字节字符中间切割
// 参考 edge-tts 的 _find_safe_utf8_split_point 实现
func findSafeUTF8SplitPoint(data []byte) int {
	splitAt := len(data)

	// 从后往前逐字节尝试，直到找到有效的 UTF-8 序列
	for splitAt > 0 {
		if utf8.Valid(data[:splitAt]) {
			return splitAt
		}
		splitAt--
	}

	return 0
}

// adjustForHTMLEntity 调整切割点以避免破坏 HTML/XML 实体
// 如果切割点落在未终止的实体中（&...），则将切割点移到 & 之前
// 参考 edge-tts 的 _adjust_split_point_for_xml_entity 实现
func adjustForHTMLEntity(text []byte, splitAt int) int {
	if splitAt <= 0 || splitAt > len(text) {
		return splitAt
	}

	// 循环检查是否有未终止的实体
	for splitAt > 0 && bytes.Contains(text[:splitAt], []byte("&")) {
		// 查找最后一个 & 符号
		ampIdx := bytes.LastIndexByte(text[:splitAt], '&')
		if ampIdx < 0 {
			break
		}

		// 检查 & 和 splitAt 之间是否有分号
		if bytes.IndexByte(text[ampIdx:splitAt], ';') != -1 {
			// 找到了完整的实体（如 &amp;），原切割点是安全的
			break
		}

		// & 后面没有分号，实体未终止，移动切割点到 & 之前
		splitAt = ampIdx
	}

	return splitAt
}
