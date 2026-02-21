package volcengine

import "github.com/simp-lee/ttsbridge/tts"

// OutputOptions 返回 Volcengine 支持的输出格式选项列表。
//
// Volcengine 免费翻译 TTS API 的输出格式由服务端固定，请求参数中不包含格式选择字段
// (仅 text + speaker)。实测始终返回 WAV 24kHz 16-bit 单声道无损音频。
//
// 格式来源: API 响应的 WAV header 实际解析结果，非文档推测。
// API 无格式切换能力，因此无需像 EdgeTTS 那样进行多格式探测。
func (p *Provider) OutputOptions() []tts.OutputOption {
	return []tts.OutputOption{
		{
			FormatID:    OutputFormatWAV_24khz,
			Label:       "WAV 24kHz 无损",
			Description: "Volcengine 免费 API 固定输出 WAV 24kHz 16-bit 单声道无损音频，不可更改",
			Profile:     wavProfile,
			IsDefault:   true,
			Verified:    "API 响应 WAV header 实际解析 (Volcengine 免费翻译 TTS, 无格式选择参数)",
		},
	}
}
