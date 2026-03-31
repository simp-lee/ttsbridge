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
	edgeVoices := printEdgeVoices(ctx)
	volcVoices := printVolcengineVoices(ctx)
	printUnifiedPreview(append(edgeVoices[:2], volcVoices[:2]...))
}

func printEdgeVoices(ctx context.Context) []tts.Voice {
	fmt.Println("=== EdgeTTS 语音扩展信息 ===")
	edgeVoices := mustListVoices(ctx, "EdgeTTS", edgetts.New())

	for i, voice := range edgeVoices {
		if i >= 3 { // 只显示前3个
			break
		}

		printVoiceSummary(voice)
		printEdgeVoiceExtra(voice)
	}
	return edgeVoices
}

func printVolcengineVoices(ctx context.Context) []tts.Voice {
	fmt.Println("\n\n=== Volcengine 语音扩展信息 ===")
	volcVoices := mustListVoices(ctx, "Volcengine", volcengine.New())

	for i, voice := range volcVoices {
		if i >= 3 { // 只显示前3个
			break
		}

		printVoiceSummary(voice)
		printVolcengineVoiceExtra(voice)
	}
	return volcVoices
}

func printUnifiedPreview(voices []tts.Voice) {
	fmt.Println("\n\n=== 统一处理示例 ===")
	for _, voice := range voices {
		fmt.Printf("\n%s - %s (%s)\n", voice.Provider, voice.Name, voice.Language)
		printUnifiedVoiceExtra(voice)
	}
}

func mustListVoices(ctx context.Context, providerName string, provider tts.Provider) []tts.Voice {
	voices, err := provider.ListVoices(ctx, tts.VoiceFilter{Language: "zh-CN"})
	if err != nil {
		log.Fatalf("获取 %s 语音列表失败: %v", providerName, err)
	}
	return voices
}

func printVoiceSummary(voice tts.Voice) {
	fmt.Printf("\n语音: %s (%s)\n", voice.Name, voice.ID)
	fmt.Printf("  语言: %s\n", voice.Language)
	fmt.Printf("  性别: %s\n", voice.Gender)
}

func printEdgeVoiceExtra(voice tts.Voice) {
	if extra, ok := tts.GetExtra[*edgetts.VoiceExtra](&voice); ok {
		fmt.Printf("  状态: %s\n", extra.Status)
		if extra.FriendlyName != "" {
			fmt.Printf("  友好名称: %s\n", extra.FriendlyName)
		}
		if len(extra.Categories) > 0 {
			fmt.Printf("  分类: %v\n", extra.Categories)
		}
		if len(extra.Personalities) > 0 {
			fmt.Printf("  个性: %v\n", extra.Personalities)
		}
	}
}

func printVolcengineVoiceExtra(voice tts.Voice) {
	if extra, ok := tts.GetExtra[*volcengine.VoiceExtra](&voice); ok {
		fmt.Printf("  分类: %s\n", extra.Category)
		fmt.Printf("  格式: %s\n", extra.Format)
		fmt.Printf("  采样率: %d Hz\n", extra.SampleRate)
		if len(extra.SceneTags) > 0 {
			fmt.Printf("  场景标签: %v\n", extra.SceneTags)
		}
	}
}

func printUnifiedVoiceExtra(voice tts.Voice) {
	switch voice.Provider {
	case "edgetts":
		if extra, ok := tts.GetExtra[*edgetts.VoiceExtra](&voice); ok {
			fmt.Printf("  EdgeTTS 状态: %s\n", extra.Status)
		}
	case "volcengine":
		if extra, ok := tts.GetExtra[*volcengine.VoiceExtra](&voice); ok {
			fmt.Printf("  Volcengine 分类: %s\n", extra.Category)
		}
	}
}
