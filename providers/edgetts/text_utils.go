package edgetts

import (
	"bytes"
	"html"
	"unicode/utf8"
)

const (
	// Maximum SSML text size in bytes (as per Microsoft Edge TTS limits)
	maxSSMLBytes = 4096
)

// SplitTextByByteLength splits text into chunks not exceeding maxBytes.
//
// This function prioritizes splitting at natural boundaries (newlines, spaces)
// while ensuring that:
// 1. No chunk exceeds maxBytes bytes
// 2. Chunks do not end with an incomplete UTF-8 multi-byte character
// 3. Chunks do not split in the middle of HTML entities (like &amp;)
//
// This implementation is based on edge-tts project:
// https://github.com/rany2/edge-tts/blob/master/src/edge_tts/communicate.py
func SplitTextByByteLength(text string, maxBytes int) []string {
	if maxBytes <= 0 {
		maxBytes = maxSSMLBytes
	}

	textBytes := []byte(text)
	if len(textBytes) <= maxBytes {
		return []string{text}
	}

	var chunks []string

	for len(textBytes) > maxBytes {
		splitAt := findSplitPoint(textBytes, maxBytes)

		if splitAt <= 0 {
			// Fallback: force split at maxBytes
			splitAt = maxBytes
		}

		chunk := bytes.TrimSpace(textBytes[:splitAt])
		if len(chunk) > 0 {
			chunks = append(chunks, string(chunk))
		}

		// Move to next segment
		if splitAt > 0 {
			textBytes = textBytes[splitAt:]
		} else {
			textBytes = textBytes[1:] // Avoid infinite loop
		}
	}

	// Add remaining text
	if remaining := bytes.TrimSpace(textBytes); len(remaining) > 0 {
		chunks = append(chunks, string(remaining))
	}

	return chunks
}

// findSplitPoint finds the best split point within the limit.
//
// Priority order:
// 1. Newline character
// 2. Space character
// 3. Safe UTF-8 boundary
func findSplitPoint(text []byte, limit int) int {
	if limit > len(text) {
		limit = len(text)
	}

	// 1. Try to find last newline within limit
	if idx := bytes.LastIndexByte(text[:limit], '\n'); idx >= 0 {
		return idx + 1
	}

	// 2. Try to find last space within limit
	if idx := bytes.LastIndexByte(text[:limit], ' '); idx >= 0 {
		splitAt := idx + 1
		// Adjust for HTML entities
		return adjustForHTMLEntity(text, splitAt)
	}

	// 3. Find safe UTF-8 split point
	splitAt := findSafeUTF8SplitPoint(text[:limit])
	return adjustForHTMLEntity(text, splitAt)
}

// findSafeUTF8SplitPoint finds the rightmost byte index where text[:index] is valid UTF-8.
//
// This prevents splitting in the middle of a multi-byte UTF-8 character.
func findSafeUTF8SplitPoint(text []byte) int {
	for i := len(text); i > 0; i-- {
		if utf8.Valid(text[:i]) {
			return i
		}
	}
	return 0
}

// adjustForHTMLEntity adjusts split point to avoid breaking HTML entities.
//
// For example, if text is "this &amp; that" and splitAt falls between
// & and ;, this moves splitAt to before the &.
func adjustForHTMLEntity(text []byte, splitAt int) int {
	if splitAt <= 0 || splitAt > len(text) {
		return splitAt
	}

	// Look for unfinished HTML entity before split point
	for splitAt > 0 {
		ampIdx := bytes.LastIndexByte(text[:splitAt], '&')
		if ampIdx < 0 {
			break
		}

		// Check if there's a semicolon between ampersand and split point
		if bytes.IndexByte(text[ampIdx:splitAt], ';') >= 0 {
			// Entity is complete, safe to split
			break
		}

		// Entity is incomplete, move split point before the ampersand
		splitAt = ampIdx
	}

	return splitAt
}

// RemoveIncompatibleCharacters removes characters that Edge TTS doesn't support.
//
// The service does not support certain control character ranges.
// Most importantly, the vertical tab character (code 11) which is
// commonly present in OCR-ed PDFs.
//
// Character codes removed:
// - 0-8: Control characters
// - 11-12: Vertical tab and form feed
// - 14-31: Other control characters
func RemoveIncompatibleCharacters(text string) string {
	var result []rune
	for _, r := range text {
		code := int(r)
		if (0 <= code && code <= 8) ||
			(11 <= code && code <= 12) ||
			(14 <= code && code <= 31) {
			result = append(result, ' ')
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// EscapeSSMLText escapes text for use in SSML.
//
// This ensures that special XML characters are properly escaped.
func EscapeSSMLText(text string) string {
	return html.EscapeString(text)
}

// PrepareTextForSSML prepares text for SSML by cleaning and escaping it.
func PrepareTextForSSML(text string) string {
	// Remove incompatible characters
	cleaned := RemoveIncompatibleCharacters(text)
	// Escape for XML/SSML
	return EscapeSSMLText(cleaned)
}
