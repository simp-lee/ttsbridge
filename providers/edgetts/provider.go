package edgetts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/retry"
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
	// 可通过 provider.OutputOptions() 查看推荐格式目录；仅已声明且 probe 成功
	// 的格式会出现在 provider.SupportedFormats() 中。未知格式、未声明格式或
	// 已探测为不可用的格式会在本地发送前被拒绝。
	OutputFormat string

	BoundaryCallback func(event tts.BoundaryEvent)
	ProgressCallback func(completed, total int) // 合成进度回调：completed=已完成块数, total=总块数
}

// Provider Edge TTS provider
type Provider struct {
	client         *http.Client
	clientToken    string
	proxyURL       string
	proxyErr       error
	connectTimeout time.Duration
	maxAttempts    int
	receiveTimeout time.Duration
	voiceCache     *tts.VoiceCache
	formatRegistry *tts.FormatRegistry
	connectHook    func(context.Context) (*websocket.Conn, error)
}

var _ tts.Provider[*SynthesizeOptions] = (*Provider)(nil)

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
func (p *Provider) Close() error {
	if p.voiceCache != nil {
		return p.voiceCache.Stop()
	}
	return nil
}

// WithFormatRegistry adopts a clone of the provided registry and rebinds the
// prober to the current Provider. This keeps probe results/configuration
// isolated per Provider instance while preserving only the caller's declared
// formats and constant declarations, not transient runtime probe state.
func (p *Provider) WithFormatRegistry(r *tts.FormatRegistry) *Provider {
	if r == nil {
		r = defaultFormatRegistry.Clone()
	} else {
		r = r.CloneDeclaredClean()
	}
	r.SetProber(&edgeTTSProber{provider: p})
	p.formatRegistry = r
	return p
}

// FormatRegistry returns the provider's format registry.
func (p *Provider) FormatRegistry() *tts.FormatRegistry {
	if p.formatRegistry == nil {
		p.WithFormatRegistry(nil)
	}
	return p.formatRegistry
}

// SupportedFormats returns declared formats that have been explicitly verified
// as available in the current registry.
func (p *Provider) SupportedFormats() []tts.OutputFormat {
	registry := p.FormatRegistry()
	available := registry.Available()
	result := make([]tts.OutputFormat, 0, len(available))
	for _, format := range available {
		if registry.IsDeclared(format.ID) {
			result = append(result, format)
		}
	}
	return result
}

// WithProxy sets proxy URL
func (p *Provider) WithProxy(proxyURL string) *Provider {
	if proxyURL == "" {
		p.proxyURL = ""
		p.proxyErr = nil
		p.client.Transport = nil
		return p
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		p.proxyURL = ""
		p.proxyErr = &tts.Error{
			Code:     tts.ErrCodeInvalidInput,
			Message:  fmt.Sprintf("invalid proxy URL %q", proxyURL),
			Provider: providerName,
			Err:      err,
		}
		if err == nil {
			p.proxyErr = &tts.Error{
				Code:     tts.ErrCodeInvalidInput,
				Message:  fmt.Sprintf("invalid proxy URL %q", proxyURL),
				Provider: providerName,
				Err:      errors.New("proxy URL must include scheme and host"),
			}
		}
		p.client.Transport = nil
		return p
	}

	p.proxyURL = proxyURL
	p.proxyErr = nil
	p.client.Transport = nil
	p.client.Transport = &http.Transport{Proxy: http.ProxyURL(parsedURL)}
	return p
}

func (p *Provider) runtimeConfigError() error {
	if p.proxyErr != nil {
		return p.proxyErr
	}
	return nil
}

func (p *Provider) Name() string { return providerName }

type closeNower interface {
	CloseNow() error
}

func closeNowIgnoreError(c closeNower) {
	if c != nil {
		_ = c.CloseNow()
	}
}

func closeIgnoreError(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}

func normalizeEdgeTTSContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeBoundaryOptions(opts *SynthesizeOptions) *SynthesizeOptions {
	if opts.BoundaryCallback == nil || opts.WordBoundaryEnabled || opts.SentenceBoundaryEnabled {
		return opts
	}
	optsCopy := *opts
	optsCopy.WordBoundaryEnabled = true
	return &optsCopy
}

func splitPreparedText(opts *SynthesizeOptions) ([]string, error) {
	preparedText := textutils.PrepareSSMLText(opts.Text)
	if preparedText == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}
	maxTextBytes := defaultMaxSSMLBytes - ssmlWrapperBytes(opts)
	if maxTextBytes <= 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "ssml wrapper leaves no room for text content", Provider: providerName}
	}
	textChunks := textutils.SplitByByteLength(preparedText, &textutils.SplitOptions{
		MaxBytes:             maxTextBytes,
		PreserveHTMLEntities: true,
	})
	if len(textChunks) == 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after text normalization", Provider: providerName}
	}
	return textChunks, nil
}

func unwrapRetryCause(err error) error {
	if !retry.IsRetryError(err) {
		return err
	}
	retryErrors := retry.GetRetryErrors(err)
	for index := len(retryErrors) - 1; index >= 0; index-- {
		if retryErrors[index] != nil {
			return retryErrors[index]
		}
	}
	return err
}

func normalizeOutputFormatID(requested string) string {
	if requested == "" {
		return defaultOutputFormat
	}
	return requested
}

func (p *Provider) declaredOutputFormat(formatID string) (*tts.OutputFormat, bool) {
	registry := p.FormatRegistry()
	if !registry.IsDeclared(formatID) {
		return nil, false
	}
	return registry.Get(formatID)
}

func (p *Provider) resolveOutputFormat(requested string) (string, error) {
	return p.resolveOutputFormatWithMode(requested, false)
}

func (p *Provider) resolveProbeOutputFormat(requested string) (string, error) {
	return p.resolveOutputFormatWithMode(requested, true)
}

func (p *Provider) resolveOutputFormatWithMode(requested string, allowUndeclaredParsed bool) (string, error) {
	formatID := normalizeOutputFormatID(requested)
	declared, ok := p.declaredOutputFormat(formatID)
	if ok {
		if declared.Status == tts.FormatUnavailable && !p.FormatRegistry().IsProbeExpired(declared) {
			return "", &tts.Error{
				Code:     tts.ErrCodeUnsupportedFormat,
				Message:  fmt.Sprintf("output format %q is unavailable", formatID),
				Provider: providerName,
			}
		}
		return formatID, nil
	}

	if allowUndeclaredParsed {
		if _, parseOK := ParseOutputFormat(formatID); parseOK {
			return formatID, nil
		}
	}

	return "", &tts.Error{
		Code:     tts.ErrCodeUnsupportedFormat,
		Message:  fmt.Sprintf("output format %q is not declared by provider", formatID),
		Provider: providerName,
	}
}

func (p *Provider) synthesizeChunks(ctx context.Context, opts *SynthesizeOptions, textChunks []string, allowUndeclaredProbeFormat bool) ([]byte, error) {
	var allAudio []byte

	for i, chunk := range textChunks {
		chunkAudio, err := p.synthesizeChunk(ctx, opts, chunk, i, allowUndeclaredProbeFormat)
		if err != nil {
			return nil, err
		}
		allAudio = append(allAudio, chunkAudio...)
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(i+1, len(textChunks))
		}
	}
	return allAudio, nil
}

func (p *Provider) synthesize(ctx context.Context, opts *SynthesizeOptions, allowUndeclaredProbeFormat bool) ([]byte, error) {
	if opts == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "options cannot be nil", Provider: providerName}
	}
	if opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}
	if err := p.runtimeConfigError(); err != nil {
		return nil, err
	}
	opts = normalizeBoundaryOptions(opts)
	textChunks, err := splitPreparedText(opts)
	if err != nil {
		return nil, err
	}
	allAudio, err := p.synthesizeChunks(ctx, opts, textChunks, allowUndeclaredProbeFormat)
	if err != nil {
		return nil, err
	}
	return allAudio, nil
}

// Synthesize synthesizes text to speech
func (p *Provider) Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	ctx = normalizeEdgeTTSContext(ctx)
	return p.synthesize(ctx, opts, false)
}

// synthesizeChunk synthesizes a single text chunk.
// Boundary offsets are reported relative to this chunk.
func (p *Provider) synthesizeChunk(ctx context.Context, opts *SynthesizeOptions, text string, chunkIndex int, allowUndeclaredProbeFormat bool) ([]byte, error) {
	chunkOpts := *opts
	chunkOpts.Text = text

	var audio []byte
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		defer closeNowIgnoreError(conn)

		if err := p.sendConfig(ctx, conn, opts, allowUndeclaredProbeFormat); err != nil {
			return err
		}

		if err := p.sendSSML(ctx, conn, &chunkOpts); err != nil {
			return err
		}

		audio, err = p.receiveAudio(ctx, conn, opts, chunkIndex)
		return err
	}, tts.RetryOptions(ctx, p.maxAttempts)...)

	if err != nil {
		if retry.IsRetryError(err) {
			cause := unwrapRetryCause(err)
			var ttsErr *tts.Error
			if errors.As(cause, &ttsErr) {
				return nil, cause
			}
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

func (p *Provider) synthesizeForProbe(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	ctx = normalizeEdgeTTSContext(ctx)
	return p.synthesize(ctx, opts, true)
}

// SynthesizeStream returns a streaming audio reader
func (p *Provider) SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (tts.AudioStream, error) {
	ctx = normalizeEdgeTTSContext(ctx)
	if opts == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "options cannot be nil", Provider: providerName}
	}
	if opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}
	if err := p.runtimeConfigError(); err != nil {
		return nil, err
	}

	// Smart default: enable word boundary when BoundaryCallback is set but no boundary option enabled
	if opts.BoundaryCallback != nil && !opts.WordBoundaryEnabled && !opts.SentenceBoundaryEnabled {
		optsCopy := *opts
		optsCopy.WordBoundaryEnabled = true
		opts = &optsCopy
	}

	textChunks, err := splitPreparedText(opts)
	if err != nil {
		return nil, err
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
//	if err := health.Start(context.Background()); err != nil {
//	    return false
//	}
//	defer func() {
//	    if err := health.Stop(); err != nil {
//	        log.Printf("provider health stop: %v", err)
//	    }
//	}()
//	if health.IsHealthy() { /* provider is reachable */ }
func (p *Provider) IsAvailable(ctx context.Context) bool {
	ctx = normalizeEdgeTTSContext(ctx)
	if err := p.runtimeConfigError(); err != nil {
		return false
	}
	err := retry.Do(func() error {
		conn, err := p.connect(ctx)
		if err != nil {
			return err
		}
		closeNowIgnoreError(conn)
		return nil
	}, tts.RetryOptions(ctx, p.maxAttempts)...)
	return err == nil
}
