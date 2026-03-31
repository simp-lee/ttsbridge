package main

import (
	"context"
	"fmt"
	"log"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	ctx := context.Background()

	// 示例 1: 查询统一输出能力
	example1ShowCapabilities()

	// 示例 2: 使用 OutputFormat 选择输出格式
	example2SelectOutputFormat(ctx)
}

// 示例 1: 查看统一输出能力
func example1ShowCapabilities() {
	fmt.Println("=== 示例 1: 查看统一输出能力 ===")

	printCapabilities("EdgeTTS", edgetts.New().Capabilities())
	printCapabilities("Volcengine", volcengine.New().Capabilities())
}

func printCapabilities(name string, caps tts.ProviderCapabilities) {
	fmt.Printf("\n%s\n", name)
	fmt.Printf("  SupportedFormats: %v\n", caps.SupportedFormats)
	fmt.Printf("  PreferredAudioFormat: %s\n", caps.PreferredAudioFormat)
}

// 示例 2: 使用共享 OutputFormat 选择最终输出容器
func example2SelectOutputFormat(ctx context.Context) {
	fmt.Println("\n=== 示例 2: 选择输出格式 ===")

	provider := edgetts.New()
	caps := provider.Capabilities()
	fmt.Printf("EdgeTTS 当前共享输出能力: %v\n", caps.SupportedFormats)
	if !caps.SupportsFormat(tts.AudioFormatWAV) {
		_, err := provider.Synthesize(ctx, tts.SynthesisRequest{
			InputMode:    tts.InputModePlainText,
			Text:         "这段文本不会发送到服务端，因为共享层不支持 WAV。",
			VoiceID:      "zh-CN-XiaoxiaoNeural",
			OutputFormat: tts.AudioFormatWAV,
		})
		if err != nil {
			fmt.Printf("显式请求 WAV 会被快速拒绝: %v\n", err)
		}
	}

	// Edge live 当前只对共享层声明 MP3。
	request := tts.SynthesisRequest{
		InputMode:    tts.InputModePlainText,
		Text:         "这是一段显式使用 MP3 输出的测试文本。",
		VoiceID:      "zh-CN-XiaoxiaoNeural",
		OutputFormat: tts.AudioFormatMP3,
	}

	result, err := provider.Synthesize(ctx, request)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("显式 MP3 生成音频: %d 字节 (格式=%s, 采样率=%d, 时长=%s)\n", len(result.Audio), result.Format, result.SampleRate, result.Duration)

	// 对比: 使用默认格式 (不设置 OutputFormat)
	defaultRequest := tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      "这是一段使用默认格式的测试文本。",
		VoiceID:   "zh-CN-XiaoxiaoNeural",
	}

	defaultResult, err := provider.Synthesize(ctx, defaultRequest)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("默认格式 (%s) 生成音频: %d 字节\n", defaultResult.Format, len(defaultResult.Audio))
}
