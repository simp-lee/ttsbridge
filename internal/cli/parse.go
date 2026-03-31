package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// percentPattern matches strings like "+50%", "-20%", "+0%"
var percentPattern = regexp.MustCompile(`^([+-])?(\d+(?:\.\d+)?)%$`)

// ParsePercent parses a percentage string like "+50%", "-20%", "+0%" to a float64 multiplier.
// Examples:
//   - "+0%" -> 1.0
//   - "+100%" -> 2.0
//   - "-50%" -> 0.5
//   - "+50%" -> 1.5
//   - "-20%" -> 0.8
func ParsePercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1.0, nil
	}

	matches := percentPattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid percentage format: %q (expected format like +50%% or -20%%)", s)
	}

	sign := matches[1]
	valueStr := matches[2]

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid percentage value: %q", valueStr)
	}

	// Convert percentage to multiplier
	// +0% = 1.0, +100% = 2.0, -50% = 0.5
	multiplier := 1.0 + (value / 100.0)
	if sign == "-" {
		multiplier = 1.0 - (value / 100.0)
	}

	// Clamp to valid range
	if multiplier < 0 {
		multiplier = 0
	}

	return multiplier, nil
}

// ParseRatePercent parses rate percentage and returns a value in range [0.5, 2.0]
func ParseRatePercent(s string) (float64, error) {
	value, err := ParsePercent(s)
	if err != nil {
		return 0, err
	}

	// Clamp to valid rate range
	if value < 0.5 {
		value = 0.5
	}
	if value > 2.0 {
		value = 2.0
	}

	return value, nil
}

// ParseVolumePercent parses volume percentage and returns a multiplier in range [0.0, 2.0].
// Note: 0.0 is a valid explicit mute value and must not be rewritten to provider default.
func ParseVolumePercent(s string) (float64, error) {
	value, err := ParsePercent(s)
	if err != nil {
		return 0, err
	}

	// Clamp to valid volume range
	if value < 0 {
		value = 0
	}
	if value > 2.0 {
		value = 2.0
	}

	return value, nil
}

// ParsePitchPercent parses pitch percentage and returns a value in range [0.5, 2.0]
func ParsePitchPercent(s string) (float64, error) {
	value, err := ParsePercent(s)
	if err != nil {
		return 0, err
	}

	// Clamp to valid pitch range
	if value < 0.5 {
		value = 0.5
	}
	if value > 2.0 {
		value = 2.0
	}

	return value, nil
}
