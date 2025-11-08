package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	fmt.Println("TTSBridge 新功能演示")
	fmt.Println("====================")
	fmt.Println()

	// 1. 基本使用（默认配置）
	fmt.Println("1. 基本使用")
	basicUsage()

	// 2. 使用元数据回调获取字幕信息
	fmt.Println("\n2. 元数据回调演示")
	metadataDemo()

	// 3. 自定义配置（代理、超时等）
	fmt.Println("\n3. 自定义配置演示")
	customConfigDemo()

	// 4. 获取真实的语音列表
	fmt.Println("\n4. 获取语音列表")
	listVoicesDemo()

	// 5. 错误处理演示
	fmt.Println("\n5. 错误处理演示")
	errorHandlingDemo()
}

// basicUsage 基本使用示例
func basicUsage() {
	provider := edgetts.New()

	opts := &tts.SynthesizeOptions{
		Text:   "你好，欢迎使用 TTSBridge！",
		Voice:  "zh-CN-XiaoxiaoNeural",
		Rate:   1.0,
		Volume: 1.0,
		Pitch:  1.0,
	}

	ctx := context.Background()
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v\n", err)
		return
	}

	filename := "output_basic.mp3"
	if err := os.WriteFile(filename, audio, 0644); err != nil {
		log.Printf("保存失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 合成成功，已保存到 %s (大小: %d 字节)\n", filename, len(audio))
}

// metadataDemo 元数据回调演示
func metadataDemo() {
	provider := edgetts.New()

	// 用于收集字幕信息
	type Subtitle struct {
		Text      string
		StartTime time.Duration
		EndTime   time.Duration
	}
	var subtitles []Subtitle

	opts := &tts.SynthesizeOptions{
		Text:                "这是第一句话。这是第二句话。这是第三句话。",
		Voice:               "zh-CN-XiaoxiaoNeural",
		WordBoundaryEnabled: true, // 启用词边界
		MetadataCallback: func(metadataType string, offset int64, duration int64, text string) {
			// 实时接收词边界信息
			if metadataType == "WordBoundary" {
				subtitles = append(subtitles, Subtitle{
					Text:      text,
					StartTime: time.Duration(offset) * 100 * time.Nanosecond,
					EndTime:   time.Duration(offset+duration) * 100 * time.Nanosecond,
				})
			}
		},
	}

	ctx := context.Background()
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v\n", err)
		return
	}

	filename := "output_metadata.mp3"
	if err := os.WriteFile(filename, audio, 0644); err != nil {
		log.Printf("保存失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 合成成功，已保存到 %s\n", filename)
	fmt.Printf("✓ 收集到 %d 个词边界:\n", len(subtitles))

	// 显示前 10 个字幕
	displayCount := 10
	if len(subtitles) < displayCount {
		displayCount = len(subtitles)
	}

	for i := 0; i < displayCount; i++ {
		sub := subtitles[i]
		fmt.Printf("  [%v-%v] %s\n",
			sub.StartTime.Round(time.Millisecond),
			sub.EndTime.Round(time.Millisecond),
			sub.Text)
	}

	if len(subtitles) > displayCount {
		fmt.Printf("  ... 还有 %d 个\n", len(subtitles)-displayCount)
	}
}

// customConfigDemo 自定义配置演示
func customConfigDemo() {
	// 创建带自定义配置的 Provider
	provider := edgetts.NewWithOptions(&edgetts.ProviderOptions{
		HTTPTimeout:      60 * time.Second, // HTTP 超时 60 秒
		ConnectTimeout:   15 * time.Second, // WebSocket 连接超时 15 秒
		MaxRetryAttempts: 3,                // 最多重试 3 次
		// ProxyURL: "http://proxy.example.com:8080", // 如果需要代理，取消注释
	})

	opts := &tts.SynthesizeOptions{
		Text:   "这是使用自定义配置的示例。",
		Voice:  "zh-CN-YunxiNeural",
		Rate:   1.2, // 加快 20%
		Volume: 1.0,
		Pitch:  1.0,
	}

	ctx := context.Background()
	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v\n", err)
		return
	}

	filename := "output_custom.mp3"
	if err := os.WriteFile(filename, audio, 0644); err != nil {
		log.Printf("保存失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 使用自定义配置合成成功，已保存到 %s\n", filename)
	fmt.Println("  配置: 超时 60s, 连接超时 15s, 最多重试 3 次")
}

// listVoicesDemo 获取语音列表演示
func listVoicesDemo() {
	provider := edgetts.New()
	ctx := context.Background()

	// 获取所有中文语音
	voices, err := provider.ListVoices(ctx, "zh-CN")
	if err != nil {
		log.Printf("获取语音列表失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 找到 %d 个中文语音:\n", len(voices))

	// 显示前 10 个
	displayCount := 10
	if len(voices) < displayCount {
		displayCount = len(voices)
	}

	for i := 0; i < displayCount; i++ {
		v := voices[i]
		displayName := v.DisplayName
		if displayName == "" {
			displayName = v.Name
		}
		if displayName == "" {
			displayName = v.ShortName
		}
		fmt.Printf("  %s (%s)\n", displayName, v.Gender)
		fmt.Printf("    ID: %s\n", v.ShortName)
		if len(v.Styles) > 0 {
			fmt.Printf("    风格: %v\n", v.Styles)
		}
	}

	if len(voices) > displayCount {
		fmt.Printf("  ... 还有 %d 个\n", len(voices)-displayCount)
	}
}

// errorHandlingDemo 错误处理演示
func errorHandlingDemo() {
	provider := edgetts.New()

	// 使用错误的语音名称
	opts := &tts.SynthesizeOptions{
		Text:  "测试错误处理",
		Voice: "invalid-voice-name",
	}

	ctx := context.Background()
	_, err := provider.Synthesize(ctx, opts)

	if err != nil {
		// 检查是否是 TTS 特定错误
		if ttsErr, ok := err.(*tts.Error); ok {
			fmt.Printf("✓ 捕获到 TTS 错误:\n")
			fmt.Printf("  代码: %s\n", ttsErr.Code)
			fmt.Printf("  消息: %s\n", ttsErr.Message)
			fmt.Printf("  提供商: %s\n", ttsErr.Provider)

			// 根据错误类型进行不同处理
			switch ttsErr.Code {
			case tts.ErrCodeClockSkew:
				fmt.Println("  建议: 检查系统时间是否正确")
			case tts.ErrCodeTimeout:
				fmt.Println("  建议: 增加超时时间或检查网络连接")
			case tts.ErrCodeNoAudioReceived:
				fmt.Println("  建议: 检查语音参数是否正确")
			case tts.ErrCodeNetworkError:
				fmt.Println("  建议: 检查网络连接")
			default:
				fmt.Println("  建议: 检查错误详情进行排查")
			}
		} else {
			fmt.Printf("✗ 普通错误: %v\n", err)
		}
	} else {
		fmt.Println("✗ 预期会出现错误，但合成成功了")
	}
}
