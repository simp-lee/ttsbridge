package cli

import "testing"

func TestParsePercent(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"+0%", 1.0, false},
		{"+100%", 2.0, false},
		{"-50%", 0.5, false},
		{"+50%", 1.5, false},
		{"-20%", 0.8, false},
		{"+25%", 1.25, false},
		{"-100%", 0.0, false},
		{"0%", 1.0, false},
		{"-0%", 1.0, false},
		{"+0.0%", 1.0, false},
		{"50%", 1.5, false},
		{"", 1.0, false},
		{"+0.5%", 1.005, false},
		{"-0.5%", 0.995, false},

		// Invalid formats
		{"50", 0, true},
		{"abc", 0, true},
		{"+50Hz", 0, true},
		{"50%%", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePercent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePercent(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !floatEquals(got, tt.want) {
				t.Errorf("ParsePercent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRatePercent(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"+0%", 1.0},
		{"+100%", 2.0}, // Clamped to max 2.0
		{"-50%", 0.5},
		{"-80%", 0.5}, // Clamped to min 0.5
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRatePercent(tt.input)
			if err != nil {
				t.Errorf("ParseRatePercent(%q) error = %v", tt.input, err)
				return
			}
			if !floatEquals(got, tt.want) {
				t.Errorf("ParseRatePercent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
