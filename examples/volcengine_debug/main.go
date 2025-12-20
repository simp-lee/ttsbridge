package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	defaultAPIURL    = "https://translate.volcengine.com/crx/tts/v1/"
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type translateRequest struct {
	Text     string `json:"text"`
	Speaker  string `json:"speaker"`
	Language string `json:"language,omitempty"`
}

type translateResponse struct {
	BaseResp struct {
		StatusCode    int    `json:"status_code"`
		StatusMessage string `json:"status_message"`
	} `json:"base_resp"`
	Audio struct {
		Data     string `json:"data"`
		Duration int    `json:"duration"`
	} `json:"audio"`
}

func main() {
	// 命令行参数
	text := flag.String("text", "你好，这是一个测试。", "要合成的文本")
	voice := flag.String("voice", "zh_female_zhubo", "语音ID")
	listVoices := flag.Bool("list", false, "列出所有可用的语音")
	testAPI := flag.Bool("test", false, "测试 API 可用性")
	verbose := flag.Bool("verbose", false, "显示详细的 HTTP 交互信息")
	saveAudio := flag.String("save", "", "保存音频到文件 (例如: output.mp3)")

	flag.Parse()

	// 列出所有语音
	if *listVoices {
		printAllVoices()
		return
	}

	// 测试 API
	if *testAPI {
		testAPIAvailability()
		return
	}

	// 调试请求
	debugRequest(*text, *voice, *verbose, *saveAudio)
}

func printAllVoices() {
	voices := volcengine.GetAllVoices()

	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Printf("所有可用的语音 (共 %d 个)\n", len(voices))
	fmt.Println("=" + strings.Repeat("=", 79))

	// 按语言分组
	localeMap := make(map[string][]tts.Voice)
	for _, v := range voices {
		localeMap[string(v.Language)] = append(localeMap[string(v.Language)], v)
	}

	for locale, voiceList := range localeMap {
		fmt.Printf("\n语言: %s\n", locale)
		fmt.Println(strings.Repeat("-", 80))
		for _, v := range voiceList {
			fmt.Printf("  %-25s  %-30s  %s\n", v.ID, v.Name, v.Gender)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("使用示例:")
	fmt.Println("  go run main.go -text \"你好\" -voice zh_female_zhubo")
	fmt.Println("  go run main.go -text \"Hello\" -voice en_male_adam")
	fmt.Println("=" + strings.Repeat("=", 79))
}

func testAPIAvailability() {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("测试火山翻译 API 可用性")
	fmt.Println("=" + strings.Repeat("=", 79))

	provider := volcengine.New()

	testCases := []struct {
		text  string
		voice string
	}{
		{"测试", "zh_female_zhubo"},
		{"Hello", "en_male_adam"},
		{"こんにちは", "ja_female_risa"},
	}

	for _, tc := range testCases {
		fmt.Printf("\n测试: %s (语音: %s) ... ", tc.text, tc.voice)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		opts := &volcengine.SynthesizeOptions{
			Text:  tc.text,
			Voice: tc.voice,
		}

		_, err := provider.Synthesize(ctx, opts)
		if err != nil {
			fmt.Printf("❌ 失败: %v\n", err)
		} else {
			fmt.Println("✓ 成功")
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
}

func debugRequest(text, voice string, verbose bool, saveAudioPath string) {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("火山翻译 API 请求调试工具")
	fmt.Println("=" + strings.Repeat("=", 79))

	// 转换语音 ID 到 speaker
	speaker := convertVoiceToSpeaker(voice)

	// 构造请求
	reqData := translateRequest{
		Text:    text,
		Speaker: speaker,
	}

	payload, err := json.Marshal(reqData)
	if err != nil {
		fmt.Printf("❌ 序列化请求失败: %v\n", err)
		os.Exit(1)
	}

	// 打印请求信息
	fmt.Println("\n【请求信息】")
	fmt.Println("URL:", defaultAPIURL)
	fmt.Println("Method: POST")
	fmt.Println("\nHeaders:")
	fmt.Println("  Content-Type: application/json")
	fmt.Println("  Accept: application/json")
	fmt.Println("  User-Agent:", defaultUserAgent)

	fmt.Println("\n请求 Body (JSON):")
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, payload, "", "  "); err != nil {
		fmt.Println(string(payload))
	} else {
		fmt.Println(prettyJSON.String())
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, defaultAPIURL, bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("❌ 创建请求失败: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	// 如果需要详细信息，打印完整的 HTTP 请求
	if verbose {
		fmt.Println("\n【完整 HTTP 请求报文】")
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			fmt.Printf("无法导出请求: %v\n", err)
		} else {
			fmt.Println(string(requestDump))
		}
	}

	// 发送请求
	fmt.Println("\n正在发送请求...")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ 发送请求失败: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ 读取响应失败: %v\n", err)
		os.Exit(1)
	}

	// 打印响应信息
	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("【响应信息】")
	fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)

	if verbose {
		fmt.Println("\nResponse Headers:")
		for key, values := range resp.Header {
			for _, value := range values {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
	}

	fmt.Println("\n响应 Body (原始 JSON):")
	fmt.Println(string(body))

	// 解析响应
	var apiResp translateResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		fmt.Printf("\n❌ 解析响应 JSON 失败: %v\n", err)
		os.Exit(1)
	}

	// 格式化打印
	fmt.Println("\n响应 Body (格式化 JSON):")
	prettyResp, _ := json.MarshalIndent(apiResp, "", "  ")
	fmt.Println(string(prettyResp))

	// 打印关键信息
	fmt.Println("\n" + strings.Repeat("-", 80))
	fmt.Println("【关键信息】")
	fmt.Printf("状态码: %d\n", apiResp.BaseResp.StatusCode)
	fmt.Printf("状态消息: %s\n", apiResp.BaseResp.StatusMessage)

	if apiResp.BaseResp.StatusCode == 0 {
		fmt.Printf("音频时长: %d 毫秒 (%.2f 秒)\n",
			apiResp.Audio.Duration,
			float64(apiResp.Audio.Duration)/1000)
		fmt.Printf("音频数据长度 (Base64): %d 字符\n", len(apiResp.Audio.Data))

		// 解码音频数据
		if apiResp.Audio.Data != "" {
			audioData, err := base64.StdEncoding.DecodeString(apiResp.Audio.Data)
			if err != nil {
				fmt.Printf("❌ 解码音频数据失败: %v\n", err)
			} else {
				fmt.Printf("音频数据大小 (解码后): %d 字节 (%.2f KB)\n",
					len(audioData),
					float64(len(audioData))/1024)

				// 保存音频文件
				if saveAudioPath != "" {
					if err := os.WriteFile(saveAudioPath, audioData, 0644); err != nil {
						fmt.Printf("❌ 保存音频文件失败: %v\n", err)
					} else {
						fmt.Printf("✓ 音频已保存到: %s\n", saveAudioPath)
					}
				}
			}
		}

		fmt.Println("\n✓ 请求成功!")
	} else {
		fmt.Printf("\n❌ API 返回错误: code=%d, message=%s\n",
			apiResp.BaseResp.StatusCode,
			apiResp.BaseResp.StatusMessage)
	}

	fmt.Println("=" + strings.Repeat("=", 79))
}

func convertVoiceToSpeaker(voice string) string {
	if voice == "" {
		return "tts.other.BV700_streaming"
	}

	if strings.HasPrefix(voice, "tts.other.") {
		return voice
	}

	if !strings.HasPrefix(voice, "BV") {
		return voice
	}

	if strings.HasPrefix(voice, "BV") && strings.HasSuffix(voice, "_streaming") {
		return "tts.other." + voice
	}

	return fmt.Sprintf("tts.other.BV%s_streaming", voice)
}
