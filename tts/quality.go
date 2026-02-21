package tts

// OutputOption 描述一个 Provider 支持的输出格式选项。
// 通过 Provider 的 OutputOptions() 方法获取可用列表，每个选项都对应一个经过验证的输出格式。
//
// 用法示例:
//
//	provider := edgetts.New()
//
//	// 1. 查看可用格式
//	for _, opt := range provider.OutputOptions() {
//	    fmt.Println(opt.FormatID, opt.Label)
//	}
//
//	// 2. 使用返回的 FormatID 调用合成
//	opts := &edgetts.SynthesizeOptions{
//	    Text:         "你好世界",
//	    Voice:        "zh-CN-XiaoxiaoNeural",
//	    OutputFormat: edgetts.OutputFormatMP3_48khz_192k, // 从 OutputOptions 中选择
//	}
type OutputOption struct {
	// FormatID Provider 内部的格式标识符，可直接传入 SynthesizeOptions.OutputFormat。
	// EdgeTTS 示例: "audio-48khz-192kbitrate-mono-mp3"
	// Volcengine 示例: "wav-24khz-16bit-mono"
	FormatID string `json:"format_id"`

	// Label 人类可读的短标签，如 "MP3 48kHz 192kbps"
	Label string `json:"label"`

	// Description 使用场景的详细说明
	Description string `json:"description,omitempty"`

	// Profile 该格式的音频特征（编码格式、采样率、声道数、比特率等）
	Profile VoiceAudioProfile `json:"profile"`

	// IsDefault 是否为 Provider 的默认输出格式
	IsDefault bool `json:"is_default,omitempty"`

	// Verified 格式验证来源说明。
	// 例如: "实际探测验证 (2026-02-21, Edge TTS WebSocket API)"
	// 用于向调用者说明该格式数据的可信来源，而非猜测或文档抄录。
	Verified string `json:"verified,omitempty"`
}
