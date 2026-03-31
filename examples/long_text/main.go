package main

import (
	"context"
	"errors"
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
	fmt.Println("\n=== 测试同步合成（读取 BoundaryEvents） ===")
	ctx := context.Background()

	request := tts.SynthesisRequest{
		InputMode:          tts.InputModePlainText,
		Text:               longText,
		VoiceID:            "zh-CN-XiaoxiaoNeural",
		NeedBoundaryEvents: true,
	}

	result, err := provider.Synthesize(ctx, request)
	if err != nil {
		log.Fatalf("同步合成失败: %v", err)
	}
	wordCount := 0
	sentenceCount := 0
	for _, event := range result.BoundaryEvents {
		switch event.Type {
		case tts.BoundaryTypeWord:
			wordCount++
		case tts.BoundaryTypeSentence:
			sentenceCount++
		}
		if wordCount+sentenceCount <= 5 {
			fmt.Printf("  [chunk=%d %s] Offset: %dms, Duration: %dms, Text: %s\n",
				event.ChunkIndex, event.Type, event.OffsetMs, event.DurationMs, event.Text)
		}
	}

	fmt.Printf("同步合成完成，音频大小: %d 字节\n", len(result.Audio))
	fmt.Printf("词边界数量: %d, 句边界数量: %d\n", wordCount, sentenceCount)

	// 保存音频文件
	outputFile := "long_text_sync.mp3"
	if writeErr := os.WriteFile(outputFile, result.Audio, 0o600); writeErr != nil {
		log.Fatalf("保存音频失败: %v", writeErr)
	}
	fmt.Printf("音频已保存到: %s\n", outputFile)

	// 测试流式合成
	fmt.Println("\n=== 测试流式合成 ===")
	streamRequest := tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      longText,
		VoiceID:   "zh-CN-XiaoxiaoNeural",
	}

	stream, err := provider.SynthesizeStream(ctx, streamRequest)
	if err != nil {
		log.Fatalf("流式合成失败: %v", err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			log.Printf("关闭流失败: %v", closeErr)
		}
	}()

	var streamAudioData []byte
	chunkCount := 0
	for {
		chunk, err := stream.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Fatalf("读取流式音频失败: %v", err)
		}
		streamAudioData = append(streamAudioData, chunk...)
		chunkCount++
	}

	fmt.Printf("流式合成完成，接收 %d 个音频块，总大小: %d 字节\n", chunkCount, len(streamAudioData))
	fmt.Println("流式接口仅返回音频块；边界事件请改用同步 Synthesize 读取结果中的 BoundaryEvents")

	// 保存流式音频文件
	streamOutputFile := "long_text_stream.mp3"
	if writeErr := os.WriteFile(streamOutputFile, streamAudioData, 0o600); writeErr != nil {
		log.Fatalf("保存流式音频失败: %v", writeErr)
	}
	fmt.Printf("流式音频已保存到: %s\n", streamOutputFile)

	fmt.Println("\n✓ 所有测试完成！")
}
