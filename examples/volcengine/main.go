package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	fmt.Println("=== 火山引擎 TTS 21款免费音色测试 ===")
	fmt.Println()

	// 创建火山翻译 TTS Provider（无需 APP_ID 和 ACCESS_TOKEN）
	provider := volcengine.New()

	ctx := context.Background()

	// 示例 1: 列出所有可用语音
	fmt.Println("=== 示例 1: 列出所有可用语音 ===")
	listVoices(ctx, provider)

	// 示例 2: 测试所有21款免费音色
	fmt.Println("\n=== 示例 2: 测试所有21款免费音色 ===")
	testAll21Voices(ctx, provider)

	fmt.Println("\n所有示例执行完成！")
	fmt.Println("\n注意：基于火山引擎服务，完全免费，无需 APP_ID 和 ACCESS_TOKEN")
}

// listVoices 列出所有可用语音
func listVoices(ctx context.Context, provider *volcengine.Provider) {
	voices, err := provider.ListVoices(ctx, "")
	if err != nil {
		log.Printf("获取语音列表失败: %v", err)
		return
	}

	fmt.Printf("找到 %d 个语音\n", len(voices))

	// 按语言分类展示
	locales := make(map[string][]tts.Voice)
	for _, v := range voices {
		locales[string(v.Language)] = append(locales[string(v.Language)], v)
	}

	for locale, voiceList := range locales {
		fmt.Printf("\n【%s】 (%d 个)\n", locale, len(voiceList))
		for _, v := range voiceList {
			fmt.Printf("  - %s (%s) [%s]\n", v.Name, v.Gender, v.ID)
		}
	}
}

// testAll21Voices 测试所有21款免费音色
func testAll21Voices(ctx context.Context, provider *volcengine.Provider) {
	// 定义所有21款免费音色的测试数据
	testVoices := []struct {
		category string
		id       string
		name     string
		text     string
	}{
		// 通用场景 (3款)
		{"通用场景", "BV700_streaming", "灿灿", "大家好，我是灿灿，一个温柔的女声。"},
		{"通用场景", "BV001_streaming", "通用女声", "您好，我是通用女声，适合各种场景使用。"},
		{"通用场景", "BV002_streaming", "通用男声", "您好，我是通用男声，声音稳重大气。"},

		// 有声阅读 (5款)
		{"有声阅读", "BV701_streaming", "擎苍", "我是擎苍，适合有声阅读和故事讲述。"},
		{"有声阅读", "BV119_streaming", "通用赞婚", "欢迎大家，我是通用赞婚音色。"},
		{"有声阅读", "BV102_streaming", "儒雅青年", "我是儒雅青年，声音温文尔雅。"},
		{"有声阅读", "BV113_streaming", "甜宠少御", "我是甜宠少御，声音甜美可爱。"},
		{"有声阅读", "BV115_streaming", "古风少御", "我是古风少御，带有古典韵味。"},

		// 智能助手/视频配音/特色/教育 (6款)
		{"智能助手", "BV007_streaming", "亲切女声", "您好，我是亲切女声，适合客服和助手场景。"},
		{"智能助手", "BV056_streaming", "阳光男声", "大家好，我是阳光男声，充满活力。"},
		{"智能助手", "BV005_streaming", "活泼女声", "嗨，我是活泼女声，元气满满！"},
		{"特色音色", "BV051_streaming", "奶气萌娃", "大家好呀，我是奶气萌娃，声音萌萌哒！"},
		{"特色音色", "BV034_streaming", "知性姐姐", "Hello，我是知性姐姐，支持中英文双语。"},
		{"特色音色", "BV033_streaming", "温柔小哥", "大家好，我是温柔小哥，声音温暖。"},

		// 方言 (3款)
		{"方言", "BV021_streaming", "东北老铁", "老铁们好啊，俺是东北老铁，说话就是这么豪爽！"},
		{"方言", "BV019_streaming", "重庆小伙", "各位朋友好，我是重庆小伙，声音很有特色。"},
		{"方言", "BV213_streaming", "广西表哥", "大家好，我是广西表哥，欢迎来广西玩。"},

		// 英语 (2款)
		{"英语", "BV503_streaming", "Ariana", "Hello, I'm Ariana, a lively female voice for English."},
		{"英语", "BV504_streaming", "Jackson", "Hi there, I'm Jackson, an energetic male voice."},

		// 日语 (2款)
		{"日语", "BV522_streaming", "气质女生", "こんにちは、気品のある女性の声です。"},
		{"日语", "BV524_streaming", "日语男声", "こんにちは、日本語の男性の声です。"},
	}

	successCount := 0
	failCount := 0
	totalSize := 0

	fmt.Printf("开始测试 %d 款音色...\n\n", len(testVoices))

	currentCategory := ""
	for i, tv := range testVoices {
		// 打印分类标题
		if tv.category != currentCategory {
			currentCategory = tv.category
			fmt.Printf("\n【%s】\n", currentCategory)
		}

		fmt.Printf("%2d. 测试 %s (%s)... ", i+1, tv.name, tv.id)

		opts := &volcengine.SynthesizeOptions{
			Text:  tv.text,
			Voice: tv.id,
		}

		audio, err := provider.Synthesize(ctx, opts)
		if err != nil {
			fmt.Printf("✗ 失败: %v\n", err)
			failCount++
			continue
		}

		// 保存音频文件
		outputFile := fmt.Sprintf("voice_%02d_%s.mp3", i+1, tv.id)
		if err := os.WriteFile(outputFile, audio, 0644); err != nil {
			fmt.Printf("✗ 保存失败: %v\n", err)
			failCount++
			continue
		}

		totalSize += len(audio)
		fmt.Printf("✓ 成功 (%d bytes)\n", len(audio))
		successCount++
	}

	// 统计结果
	fmt.Printf("\n" + "===========================================\n")
	fmt.Printf("测试完成！\n")
	fmt.Printf("成功: %d / %d\n", successCount, len(testVoices))
	fmt.Printf("失败: %d / %d\n", failCount, len(testVoices))
	fmt.Printf("总音频大小: %.2f MB\n", float64(totalSize)/(1024*1024))
	fmt.Printf("===========================================\n")

	if failCount > 0 {
		fmt.Printf("\n⚠️  有 %d 个音色测试失败，请检查错误信息\n", failCount)
	} else {
		fmt.Printf("\n🎉 所有21款音色测试通过！\n")
	}
}
