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

	// 示例 1: EdgeTTS - 访问特有的风格和角色信息
	fmt.Println("=== EdgeTTS 语音扩展信息 ===")
	edgeProvider := edgetts.New()
	edgeVoices, err := edgeProvider.ListVoices(ctx, "zh-CN")
	if err != nil {
		log.Fatalf("获取 EdgeTTS 语音列表失败: %v", err)
	}

	for i, voice := range edgeVoices {
		if i >= 3 { // 只显示前3个
			break
		}

		fmt.Printf("\n语音: %s (%s)\n", voice.Name, voice.ID)
		fmt.Printf("  语言: %s\n", voice.Language)
		fmt.Printf("  性别: %s\n", voice.Gender)

		// 使用类型断言访问 EdgeTTS 特有信息
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

	// 示例 2: Volcengine - 访问特有的分类和标签信息
	fmt.Println("\n\n=== Volcengine 语音扩展信息 ===")
	volcProvider := volcengine.New()
	volcVoices, err := volcProvider.ListVoices(ctx, "zh-CN")
	if err != nil {
		log.Fatalf("获取 Volcengine 语音列表失败: %v", err)
	}

	for i, voice := range volcVoices {
		if i >= 3 { // 只显示前3个
			break
		}

		fmt.Printf("\n语音: %s (%s)\n", voice.Name, voice.ID)
		fmt.Printf("  语言: %s\n", voice.Language)
		fmt.Printf("  性别: %s\n", voice.Gender)

		// 使用类型断言访问 Volcengine 特有信息
		if extra, ok := tts.GetExtra[*volcengine.VoiceExtra](&voice); ok {
			fmt.Printf("  分类: %s\n", extra.Category)
			fmt.Printf("  格式: %s\n", extra.Format)
			fmt.Printf("  采样率: %d Hz\n", extra.SampleRate)
			if len(extra.SceneTags) > 0 {
				fmt.Printf("  场景标签: %v\n", extra.SceneTags)
			}
		}
	}

	// 示例 3: 统一处理不同 Provider 的语音
	fmt.Println("\n\n=== 统一处理示例 ===")
	allVoices := append(edgeVoices[:2], volcVoices[:2]...)

	for _, voice := range allVoices {
		fmt.Printf("\n%s - %s (%s)\n", voice.Provider, voice.Name, voice.Language)

		// 根据 Provider 类型访问不同的 Extra 信息
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
}
