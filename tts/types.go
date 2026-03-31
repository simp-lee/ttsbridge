package tts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/simp-lee/retry"
)

// Default retry and timeout configuration
// These values can be used by all TTS providers as starting defaults
const (
	// DefaultMaxRetries is the default maximum number of retry attempts
	DefaultMaxRetries = 3

	// DefaultBackoffInitial is the default initial backoff duration for exponential retry
	DefaultBackoffInitial = 100 * time.Millisecond

	// DefaultBackoffMax is the default maximum backoff duration for exponential retry
	DefaultBackoffMax = 5 * time.Second

	// DefaultBackoffJitter is the default jitter duration for exponential retry
	DefaultBackoffJitter = 1 * time.Second

	// DefaultConnectTimeout is the default timeout for establishing connections
	DefaultConnectTimeout = 10 * time.Second

	// DefaultReceiveTimeout is the default timeout for receiving data
	DefaultReceiveTimeout = 60 * time.Second

	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 30 * time.Second
)

// Language 语言代码类型
type Language string

// 常见语言代码常量
const (
	LanguageZhCN Language = "zh-CN" // 简体中文
	LanguageZhTW Language = "zh-TW" // 繁体中文
	LanguageEnUS Language = "en-US" // 美式英语
	LanguageEnGB Language = "en-GB" // 英式英语
	LanguageJaJP Language = "ja-JP" // 日语
	LanguageKoKR Language = "ko-KR" // 韩语
	LanguageFrFR Language = "fr-FR" // 法语
	LanguageDeDE Language = "de-DE" // 德语
	LanguageEsES Language = "es-ES" // 西班牙语
	LanguageEsMX Language = "es-MX" // 西班牙语（墨西哥）
	LanguageRuRU Language = "ru-RU" // 俄语
	LanguageItIT Language = "it-IT" // 意大利语
	LanguagePtBR Language = "pt-BR" // 葡萄牙语（巴西）
	LanguageArSA Language = "ar-SA" // 阿拉伯语
	LanguageThTH Language = "th-TH" // 泰语
	LanguageViVN Language = "vi-VN" // 越南语
	LanguageIDID Language = "id-ID" // 印度尼西亚语
)

// Gender 性别类型
type Gender string

// 性别常量
const (
	GenderMale    Gender = "Male"    // 男性
	GenderFemale  Gender = "Female"  // 女性
	GenderNeutral Gender = "Neutral" // 中性
)

// Audio format constants
const (
	// Output formats
	AudioFormatMP3 = "mp3" // MP3 格式，兼容性最好
	AudioFormatPCM = "pcm" // 原始 16-bit PCM 音频，无容器封装
	AudioFormatWAV = "wav" // WAV 格式，无损但文件大

	// Sample rates (Hz)
	SampleRate24kHz = 24000 // TTS 常用采样率
	SampleRate48kHz = 48000 // 专业音频采样率
)

// BoundaryEvent 边界事件（词/句边界）
type BoundaryEvent struct {
	Type       string        // "WordBoundary" 或 "SentenceBoundary"
	Text       string        // 边界文本内容
	Offset     time.Duration // 当前 chunk 内偏移量（已换算为 time.Duration，原始 100ns 单位）
	Duration   time.Duration // 持续时长（已换算为 time.Duration，原始 100ns 单位）
	OffsetMs   int64         // 当前 chunk 内偏移量（毫秒），方便前端消费者直接使用
	DurationMs int64         // 持续时长（毫秒），方便前端消费者直接使用
	ChunkIndex int           // 当前文本块索引（从 0 开始），用于多 chunk 场景下定位或重建时间轴
}

// Boundary event type constants.
const (
	BoundaryTypeWord     = "WordBoundary"
	BoundaryTypeSentence = "SentenceBoundary"
)

// Voice 语音信息 - 最小公共结构
// 只包含跨Provider的通用字段,用于统一查询和过滤
// Extra 字段用于存储 Provider 特有的扩展信息
type Voice struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Language  Language   `json:"language"`            // 主语言代码,如 "zh-CN", "en-US"
	Languages []Language `json:"languages,omitempty"` // 支持的所有语言（包含主语言）
	Gender    Gender     `json:"gender"`              // 性别: Male, Female, Neutral
	Provider  string     `json:"provider"`
	Extra     any        `json:"extra,omitempty"` // Provider 特有的扩展信息
}

// SupportsLanguage 检查是否支持指定语言
// 支持完全匹配或前缀匹配（如 "zh" 匹配 "zh-CN"）
func (v *Voice) SupportsLanguage(lang string) bool {
	lang, ok := normalizeLanguageFilter(lang)
	if !ok {
		return false
	}

	if languageMatchesFilter(v.Language, lang) {
		return true
	}
	for _, l := range v.Languages {
		if languageMatchesFilter(l, lang) {
			return true
		}
	}
	return false
}

func normalizeLanguageFilter(lang string) (string, bool) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return "", false
	}

	parts := strings.Split(lang, "-")
	for index, part := range parts {
		if part == "" {
			return "", false
		}
		if index == 0 && len(part) < 2 {
			return "", false
		}
		for _, ch := range part {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
				return "", false
			}
		}
	}

	return lang, true
}

func languageMatchesFilter(candidate Language, lang string) bool {
	candidateStr := strings.ToLower(strings.TrimSpace(string(candidate)))
	return candidateStr == lang || strings.HasPrefix(candidateStr, lang+"-")
}

// GetExtra 获取特定类型的扩展信息
// 使用泛型提供类型安全的访问方式
//
// 示例:
//
//	if extra, ok := voice.GetExtra[*edgetts.VoiceExtra](); ok {
//	    styles := extra.Styles
//	}
func GetExtra[T any](v *Voice) (T, bool) {
	var zero T
	if v.Extra == nil {
		return zero, false
	}
	extra, ok := v.Extra.(T)
	return extra, ok
}

// SynthesisInputMode describes how the caller provides synthesis content.
type SynthesisInputMode string

const (
	InputModeRawSSML              SynthesisInputMode = "raw_ssml"
	InputModePlainText            SynthesisInputMode = "plain_text"
	InputModePlainTextWithProsody SynthesisInputMode = "plain_text_with_prosody"
)

// ProsodyParams carries provider-neutral speech prosody controls.
// Unset fields mean "provider default". Volume 0 is ambiguous in a plain
// Go struct literal, so callers should set VolumeSet=true or use WithVolume(0)
// when they need an explicit mute request.
type ProsodyParams struct {
	Rate      float64 `json:"-"`
	RateSet   bool    `json:"-"`
	Volume    float64 `json:"-"`
	VolumeSet bool    `json:"-"`
	Pitch     float64 `json:"-"`
	PitchSet  bool    `json:"-"`
}

type prosodyParamsJSON struct {
	Rate   *float64 `json:"rate,omitempty"`
	Volume *float64 `json:"volume,omitempty"`
	Pitch  *float64 `json:"pitch,omitempty"`
}

func (p ProsodyParams) MarshalJSON() ([]byte, error) {
	payload := prosodyParamsJSON{}
	if p.HasRate() {
		payload.Rate = &p.Rate
	}
	if p.HasVolume() {
		payload.Volume = &p.Volume
	}
	if p.HasPitch() {
		payload.Pitch = &p.Pitch
	}
	return json.Marshal(payload)
}

func (p *ProsodyParams) UnmarshalJSON(data []byte) error {
	var payload prosodyParamsJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*p = ProsodyParams{}
	if payload.Rate != nil {
		p.Rate = *payload.Rate
		p.RateSet = true
	}
	if payload.Volume != nil {
		p.Volume = *payload.Volume
		p.VolumeSet = true
	}
	if payload.Pitch != nil {
		p.Pitch = *payload.Pitch
		p.PitchSet = true
	}

	return nil
}

func (p ProsodyParams) WithRate(value float64) ProsodyParams {
	p.Rate = value
	p.RateSet = true
	return p
}

func (p ProsodyParams) WithVolume(value float64) ProsodyParams {
	p.Volume = value
	p.VolumeSet = true
	return p
}

func (p ProsodyParams) WithPitch(value float64) ProsodyParams {
	p.Pitch = value
	p.PitchSet = true
	return p
}

func (p ProsodyParams) HasRate() bool {
	return p.RateSet || p.Rate != 0
}

func (p ProsodyParams) HasVolume() bool {
	return p.VolumeSet || p.Volume != 0
}

func (p ProsodyParams) HasPitch() bool {
	return p.PitchSet || p.Pitch != 0
}

func (p ProsodyParams) IsZero() bool {
	return !p.HasRate() && !p.HasVolume() && !p.HasPitch()
}

func (p ProsodyParams) Validate(provider string) error {
	if err := validateProsodyValue(provider, "rate", p.HasRate(), p.Rate, 0.5, 2.0); err != nil {
		return err
	}
	if err := validateProsodyValue(provider, "volume", p.HasVolume(), p.Volume, 0.0, 2.0); err != nil {
		return err
	}
	if err := validateProsodyValue(provider, "pitch", p.HasPitch(), p.Pitch, 0.5, 2.0); err != nil {
		return err
	}
	return nil
}

func validateProsodyValue(provider, field string, hasValue bool, value, min, max float64) error {
	if !hasValue {
		return nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return &Error{Code: ErrCodeInvalidInput, Message: fmt.Sprintf("prosody %s must be a finite value", field), Provider: provider}
	}
	if value < min || value > max {
		return &Error{Code: ErrCodeInvalidInput, Message: fmt.Sprintf("prosody %s must be between %.1f and %.1f", field, min, max), Provider: provider}
	}
	return nil
}

// ProviderCapabilities describes the caller-facing feature set of a provider.
type ProviderCapabilities struct {
	RawSSML              bool     `json:"raw_ssml"`
	ProsodyParams        bool     `json:"prosody_params"`
	PlainTextOnly        bool     `json:"plain_text_only"`
	BoundaryEvents       bool     `json:"boundary_events"`
	Streaming            bool     `json:"streaming"`
	SupportedFormats     []string `json:"supported_formats,omitempty"`
	PreferredAudioFormat string   `json:"preferred_audio_format,omitempty"`
}

// Clone returns a defensive copy of the capability set.
func (c ProviderCapabilities) Clone() ProviderCapabilities {
	c.SupportedFormats = slices.Clone(c.SupportedFormats)
	return c
}

// SupportsFormat reports whether the caller-facing output format is accepted.
func (c ProviderCapabilities) SupportsFormat(format string) bool {
	requested := normalizeAudioFormat(format)
	if requested == "" {
		return true
	}
	for _, candidate := range c.SupportedFormats {
		if normalizeAudioFormat(candidate) == requested {
			return true
		}
	}
	return false
}

// ResolvedOutputFormat returns the normalized requested format or the provider
// preferred format when the request did not specify one.
func (c ProviderCapabilities) ResolvedOutputFormat(requested string) string {
	if normalized := normalizeAudioFormat(requested); normalized != "" {
		return normalized
	}
	return normalizeAudioFormat(c.PreferredAudioFormat)
}

// SynthesisRequest is the provider-neutral synthesis contract shared across
// all providers.
type SynthesisRequest struct {
	InputMode          SynthesisInputMode `json:"input_mode"`
	Text               string             `json:"text,omitempty"`
	SSML               string             `json:"ssml,omitempty"`
	Prosody            ProsodyParams      `json:"prosody,omitempty"`
	VoiceID            string             `json:"voice_id,omitempty"`
	OutputFormat       string             `json:"output_format,omitempty"`
	NeedBoundaryEvents bool               `json:"need_boundary_events,omitempty"`
}

// Validate checks the request shape independently of any concrete provider.
func (r SynthesisRequest) Validate(provider string) error {
	provider = normalizeProviderName(provider)

	switch r.InputMode {
	case InputModeRawSSML:
		if strings.TrimSpace(r.SSML) == "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "ssml cannot be empty in raw_ssml mode", Provider: provider}
		}
		if strings.TrimSpace(r.Text) != "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "text must be empty in raw_ssml mode", Provider: provider}
		}
		if !r.Prosody.IsZero() {
			return &Error{Code: ErrCodeInvalidInput, Message: "prosody is not allowed in raw_ssml mode", Provider: provider}
		}
	case InputModePlainText:
		if strings.TrimSpace(r.Text) == "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "text cannot be empty in plain_text mode", Provider: provider}
		}
		if strings.TrimSpace(r.SSML) != "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "ssml must be empty in plain_text mode", Provider: provider}
		}
		if !r.Prosody.IsZero() {
			return &Error{Code: ErrCodeInvalidInput, Message: "prosody is not allowed in plain_text mode", Provider: provider}
		}
	case InputModePlainTextWithProsody:
		if strings.TrimSpace(r.Text) == "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "text cannot be empty in plain_text_with_prosody mode", Provider: provider}
		}
		if strings.TrimSpace(r.SSML) != "" {
			return &Error{Code: ErrCodeInvalidInput, Message: "ssml must be empty in plain_text_with_prosody mode", Provider: provider}
		}
		if r.Prosody.IsZero() {
			return &Error{Code: ErrCodeInvalidInput, Message: "plain_text_with_prosody mode requires at least one prosody field; set VolumeSet to send volume 0 explicitly", Provider: provider}
		}
		if err := r.Prosody.Validate(provider); err != nil {
			return err
		}
	default:
		return &Error{Code: ErrCodeInvalidInput, Message: "input mode is required", Provider: provider}
	}

	return nil
}

// ValidateAgainst checks the request against a provider capability set.
func (r SynthesisRequest) ValidateAgainst(provider string, capabilities ProviderCapabilities) error {
	if err := r.Validate(provider); err != nil {
		return err
	}

	provider = normalizeProviderName(provider)

	if capabilities.PlainTextOnly && r.InputMode != InputModePlainText {
		return &Error{Code: ErrCodeUnsupportedCapability, Message: "provider only supports plain text input", Provider: provider}
	}

	switch r.InputMode {
	case InputModeRawSSML:
		if !capabilities.RawSSML {
			return &Error{Code: ErrCodeUnsupportedCapability, Message: "raw SSML is not supported by provider", Provider: provider}
		}
	case InputModePlainTextWithProsody:
		if !capabilities.ProsodyParams {
			return &Error{Code: ErrCodeUnsupportedCapability, Message: "prosody parameters are not supported by provider", Provider: provider}
		}
	}

	if r.NeedBoundaryEvents && !capabilities.BoundaryEvents {
		return &Error{Code: ErrCodeUnsupportedCapability, Message: "boundary events are not supported by provider", Provider: provider}
	}

	resolvedFormat := capabilities.ResolvedOutputFormat(r.OutputFormat)
	if resolvedFormat != "" && !capabilities.SupportsFormat(resolvedFormat) {
		return &Error{Code: ErrCodeUnsupportedFormat, Message: "output format is not supported by provider", Provider: provider}
	}

	return nil
}

// ValidateStreamAgainst checks the request against a provider capability set
// and the public streaming contract.
//
// Streaming=false means callers must treat the provider as synchronous-only.
func (r SynthesisRequest) ValidateStreamAgainst(provider string, capabilities ProviderCapabilities) error {
	if err := r.ValidateAgainst(provider, capabilities); err != nil {
		return err
	}

	provider = normalizeProviderName(provider)

	if !capabilities.Streaming {
		return &Error{Code: ErrCodeUnsupportedCapability, Message: "streaming synthesis is not supported by provider", Provider: provider}
	}

	if r.NeedBoundaryEvents {
		return &Error{Code: ErrCodeUnsupportedCapability, Message: "boundary events are only available from Synthesize results", Provider: provider}
	}

	return nil
}

// ResolvedOutputFormat returns the normalized requested output format or the
// provider preferred format when omitted.
func (r SynthesisRequest) ResolvedOutputFormat(capabilities ProviderCapabilities) string {
	return capabilities.ResolvedOutputFormat(r.OutputFormat)
}

// SynthesisResult contains synthesized audio and provider-neutral metadata.
type SynthesisResult struct {
	Audio          []byte          `json:"audio,omitempty"`
	Format         string          `json:"format"`
	SampleRate     int             `json:"sample_rate"`
	Duration       time.Duration   `json:"duration"`
	Provider       string          `json:"provider"`
	VoiceID        string          `json:"voice_id"`
	BoundaryEvents []BoundaryEvent `json:"boundary_events,omitempty"`
	Limitations    []string        `json:"limitations,omitempty"`
}

// AudioStream 音频流接口
type AudioStream interface {
	// Read 读取音频数据块
	Read() ([]byte, error)

	// Close 关闭流
	Close() error
}

// Provider is the single provider-neutral TTS contract exposed to callers.
type Provider interface {
	// Name 返回提供商名称
	Name() string

	// Capabilities returns the caller-facing feature set of the provider.
	Capabilities() ProviderCapabilities

	// Synthesize 同步合成语音,返回完整结果与元数据
	Synthesize(ctx context.Context, request SynthesisRequest) (*SynthesisResult, error)

	// SynthesizeStream 流式合成语音,返回音频流
	SynthesizeStream(ctx context.Context, request SynthesisRequest) (AudioStream, error)

	// ListVoices 列出可用的语音列表
	ListVoices(ctx context.Context, filter VoiceFilter) ([]Voice, error)
}

// Error 错误类型
type Error struct {
	Code     string
	Message  string
	Provider string
	Err      error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Provider + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Provider + ": " + e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

// 常见错误代码
const (
	ErrCodeInvalidInput          = "INVALID_INPUT"
	ErrCodeAuthFailed            = "AUTH_FAILED"
	ErrCodeQuotaExceeded         = "QUOTA_EXCEEDED"
	ErrCodeProviderUnavail       = "PROVIDER_UNAVAILABLE"
	ErrCodeNetworkError          = "NETWORK_ERROR"
	ErrCodeInternalError         = "INTERNAL_ERROR"
	ErrCodeUnsupportedFormat     = "UNSUPPORTED_FORMAT"
	ErrCodeUnsupportedVoice      = "UNSUPPORTED_VOICE"
	ErrCodeUnsupportedCapability = "UNSUPPORTED_CAPABILITY"
	ErrCodeClockSkew             = "CLOCK_SKEW_ERROR"    // 时钟不同步 (403 错误)
	ErrCodeTimeout               = "TIMEOUT_ERROR"       // 连接/读取超时
	ErrCodeNoAudioReceived       = "NO_AUDIO_RECEIVED"   // 未接收到音频数据
	ErrCodeUnexpectedResponse    = "UNEXPECTED_RESPONSE" // 意外响应
	ErrCodeWebSocketError        = "WEBSOCKET_ERROR"     // WebSocket 连接错误
)

// IsRetryableError checks if a TTS error should be retried
// This function is used by providers to determine if a failed operation should be retried
func IsRetryableError(err error) bool {
	var ttsErr *Error
	if err == nil {
		return false
	}

	// Context cancellation and deadline errors should never be retried
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Non-TTS errors are not retryable by default
	if !errors.As(err, &ttsErr) {
		return false
	}

	switch ttsErr.Code {
	case ErrCodeClockSkew,
		ErrCodeNetworkError,
		ErrCodeWebSocketError,
		ErrCodeTimeout,
		ErrCodeNoAudioReceived,
		ErrCodeProviderUnavail:
		return true
	default:
		return false
	}
}

// RetryOptions creates standard retry configuration for TTS providers.
// This function centralizes retry configuration to ensure consistency across all providers.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - maxAttempts: Maximum number of retry attempts (including the first try)
//
// Returns a slice of retry.Option configured with:
//   - Exponential backoff with jitter (100ms initial, 5s max, 1s jitter)
//   - IsRetryableError condition to determine which errors should trigger retries
//
// Usage example:
//
//	err := retry.Do(func() error {
//	    return someOperation()
//	}, tts.RetryOptions(ctx, maxAttempts)...)
func RetryOptions(ctx context.Context, maxAttempts int) []retry.Option {
	return []retry.Option{
		retry.WithContext(ctx),
		retry.WithTimes(maxAttempts),
		retry.WithExponentialBackoff(DefaultBackoffInitial, DefaultBackoffMax, DefaultBackoffJitter),
		retry.WithLogger(func(string, ...any) {}),
		retry.WithRetryCondition(IsRetryableError),
	}
}

func normalizeAudioFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

func normalizeProviderName(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "tts"
	}
	return provider
}
