package edgetts

import (
	"testing"
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
