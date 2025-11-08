package tts

import "context"

// SynthesizeOptions 语音合成选项
type SynthesizeOptions struct {
	// Text 要合成的文本内容
	Text string

	// Voice 语音名称,如 "zh-CN-XiaoxiaoNeural"
	Voice string

	// Rate 语速,范围通常为 0.5-2.0,1.0 为正常速度
	Rate float64

	// Volume 音量,范围通常为 0.0-1.0,1.0 为最大音量
	Volume float64

	// Pitch 音调,范围通常为 0.5-2.0,1.0 为正常音调
	Pitch float64

	// Locale 语言区域,如 "zh-CN", "en-US"
	Locale string

	// WordBoundaryEnabled 是否启用词边界元数据 (默认: false)
	WordBoundaryEnabled bool

	// SentenceBoundaryEnabled 是否启用句边界元数据 (默认: false)
	SentenceBoundaryEnabled bool

	// MetadataCallback 元数据回调函数，用于接收词/句边界信息
	// 参数: metadataType (WordBoundary/SentenceBoundary), offset (时间偏移), duration (持续时间), text (文本)
	MetadataCallback func(metadataType string, offset int64, duration int64, text string)

	// BackgroundMusic 背景音乐选项
	BackgroundMusic *BackgroundMusicOptions

	// Extra 提供商特定的额外参数
	Extra map[string]interface{}
}

// BackgroundMusicOptions 背景音乐混音选项
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

// Voice 语音信息
type Voice struct {
	// ID 语音唯一标识
	ID string

	// Name 语音名称,如 "晓晓"
	Name string

	// DisplayName 显示名称,如 "晓晓 (女性)"
	DisplayName string

	// Locale 语言区域,如 "zh-CN"
	Locale string

	// Gender 性别: Male, Female, Neutral
	Gender string

	// ShortName 短名称,如 "zh-CN-XiaoxiaoNeural"
	ShortName string

	// Provider 提供商名称
	Provider string

	// Styles 支持的风格列表,如 ["affectionate", "angry", "cheerful"]
	Styles []string

	// Description 描述信息
	Description string
}

// AudioStream 音频流接口
type AudioStream interface {
	// Read 读取音频数据块
	Read() ([]byte, error)

	// Close 关闭流
	Close() error
}

// Provider TTS 提供商接口
type Provider interface {
	// Name 返回提供商名称
	Name() string

	// Synthesize 同步合成语音,返回完整的音频数据
	Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error)

	// SynthesizeStream 流式合成语音,返回音频流
	SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (AudioStream, error)

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
