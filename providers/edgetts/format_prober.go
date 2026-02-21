package edgetts

import "context"

// edgeTTSProber implements tts.FormatProber by synthesizing a very short text
// snippet with the specified output format and checking whether the service
// returns any audio data.
type edgeTTSProber struct {
	provider *Provider
}

// ProbeFormat synthesizes the word "test" using the given format and reports
// whether the service returned valid audio data (len > 0).
func (p *edgeTTSProber) ProbeFormat(ctx context.Context, formatID string) (bool, error) {
	data, err := p.provider.Synthesize(ctx, &SynthesizeOptions{
		Text:         "test",
		Voice:        defaultVoice,
		OutputFormat: formatID,
	})
	if err != nil {
		return false, err
	}
	return len(data) > 0, nil
}
