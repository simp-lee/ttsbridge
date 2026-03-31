package edgetts

import (
	"fmt"
	"html"
)

// buildSSML constructs SSML from options
func buildSSML(opts *synthesizeOptions) string {
	if opts == nil {
		opts = newSynthesizeOptions()
	}

	voice := opts.Voice
	if voice == "" {
		voice = defaultVoice
	}

	rate := formatProsody(opts.Rate)
	volume := formatProsody(opts.Volume)
	pitch := formatPitch(opts.Pitch)

	return fmt.Sprintf(
		`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'><voice name='%s'><prosody rate='%s' volume='%s' pitch='%s'>%s</prosody></voice></speak>`,
		html.EscapeString(voice), rate, volume, pitch, opts.Text,
	)
}

func ssmlWrapperBytes(opts *synthesizeOptions) int {
	if opts == nil {
		opts = newSynthesizeOptions()
	}
	optsCopy := *opts
	optsCopy.Text = ""
	return len([]byte(buildSSML(&optsCopy)))
}

// formatProsody formats rate/volume: 1.0 = +0%, 0.0 = -100%, 1.5 = +50%.
func formatProsody(value float64) string {
	if value == 1.0 {
		return "+0%"
	}
	percent := (value - 1.0) * 100
	if percent > 0 {
		return fmt.Sprintf("+%.0f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// formatPitch formats pitch: 1.0 = +0Hz, 0.0 = -100%, 1.5 = +50%.
func formatPitch(value float64) string {
	if value == 1.0 {
		return "+0Hz"
	}
	percent := (value - 1.0) * 100
	if percent > 0 {
		return fmt.Sprintf("+%.0f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// extractAudioData extracts audio from WebSocket binary message
func extractAudioData(message []byte) []byte {
	if len(message) < 2 {
		return nil
	}
	headerLen := int(message[0])<<8 | int(message[1])
	totalHeaderLen := 2 + headerLen
	if len(message) <= totalHeaderLen {
		return nil
	}
	return message[totalHeaderLen:]
}
