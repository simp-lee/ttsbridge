package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	// 创建 Edge TTS 提供商，配置接收超时为 90 秒
	provider := edgetts.New().WithReceiveTimeout(90 * time.Second)

	// 创建一个超长文本（超过 4096 字节）
	longText := strings.Repeat("这是一段用于测试长文本分块功能的内容。", 200) +
		strings.Repeat("This is a test for long text chunking. ", 200)

	fmt.Printf("文本长度: %d 字节\n", len(longText))

	// 测试同步合成（带元数据）
	fmt.Println("\n=== 测试同步合成（带元数据回调） ===")
	ctx := context.Background()

	var wordCount int
	var sentenceCount int

	opts := &edgetts.SynthesizeOptions{
		Text:                    longText,
		Voice:                   "zh-CN-XiaoxiaoNeural",
		Rate:                    1.0,
		Volume:                  1.0,
		Pitch:                   1.0,
		WordBoundaryEnabled:     true,
		SentenceBoundaryEnabled: true,
		BoundaryCallback: func(event tts.BoundaryEvent) {
			switch event.Type {
			case "WordBoundary":
				wordCount++
			case "SentenceBoundary":
				sentenceCount++
			}
			// 只打印前几个示例
			if wordCount+sentenceCount <= 5 {
				fmt.Printf("  [%s] Offset: %dms, Duration: %dms, Text: %s\n",
					event.Type, event.OffsetMs, event.DurationMs, event.Text)
			}
		},
	}

	audioData, err := provider.Synthesize(ctx, opts)
	if err != nil {
		log.Fatalf("同步合成失败: %v", err)
	}

	fmt.Printf("同步合成完成，音频大小: %d 字节\n", len(audioData))
	fmt.Printf("词边界数量: %d, 句边界数量: %d\n", wordCount, sentenceCount)

	// 保存音频文件
	outputFile := "long_text_sync.mp3"
	if err := os.WriteFile(outputFile, audioData, 0644); err != nil {
		log.Fatalf("保存音频失败: %v", err)
	}
	fmt.Printf("音频已保存到: %s\n", outputFile)

	// 测试流式合成
	fmt.Println("\n=== 测试流式合成 ===")
	wordCount = 0
	sentenceCount = 0

	streamOpts := &edgetts.SynthesizeOptions{
		Text:                    longText,
		Voice:                   "zh-CN-XiaoxiaoNeural",
		Rate:                    1.0,
		Volume:                  1.0,
		Pitch:                   1.0,
		WordBoundaryEnabled:     true,
		SentenceBoundaryEnabled: true,
		BoundaryCallback: func(event tts.BoundaryEvent) {
			switch event.Type {
			case "WordBoundary":
				wordCount++
			case "SentenceBoundary":
				sentenceCount++
			}
		},
	}

	stream, err := provider.SynthesizeStream(ctx, streamOpts)
	if err != nil {
		log.Fatalf("流式合成失败: %v", err)
	}
	defer stream.Close()

	var streamAudioData []byte
	chunkCount := 0
	for {
		chunk, err := stream.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("读取流式音频失败: %v", err)
		}
		streamAudioData = append(streamAudioData, chunk...)
		chunkCount++
	}

	fmt.Printf("流式合成完成，接收 %d 个音频块，总大小: %d 字节\n", chunkCount, len(streamAudioData))
	fmt.Printf("词边界数量: %d, 句边界数量: %d\n", wordCount, sentenceCount)

	// 保存流式音频文件
	streamOutputFile := "long_text_stream.mp3"
	if err := os.WriteFile(streamOutputFile, streamAudioData, 0644); err != nil {
		log.Fatalf("保存流式音频失败: %v", err)
	}
	fmt.Printf("流式音频已保存到: %s\n", streamOutputFile)

	fmt.Println("\n✓ 所有测试完成！")
}
