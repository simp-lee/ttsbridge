package edgetts

import (
	"testing"
)

// TestFormatPitch 测试音调格式化函数
func TestFormatPitch(t *testing.T) {
	tests := []struct {
		name     string
		pitch    float64
		expected string
	}{
		{
			name:     "默认音调",
			pitch:    1.0,
			expected: "+0%",
		},
		{
			name:     "提高音调 50%",
			pitch:    1.5,
			expected: "+50%",
		},
		{
			name:     "提高音调 100%（翻倍）",
			pitch:    2.0,
			expected: "+100%",
		},
		{
			name:     "降低音调 50%",
			pitch:    0.5,
			expected: "-50%",
		},
		{
			name:     "降低音调 20%",
			pitch:    0.8,
			expected: "-20%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPitch(tt.pitch)
			if result != tt.expected {
				t.Errorf("formatPitch(%v) = %v, expected %v", tt.pitch, result, tt.expected)
			}
		})
	}
}

// TestFormatRate 测试语速格式化函数
func TestFormatRate(t *testing.T) {
	tests := []struct {
		name     string
		rate     float64
		expected string
	}{
		{
			name:     "默认语速",
			rate:     1.0,
			expected: "+0%",
		},
		{
			name:     "加快 50%",
			rate:     1.5,
			expected: "+50%",
		},
		{
			name:     "加快 100%",
			rate:     2.0,
			expected: "+100%",
		},
		{
			name:     "减慢 50%",
			rate:     0.5,
			expected: "-50%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRate(tt.rate)
			if result != tt.expected {
				t.Errorf("formatRate(%v) = %v, expected %v", tt.rate, result, tt.expected)
			}
		})
	}
}

// TestFormatVolume 测试音量格式化函数
func TestFormatVolume(t *testing.T) {
	tests := []struct {
		name     string
		volume   float64
		expected string
	}{
		{
			name:     "默认音量",
			volume:   1.0,
			expected: "+0%",
		},
		{
			name:     "提高 50%",
			volume:   1.5,
			expected: "+50%",
		},
		{
			name:     "降低 20%",
			volume:   0.8,
			expected: "-20%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatVolume(tt.volume)
			if result != tt.expected {
				t.Errorf("formatVolume(%v) = %v, expected %v", tt.volume, result, tt.expected)
			}
		})
	}
}
