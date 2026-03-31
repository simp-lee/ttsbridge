package edgetts

import (
	"context"
	"errors"

	"github.com/simp-lee/ttsbridge/tts"
)

// edgeTTSProber implements tts.FormatProber by synthesizing a very short text
// snippet with the specified output format and checking whether the service
// returns any audio data.
type edgeTTSProber struct {
	provider *Provider
}

var _ tts.FormatProber = (*edgeTTSProber)(nil)

// ProbeFormat synthesizes the word "test" using the given format and reports
// whether the service returned valid audio data (len > 0).
func (p *edgeTTSProber) ProbeFormat(ctx context.Context, formatID string) (bool, error) {
	options := newSynthesizeOptions()
	options.Text = "test"
	options.Voice = defaultVoice
	options.OutputFormat = formatID

	data, err := p.provider.synthesizeForProbe(ctx, options)
	if err != nil {
		var ttsErr *tts.Error
		if errors.As(err, &ttsErr) && ttsErr.Code == tts.ErrCodeUnsupportedFormat {
			return false, nil
		}
		return false, err
	}
	return len(data) > 0, nil
}
