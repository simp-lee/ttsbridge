package tts

// OutputOption 描述一个 Provider 提供的输出格式目录项。
// 尽管文件名沿用 quality，这里承载的是输出选项元数据，不是独立的 quality 领域模型。
// 该目录能力属于 Provider 的可选扩展，不属于通用 [Provider] 接口；是否已在当前环境
// 验证，取决于各 Provider 在 Verified 字段中的说明以及其 FormatRegistry 的 probe 结果。
//
// 用法示例:
//
//	provider := edgetts.New()
//
//	// 1. 若具体 Provider 暴露了 OutputOptions()，可查看其推荐格式目录
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

	// Verified 格式来源或验证说明。
	// 例如: "实际探测验证 (2026-02-21, Edge TTS WebSocket API)"
	// 或: "目录条目；当前环境可用性需显式 probe"。
	Verified string `json:"verified,omitempty"`
}
