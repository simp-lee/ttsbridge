package edgetts

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/simp-lee/retry"
	"github.com/simp-lee/ttsbridge/audio"
	"github.com/simp-lee/ttsbridge/tts"
	"github.com/simp-lee/ttsbridge/tts/textutils"
)

// SynthesizeOptions EdgeTTS synthesis options
type SynthesizeOptions struct {
	Text  string
	Voice string

	Rate   float64 // Speech rate, 0.5-2.0, default 1.0
	Volume float64 // Volume, 0.0-1.0, default 1.0
	Pitch  float64 // Pitch, 0.5-2.0, default 1.0

	WordBoundaryEnabled     bool
	SentenceBoundaryEnabled bool
	MetadataCallback        func(metadataType string, offset int64, duration int64, text string)

	BackgroundMusic *tts.BackgroundMusicOptions
}

// Provider Edge TTS provider
type Provider struct {
	client         *http.Client
	clientToken    string
	proxyURL       string
	connectTimeout time.Duration
	maxAttempts    int
	receiveTimeout time.Duration
}

// New creates Edge TTS provider with default settings
// Use With* methods to customize configuration:
//
//	provider := edgetts.New().
//	    WithClientToken("your-token").
//	    WithTimeout(30*time.Second).
//	    WithMaxAttempts(5)
func New() *Provider {
	return &Provider{
		clientToken:    defaultClientToken,
		connectTimeout: tts.DefaultConnectTimeout,
		maxAttempts:    tts.DefaultMaxRetries,
		receiveTimeout: tts.DefaultReceiveTimeout,
		client:         &http.Client{Timeout: tts.DefaultHTTPTimeout},
	}
}

// WithClientToken sets custom client token
func (p *Provider) WithClientToken(token string) *Provider {
	p.clientToken = token
	return p
}

// WithHTTPTimeout sets HTTP client timeout
func (p *Provider) WithHTTPTimeout(timeout time.Duration) *Provider {
	p.client.Timeout = timeout
	return p
}

// WithConnectTimeout sets WebSocket connection timeout
func (p *Provider) WithConnectTimeout(timeout time.Duration) *Provider {
	p.connectTimeout = timeout
	return p
}

// WithReceiveTimeout sets WebSocket receive timeout
func (p *Provider) WithReceiveTimeout(timeout time.Duration) *Provider {
	p.receiveTimeout = timeout
	return p
}

// WithMaxAttempts sets maximum retry attempts (including first try)
func (p *Provider) WithMaxAttempts(attempts int) *Provider {
	if attempts > 0 {
		p.maxAttempts = attempts
	}
	return p
}

// WithProxy sets proxy URL
func (p *Provider) WithProxy(proxyURL string) *Provider {
	p.proxyURL = proxyURL
	if proxyURL != "" {
		if parsedURL, _ := url.Parse(proxyURL); parsedURL != nil {
			p.client.Transport = &http.Transport{Proxy: http.ProxyURL(parsedURL)}
		}
	}
	return p
}

func (p *Provider) Name() string { return providerName }

// Synthesize synthesizes text to speech
func (p *Provider) Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	preparedText := textutils.PrepareSSMLText(opts.Text)
	textChunks := textutils.SplitByByteLength(preparedText, &textutils.SplitOptions{
		MaxBytes:             defaultMaxSSMLBytes,
		PreserveHTMLEntities: true,
	})

	var allAudio []byte
	var offsetCompensation int64

	for _, chunk := range textChunks {
		chunkAudio, err := p.synthesizeChunk(ctx, opts, chunk, offsetCompensation)
		if err != nil {
			return nil, err
		}
		allAudio = append(allAudio, chunkAudio...)
		offsetCompensation += defaultOffsetPadding
	}

	// Mix with background music if configured
	if opts.BackgroundMusic != nil && opts.BackgroundMusic.MusicPath != "" {
		mixedAudio, err := audio.MixWithBackgroundMusic(ctx, allAudio, providerName, opts.Voice, opts.BackgroundMusic)
		if err != nil {
			return nil, &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "background music mixing failed",
				Provider: providerName,
				Err:      err,
			}
		}
		return mixedAudio, nil
	}

	return allAudio, nil
}

// synthesizeChunk synthesizes a single text chunk
func (p *Provider) synthesizeChunk(ctx context.Context, opts *SynthesizeOptions, text string, offsetCompensation int64) ([]byte, error) {
	chunkOpts := *opts
	chunkOpts.Text = text

	var audio []byte
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()

		if err := p.sendConfig(ctx, conn, opts); err != nil {
			return err
		}

		if err := p.sendSSML(ctx, conn, &chunkOpts); err != nil {
			return err
		}

		audio, err = p.receiveAudio(ctx, conn, opts, offsetCompensation)
		return err
	}, tts.RetryOptions(ctx, p.maxAttempts)...)

	if err != nil {
		if retry.IsRetryError(err) {
			return nil, &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  fmt.Sprintf("synthesis failed after %d attempts", p.maxAttempts),
				Provider: providerName,
				Err:      err,
			}
		}
		return nil, err
	}

	return audio, nil
}

// SynthesizeStream returns a streaming audio reader
func (p *Provider) SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (tts.AudioStream, error) {
	preparedText := textutils.PrepareSSMLText(opts.Text)
	textChunks := textutils.SplitByByteLength(preparedText, &textutils.SplitOptions{
		MaxBytes:             defaultMaxSSMLBytes,
		PreserveHTMLEntities: true,
	})

	return &edgeAudioStream{
		ctx:        ctx,
		opts:       opts,
		provider:   p,
		textChunks: textChunks,
		chunkIndex: 0,
		closed:     false,
	}, nil
}

// IsAvailable checks if the provider is available
func (p *Provider) IsAvailable(ctx context.Context) bool {
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}, tts.RetryOptions(ctx, p.maxAttempts)...)
	return err == nil
}
