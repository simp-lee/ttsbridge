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

	// 示例 1: EdgeTTS - 音质由接口参数决定
	example1_EdgeTTS(ctx)

	// 示例 2: Volcengine - 音质由代码测试确定
	example2_Volcengine(ctx)

	// 示例 3: 混音配置 - 输出音质完全由 TTS 语音决定
	example3_BackgroundMusic(ctx)
}

// 示例 1: EdgeTTS - 输出 MP3 24kHz 48kbps mono
func example1_EdgeTTS(ctx context.Context) {
	fmt.Println("\n=== 示例 1: EdgeTTS ===")

	provider := edgetts.New()

	// EdgeTTS 音质已由接口参数确定: audio-24khz-48kbitrate-mono-mp3
	// 无需设置任何音质参数，系统会自动使用 TTS 的原始质量
	opts := &edgetts.SynthesizeOptions{
		Text:  "这是一段测试文本，EdgeTTS 的音质是固定的。",
		Voice: "zh-CN-XiaoxiaoNeural",
	}

	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("✓ 生成音频: %d 字节\n", len(audio))
	fmt.Println("说明: EdgeTTS 输出格式为 MP3 24kHz 48kbps mono")
	fmt.Println("     这是 EdgeTTS API 的固定输出，无法更改")
}

// 示例 2: Volcengine - 输出 WAV 无损
func example2_Volcengine(ctx context.Context) {
	fmt.Println("\n=== 示例 2: Volcengine ===")

	provider := volcengine.New()

	// Volcengine 21种免费语音都输出 WAV 格式
	// 音质已通过代码测试确定，无需用户配置
	opts := &volcengine.SynthesizeOptions{
		Text:  "这是一段测试文本，Volcengine 输出无损 WAV 格式。",
		Voice: "BV700_streaming",
	}

	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("✓ 生成音频: %d 字节\n", len(audio))
	fmt.Println("说明: Volcengine 21种免费语音都输出 WAV 无损格式")
	fmt.Println("     音质: WAV 24kHz mono")
}

// 示例 3: 使用背景音乐 - 输出音质由 TTS 语音决定
func example3_BackgroundMusic(ctx context.Context) {
	fmt.Println("\n=== 示例 3: 背景音乐混音 ===")

	// EdgeTTS + 背景音乐
	fmt.Println("\n1. EdgeTTS + 背景音乐")
	edgeProvider := edgetts.New()
	edgeOpts := &edgetts.SynthesizeOptions{
		Text:  "EdgeTTS 测试",
		Voice: "zh-CN-XiaoxiaoNeural",
		BackgroundMusic: &tts.BackgroundMusicOptions{
			MusicPath: "background.mp3",
			Volume:    0.3,
			// 注意: 不需要设置任何音质参数
			// 输出音质会自动使用 EdgeTTS 的原始质量: MP3 24kHz 48kbps mono
		},
	}
	audioEdge, err := edgeProvider.Synthesize(ctx, edgeOpts)
	if err == nil {
		fmt.Printf("✓ EdgeTTS 混音: %d 字节\n", len(audioEdge))
		fmt.Println("  输出音质: MP3 24kHz 48kbps mono (与 TTS 语音一致)")
	}

	// Volcengine + 背景音乐
	fmt.Println("\n2. Volcengine + 背景音乐")
	volcProvider := volcengine.New()
	volcOpts := &volcengine.SynthesizeOptions{
		Text:  "Volcengine 测试",
		Voice: "BV700_streaming",
		BackgroundMusic: &tts.BackgroundMusicOptions{
			MusicPath: "background.mp3",
			Volume:    0.3,
			// 注意: 不需要设置任何音质参数
			// 输出音质会自动使用 Volcengine 的原始质量: WAV 24kHz mono
		},
	}
	audioVolc, err := volcProvider.Synthesize(ctx, volcOpts)
	if err == nil {
		fmt.Printf("✓ Volcengine 混音: %d 字节\n", len(audioVolc))
		fmt.Println("  输出音质: WAV 24kHz mono (与 TTS 语音一致)")
	}

	// 总结
	fmt.Println("\n📊 音质说明:")
	fmt.Println("┌──────────────┬─────────────────────┬────────────────────────┐")
	fmt.Println("│ Provider     │ TTS 语音音质        │ 混音输出音质           │")
	fmt.Println("├──────────────┼─────────────────────┼────────────────────────┤")
	fmt.Println("│ EdgeTTS      │ MP3 24kHz 48kbps    │ MP3 24kHz 48kbps       │")
	fmt.Println("│ Volcengine   │ WAV 24kHz mono      │ WAV 24kHz mono         │")
	fmt.Println("└──────────────┴─────────────────────┴────────────────────────┘")
	fmt.Println("\n重要: 混音输出音质完全由 TTS 语音决定，无需用户配置")
	fmt.Println("     背景音乐只是辅助，会自动适配 TTS 语音的质量")
}
