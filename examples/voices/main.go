package main

import (
	"context"
	"fmt"
	"log"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	// 创建 Edge TTS 提供商
	provider := edgetts.New()

	// 创建 TTS 引擎
	engine, err := tts.NewEngine(provider)
	if err != nil {
		log.Fatalf("创建引擎失败: %v", err)
	}

	ctx := context.Background()

	// 列出所有中文语音
	log.Println("正在获取可用语音列表...")
	voices, err := engine.ListVoices(ctx, "zh-CN")
	if err != nil {
		log.Fatalf("获取语音列表失败: %v", err)
	}

	fmt.Println("\n=== 中文语音列表 ===")
	for i, voice := range voices {
		fmt.Printf("%d. %s\n", i+1, voice.DisplayName)
		fmt.Printf("   ID: %s\n", voice.ID)
		fmt.Printf("   性别: %s\n", voice.Gender)
		fmt.Printf("   描述: %s\n", voice.Description)
		fmt.Println()
	}

	// 列出所有英文语音
	voices, err = engine.ListVoices(ctx, "en-US")
	if err != nil {
		log.Fatalf("获取语音列表失败: %v", err)
	}

	fmt.Println("\n=== 英文语音列表 ===")
	for i, voice := range voices {
		fmt.Printf("%d. %s\n", i+1, voice.DisplayName)
		fmt.Printf("   ID: %s\n", voice.ID)
		fmt.Printf("   性别: %s\n", voice.Gender)
		fmt.Printf("   描述: %s\n", voice.Description)
		fmt.Println()
	}

	// 列出所有语音
	allVoices, err := engine.ListVoices(ctx, "")
	if err != nil {
		log.Fatalf("获取语音列表失败: %v", err)
	}

	fmt.Printf("\n共有 %d 个可用语音\n", len(allVoices))
}
