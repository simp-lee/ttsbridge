package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

// boolPtr 返回 bool 值的指针
func boolPtr(b bool) *bool {
	return &b
}

func main() {
	fmt.Println("🎵 TTSBridge 背景音乐混音示例")
	fmt.Println("================================")
	fmt.Println()

	// 检查命令行参数
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go <背景音乐文件路径>")
		fmt.Println("示例: go run main.go background.mp3")
		os.Exit(1)
	}

	musicPath := os.Args[1]

	// 检查背景音乐文件是否存在
	if _, err := os.Stat(musicPath); os.IsNotExist(err) {
		log.Fatalf("❌ 背景音乐文件不存在: %s", musicPath)
	}

	// 创建 Edge TTS 提供商
	provider := edgetts.New()

	// 配置语音合成选项
	opts := &edgetts.SynthesizeOptions{
		Text:   "欢迎使用 TTSBridge 背景音乐混音功能！这是一个示例，演示如何将背景音乐与语音合成结果混合。您可以调节背景音乐的音量、设置淡入淡出效果、选择起始时间点，以及控制是否循环播放。",
		Voice:  "zh-CN-XiaoxiaoNeural",
		Rate:   1.0,
		Volume: 1.0,
		Pitch:  1.0,
		// 配置背景音乐
		BackgroundMusic: &tts.BackgroundMusicOptions{
			MusicPath: musicPath,     // 背景音乐文件路径
			Volume:    0.3,           // 背景音乐音量 30%
			FadeIn:    2.0,           // 淡入 2 秒
			FadeOut:   3.0,           // 淡出 3 秒
			Loop:      boolPtr(true), // 循环播放
		},
	}

	fmt.Println("📝 文本:", opts.Text)
	fmt.Println("🎙️  语音:", opts.Voice)
	fmt.Println("🎵 背景音乐:", musicPath)
	fmt.Println("🔊 背景音乐音量:", fmt.Sprintf("%.0f%%", opts.BackgroundMusic.Volume*100))
	fmt.Println("⏱️  淡入:", fmt.Sprintf("%.1f秒", opts.BackgroundMusic.FadeIn))
	fmt.Println("⏱️  淡出:", fmt.Sprintf("%.1f秒", opts.BackgroundMusic.FadeOut))
	fmt.Println("🔁 循环播放:", *opts.BackgroundMusic.Loop)
	fmt.Println()

	// 开始合成
	fmt.Println("🔄 正在合成语音并混音...")
	ctx := context.Background()
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Fatalf("❌ 合成失败: %v", err)
	}

	// 保存结果
	outputFile := "output_with_music.mp3"
	if err := os.WriteFile(outputFile, audio, 0644); err != nil {
		log.Fatalf("❌ 保存文件失败: %v", err)
	}

	fmt.Printf("✅ 成功！混音后的音频已保存到: %s\n", outputFile)
	fmt.Printf("📊 文件大小: %d 字节\n", len(audio))
}
