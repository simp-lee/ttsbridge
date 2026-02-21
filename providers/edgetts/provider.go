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

	// OutputFormat Edge TTS 输出格式字符串，如 "audio-48khz-192kbitrate-mono-mp3"。
	// 不设置时使用 Provider 默认格式（MP3 24kHz 48kbps）。
	// 可通过 provider.OutputOptions() 查看所有可用的输出格式。
	OutputFormat string

	BoundaryCallback func(event tts.BoundaryEvent)
	ProgressCallback func(completed, total int) // 合成进度回调：completed=已完成块数, total=总块数

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
	voiceCache     *tts.VoiceCache
	formatRegistry *tts.FormatRegistry
}

// New creates Edge TTS provider with default settings
// Use With* methods to customize configuration:
//
//	provider := edgetts.New().
//	    WithClientToken("your-token").
//	    WithTimeout(30*time.Second).
//	    WithMaxAttempts(5)
func New() *Provider {
	reg := defaultFormatRegistry.Clone()
	p := &Provider{
		clientToken:    defaultClientToken,
		connectTimeout: tts.DefaultConnectTimeout,
		maxAttempts:    tts.DefaultMaxRetries,
		receiveTimeout: tts.DefaultReceiveTimeout,
		client:         &http.Client{Timeout: tts.DefaultHTTPTimeout},
		formatRegistry: reg,
	}
	reg.SetProber(&edgeTTSProber{provider: p})
	ensureStableResolverRegistered()
	return p
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

// WithVoiceCache enables voice list caching. The cache fetches the full
// voice list once and filters by locale on each ListVoices call.
// Pass VoiceCacheOption values (e.g. tts.WithTTL, tts.WithBackgroundRefresh)
// to customise cache behaviour.
func (p *Provider) WithVoiceCache(opts ...tts.VoiceCacheOption) *Provider {
	fetcher := func(ctx context.Context) ([]tts.Voice, error) {
		entries, err := retry.DoWithResult(func() ([]voiceListEntry, error) {
			return fetchVoiceList(ctx, p.client, p.clientToken)
		}, tts.RetryOptions(ctx, p.maxAttempts)...)
		if err != nil {
			if retry.IsRetryError(err) {
				return nil, &tts.Error{
					Code:     tts.ErrCodeNetworkError,
					Message:  fmt.Sprintf("voice list retrieval failed after %d attempts", p.maxAttempts),
					Provider: providerName,
					Err:      err,
				}
			}
			return nil, err
		}
		return filterAndConvertVoices(entries, ""), nil
	}
	p.voiceCache = tts.NewVoiceCache(fetcher, opts...)
	return p
}

// Close releases resources held by the provider.
// If voice caching is enabled, its background goroutine (if any) is stopped.
// Close is safe to call multiple times.
func (p *Provider) Close() {
	if p.voiceCache != nil {
		p.voiceCache.Stop()
	}
}

// WithFormatRegistry replaces the default format registry with a custom one.
// Useful for testing or providers that need isolated format state.
func (p *Provider) WithFormatRegistry(r *tts.FormatRegistry) *Provider {
	if r == nil {
		r = defaultFormatRegistry.Clone()
	}
	p.formatRegistry = r
	if !r.HasProber() {
		r.SetProber(&edgeTTSProber{provider: p})
	}
	return p
}

// FormatRegistry returns the provider's format registry.
func (p *Provider) FormatRegistry() *tts.FormatRegistry {
	if p.formatRegistry == nil {
		p.WithFormatRegistry(nil)
	}
	return p.formatRegistry
}

// SupportedFormats returns all formats verified as available in the registry.
func (p *Provider) SupportedFormats() []tts.OutputFormat {
	return p.FormatRegistry().Available()
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
	if opts == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "options cannot be nil", Provider: providerName}
	}
	if opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}

	// Smart default: enable word boundary when BoundaryCallback is set but no boundary option enabled
	if opts.BoundaryCallback != nil && !opts.WordBoundaryEnabled && !opts.SentenceBoundaryEnabled {
		optsCopy := *opts
		optsCopy.WordBoundaryEnabled = true
		opts = &optsCopy
	}

	preparedText := textutils.PrepareSSMLText(opts.Text)
	if preparedText == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}
	textChunks := textutils.SplitByByteLength(preparedText, &textutils.SplitOptions{
		MaxBytes:             defaultMaxSSMLBytes,
		PreserveHTMLEntities: true,
	})
	if len(textChunks) == 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}

	var allAudio []byte
	var offsetCompensation int64

	for i, chunk := range textChunks {
		chunkAudio, err := p.synthesizeChunk(ctx, opts, chunk, offsetCompensation, i)
		if err != nil {
			return nil, err
		}
		allAudio = append(allAudio, chunkAudio...)
		offsetCompensation += defaultOffsetPadding
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(i+1, len(textChunks))
		}
	}

	// Mix with background music if configured
	if opts.BackgroundMusic != nil && opts.BackgroundMusic.MusicPath != "" {
		var profileOverride *tts.VoiceAudioProfile
		if opts.OutputFormat != "" {
			if parsed, ok := parseOutputFormat(opts.OutputFormat); ok {
				profileOverride = &parsed
			}
		}
		mixedAudio, err := audio.MixWithBackgroundMusic(ctx, allAudio, providerName, opts.Voice, opts.BackgroundMusic, profileOverride)
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
func (p *Provider) synthesizeChunk(ctx context.Context, opts *SynthesizeOptions, text string, offsetCompensation int64, chunkIndex int) ([]byte, error) {
	chunkOpts := *opts
	chunkOpts.Text = text

	var audio []byte
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		defer conn.CloseNow()

		if err := p.sendConfig(ctx, conn, opts); err != nil {
			return err
		}

		if err := p.sendSSML(ctx, conn, &chunkOpts); err != nil {
			return err
		}

		audio, err = p.receiveAudio(ctx, conn, opts, offsetCompensation, chunkIndex)
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
	if opts == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "options cannot be nil", Provider: providerName}
	}
	if opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}

	// Smart default: enable word boundary when BoundaryCallback is set but no boundary option enabled
	if opts.BoundaryCallback != nil && !opts.WordBoundaryEnabled && !opts.SentenceBoundaryEnabled {
		optsCopy := *opts
		optsCopy.WordBoundaryEnabled = true
		opts = &optsCopy
	}

	preparedText := textutils.PrepareSSMLText(opts.Text)
	if preparedText == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}
	textChunks := textutils.SplitByByteLength(preparedText, &textutils.SplitOptions{
		MaxBytes:             defaultMaxSSMLBytes,
		PreserveHTMLEntities: true,
	})
	if len(textChunks) == 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}

	return &edgeAudioStream{
		ctx:        ctx,
		opts:       opts,
		provider:   p,
		textChunks: textChunks,
		chunkIndex: 0,
		closed:     false,
	}, nil
}

// IsAvailable checks if the provider is available by attempting a WebSocket
// connection to the Edge TTS service.
//
// To monitor availability continuously, wrap IsAvailable with [tts.ProviderHealth]:
//
//	p := edgetts.New()
//	health := tts.NewProviderHealth(func(ctx context.Context) bool {
//	    return p.IsAvailable(ctx)
//	}, tts.WithCheckInterval(5*time.Minute))
//	health.Start(context.Background())
//	defer health.Stop()
//	if health.IsHealthy() { /* provider is reachable */ }
func (p *Provider) IsAvailable(ctx context.Context) bool {
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		conn.CloseNow()
		return nil
	}, tts.RetryOptions(ctx, p.maxAttempts)...)
	return err == nil
}
