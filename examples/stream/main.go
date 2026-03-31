package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"

	"github.com/simp-lee/ttsbridge/providers/edgetts"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	// 创建 Edge TTS 提供商
	provider := edgetts.New()

	// 配置语音参数
	request := tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      "这是一个流式语音合成的示例。TTSBridge 可以实时获取音频数据块,非常适合需要低延迟的场景。",
		VoiceID:   "zh-CN-XiaoxiaoNeural",
	}

	log.Printf("正在流式合成语音...")

	// 流式合成语音
	ctx := context.Background()
	stream, err := provider.SynthesizeStream(ctx, request)
	if err != nil {
		log.Fatalf("合成失败: %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	// 创建输出文件
	outFile, err := os.Create("stream_output.mp3")
	if err != nil {
		log.Fatalf("创建文件失败: %v", err)
	}
	defer func() {
		_ = outFile.Close()
	}()

	// 逐块读取音频数据
	totalBytes := 0
	chunkCount := 0
	for {
		chunk, err := stream.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("读取流失败: %v", err)
		}

		// 写入文件
		n, err := outFile.Write(chunk)
		if err != nil {
			log.Fatalf("写入文件失败: %v", err)
		}
		if n != len(chunk) {
			log.Fatalf("写入文件不完整: %d/%d", n, len(chunk))
		}

		totalBytes += n
		chunkCount++
		log.Printf("接收音频块 #%d: %d 字节 (总计: %d 字节)", chunkCount, n, totalBytes)
	}

	log.Printf("流式语音合成完成! 共接收 %d 个音频块,总计 %d 字节", chunkCount, totalBytes)
	log.Printf("流式接口只返回音频块；如需 BoundaryEvents 请改用同步 Synthesize")
}
