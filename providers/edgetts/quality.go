package edgetts

import "github.com/simp-lee/ttsbridge/tts"

// OutputOptions 返回 EdgeTTS 支持的输出格式选项列表（按比特率从低到高排列）。
//
// 格式来源: 通过实际调用 Edge TTS WebSocket API 探测验证（2026-02-21）。
// Edge TTS 注册表中共 37 个格式，其中 9 个验证为可用（FormatAvailable），
// 28 个验证为不可用（FormatUnavailable）。本方法仅返回推荐的 5 种常用格式。
// 完整的 9 个可用格式可通过 provider.FormatRegistry().Available() 获取。
//
// 如需重新验证:
//
//	go test -v -tags probe -run TestEdgeTTSFormatProbe ./providers/edgetts/
//
// 使用示例:
//
//	provider := edgetts.New()
//	for _, opt := range provider.OutputOptions() {
//	    fmt.Printf("%-45s %s\n", opt.FormatID, opt.Label)
//	}
func (p *Provider) OutputOptions() []tts.OutputOption {
	const verifiedNote = "实际探测验证 (2026-02-21, Edge TTS WebSocket API)"

	return []tts.OutputOption{
		{
			FormatID:    OutputFormatMP3_24khz_48k,
			Label:       "MP3 24kHz 48kbps",
			Description: "最小文件体积，适合预览和带宽受限场景",
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 48},
			IsDefault:   true,
			Verified:    verifiedNote,
		},
		{
			FormatID:    OutputFormatMP3_24khz_96k,
			Label:       "MP3 24kHz 96kbps",
			Description: "平衡音质与体积，适合大多数使用场景",
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 24000, Channels: 1, Bitrate: 96},
			Verified:    verifiedNote,
		},
		{
			FormatID:    OutputFormatMP3_48khz_192k,
			Label:       "MP3 48kHz 192kbps",
			Description: "高音质，适合播客、有声书、视频配音",
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 192},
			Verified:    verifiedNote,
		},
		{
			FormatID:    OutputFormatMP3_48khz_320k,
			Label:       "MP3 48kHz 320kbps",
			Description: "最高 MP3 音质，适合专业制作和正式发布",
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatMP3, SampleRate: 48000, Channels: 1, Bitrate: 320},
			Verified:    verifiedNote,
		},
		{
			FormatID:    OutputFormatPCM_24khz,
			Label:       "PCM 24kHz 无损",
			Description: "无损音频，文件体积大，适合存档或后期加工",
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatWAV, SampleRate: 24000, Channels: 1, Lossless: true},
			Verified:    verifiedNote,
		},
	}
}
