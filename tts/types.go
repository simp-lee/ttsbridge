package tts

import (
	"context"
	"errors"
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
	LanguageRuRU Language = "ru-RU" // 俄语
	LanguageItIT Language = "it-IT" // 意大利语
	LanguagePtBR Language = "pt-BR" // 葡萄牙语（巴西）
	LanguageArSA Language = "ar-SA" // 阿拉伯语
	LanguageThTH Language = "th-TH" // 泰语
	LanguageViVN Language = "vi-VN" // 越南语
)

// Gender 性别类型
type Gender string

// 性别常量
const (
	GenderMale    Gender = "Male"    // 男性
	GenderFemale  Gender = "Female"  // 女性
	GenderNeutral Gender = "Neutral" // 中性
)

// Audio quality constants for background music mixing
const (
	// Output formats
	AudioFormatMP3  = "mp3"  // MP3 格式，兼容性最好
	AudioFormatWAV  = "wav"  // WAV 格式，无损但文件大
	AudioFormatFLAC = "flac" // FLAC 格式，无损压缩
	AudioFormatM4A  = "m4a"  // M4A/AAC 格式，高压缩率
	AudioFormatAAC  = "aac"  // AAC 格式（通常用 M4A 容器）

	// MP3 bitrate presets (kbps)
	MP3BitrateBalanced = 256 // 平衡质量，混音 fallback 默认值

	// Sample rates (Hz)
	SampleRate24kHz = 24000 // TTS 常用采样率
	SampleRate44kHz = 44100 // CD 音质采样率
	SampleRate48kHz = 48000 // 专业音频采样率

	// Default values
	DefaultBackgroundMusicVolume = 0.3 // 默认背景音乐音量
	DefaultMainAudioVolume       = 1.0 // 默认主音频音量
)

// BackgroundMusicOptions 背景音乐混音选项（通用组件）
type BackgroundMusicOptions struct {
	// MusicPath 背景音乐文件路径（支持 MP3, WAV, OGG, FLAC 等常见格式）
	MusicPath string

	// Volume 背景音乐音量，范围 0.0-1.0，默认 0.3
	Volume float64

	// FadeIn 淡入时长（秒），默认 0（不淡入）
	FadeIn float64

	// FadeOut 淡出时长（秒），默认 0（不淡出）
	FadeOut float64

	// StartTime 背景音乐起始时间点（秒），默认 0（从头开始）
	StartTime float64

	// Loop 是否循环播放背景音乐以覆盖整个语音长度，默认 true
	// 使用指针类型以便区分"未设置"和"显式设置为 false"
	Loop *bool

	// MainAudioVolume 主音频（语音）音量，范围 0.0-1.0，默认 1.0
	MainAudioVolume float64
}

// BoundaryEvent 边界事件（词/句边界）
type BoundaryEvent struct {
	Type       string        // "WordBoundary" 或 "SentenceBoundary"
	Text       string        // 边界文本内容
	Offset     time.Duration // 偏移量（已换算为 time.Duration，原始 100ns 单位）
	Duration   time.Duration // 持续时长（已换算为 time.Duration，原始 100ns 单位）
	OffsetMs   int64         // 偏移量（毫秒），方便前端消费者直接使用
	DurationMs int64         // 持续时长（毫秒），方便前端消费者直接使用
	ChunkIndex int           // 当前文本块索引（从 0 开始），用于多 chunk 场景下定位
}

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
	langStr := string(v.Language)
	// 检查主语言
	if langStr == lang || len(langStr) >= len(lang) && langStr[:len(lang)] == lang {
		return true
	}
	// 检查多语言列表
	for _, l := range v.Languages {
		lStr := string(l)
		if lStr == lang || len(lStr) >= len(lang) && lStr[:len(lang)] == lang {
			return true
		}
	}
	return false
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

// AudioStream 音频流接口
type AudioStream interface {
	// Read 读取音频数据块
	Read() ([]byte, error)

	// Close 关闭流
	Close() error
}

// Provider 泛型 TTS 提供商接口
// T 为该 Provider 的 SynthesizeOptions 类型
type Provider[T any] interface {
	// Name 返回提供商名称
	Name() string

	// Synthesize 同步合成语音,返回完整的音频数据
	Synthesize(ctx context.Context, opts T) ([]byte, error)

	// SynthesizeStream 流式合成语音,返回音频流
	SynthesizeStream(ctx context.Context, opts T) (AudioStream, error)

	// ListVoices 列出可用的语音列表
	ListVoices(ctx context.Context, locale string) ([]Voice, error)

	// IsAvailable 检查提供商是否可用
	IsAvailable(ctx context.Context) bool
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
	ErrCodeInvalidInput       = "INVALID_INPUT"
	ErrCodeAuthFailed         = "AUTH_FAILED"
	ErrCodeQuotaExceeded      = "QUOTA_EXCEEDED"
	ErrCodeProviderUnavail    = "PROVIDER_UNAVAILABLE"
	ErrCodeNetworkError       = "NETWORK_ERROR"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeUnsupportedFormat  = "UNSUPPORTED_FORMAT"
	ErrCodeUnsupportedVoice   = "UNSUPPORTED_VOICE"
	ErrCodeClockSkew          = "CLOCK_SKEW_ERROR"    // 时钟不同步 (403 错误)
	ErrCodeTimeout            = "TIMEOUT_ERROR"       // 连接/读取超时
	ErrCodeNoAudioReceived    = "NO_AUDIO_RECEIVED"   // 未接收到音频数据
	ErrCodeUnexpectedResponse = "UNEXPECTED_RESPONSE" // 意外响应
	ErrCodeWebSocketError     = "WEBSOCKET_ERROR"     // WebSocket 连接错误
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
