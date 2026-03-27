package main

import (
	"context"
	"fmt"
	"log"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/providers/volcengine"
)

func main() {
	ctx := context.Background()

	// 示例 1: 查询 Provider 支持的输出格式列表
	example1_DiscoverFormats()

	// 示例 2: 使用 OutputFormat 选择输出格式
	example2_SelectOutputFormat(ctx)
}

// 示例 1: 通过 OutputOptions() 查看推荐输出格式目录
func example1_DiscoverFormats() {
	fmt.Println("=== 示例 1: 查看推荐输出格式目录 ===")

	// EdgeTTS: OutputOptions 提供推荐目录，不代表当前环境已经 probe 验证。
	fmt.Println("\nEdgeTTS 推荐输出格式目录:")
	edgeProvider := edgetts.New()
	for _, opt := range edgeProvider.OutputOptions() {
		defaultMark := ""
		if opt.IsDefault {
			defaultMark = " (默认)"
		}
		fmt.Printf("  %-45s %-25s %s%s\n", opt.FormatID, opt.Label, opt.Description, defaultMark)
	}
	fmt.Println("  提示: 如需得到当前环境已验证可用的格式，请先调用 FormatRegistry().ProbeAll(ctx)，再读取 SupportedFormats().")

	// Volcengine: 固定 WAV 无损
	fmt.Println("\nVolcengine 支持的输出格式:")
	volcProvider := volcengine.New()
	for _, opt := range volcProvider.OutputOptions() {
		fmt.Printf("  %-45s %-25s %s\n", opt.FormatID, opt.Label, opt.Description)
	}
}

// 示例 2: 使用 FormatID 选择输出格式
func example2_SelectOutputFormat(ctx context.Context) {
	fmt.Println("\n=== 示例 2: 选择输出格式 ===")

	provider := edgetts.New()

	// 使用高音质格式: MP3 48kHz 192kbps
	opts := &edgetts.SynthesizeOptions{
		Text:         "这是一段使用高音质输出的测试文本。",
		Voice:        "zh-CN-XiaoxiaoNeural",
		OutputFormat: edgetts.OutputFormatMP3_48khz_192k,
	}

	audio, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("MP3 48kHz 192kbps 生成音频: %d 字节\n", len(audio))

	// 对比: 使用默认格式 (不设置 OutputFormat)
	optsDefault := &edgetts.SynthesizeOptions{
		Text:  "这是一段使用默认格式的测试文本。",
		Voice: "zh-CN-XiaoxiaoNeural",
	}

	audioDefault, err := provider.Synthesize(ctx, optsDefault)
	if err != nil {
		log.Printf("合成失败: %v", err)
		return
	}

	fmt.Printf("默认格式 (MP3 24kHz 48kbps) 生成音频: %d 字节\n", len(audioDefault))
	fmt.Printf("高音质比默认大约 %.1f 倍\n", float64(len(audio))/float64(len(audioDefault)))
}
