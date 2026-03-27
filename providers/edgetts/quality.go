package edgetts

import "github.com/simp-lee/ttsbridge/tts"

// OutputOptions 返回 EdgeTTS 推荐的输出格式目录（按比特率从低到高排列）。
//
// 这些条目是稳定的格式目录与音频 profile 映射，不代表当前环境已经验证可用。
// 如需得到当前环境下已验证可用的格式，请先对 FormatRegistry 做显式 probe，
// 再读取 provider.SupportedFormats()。
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
	const verifiedNote = "目录条目；当前环境可用性需显式 probe"

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
			Profile:     tts.VoiceAudioProfile{Format: tts.AudioFormatPCM, SampleRate: 24000, Channels: 1, Lossless: true},
			Verified:    verifiedNote,
		},
	}
}
