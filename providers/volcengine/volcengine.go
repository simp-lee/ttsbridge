package volcengine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/simp-lee/retry"
	"github.com/simp-lee/ttsbridge/audio"
	"github.com/simp-lee/ttsbridge/tts"
	"github.com/simp-lee/ttsbridge/tts/textutils"
)

const (
	providerName        = "volcengine"
	defaultAPIURL       = "https://translate.volcengine.com/crx/tts/v1/" // 备用地址：https://translate.volcengine.com/web/tts/v1
	defaultMaxTextBytes = 1024
	defaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// SynthesizeOptions Volcengine 专属合成选项
type SynthesizeOptions struct {
	Text  string
	Voice string

	// Volcengine 不支持 Rate/Volume/Pitch 参数
	// 只支持固定的语音和语言

	// 背景音乐
	BackgroundMusic *tts.BackgroundMusicOptions
}

// Provider 火山翻译 TTS 提供商
type Provider struct {
	client           *http.Client
	baseURL          string
	maxTextBytes     int
	maxRetryAttempts int
}

type translateRequest struct {
	Text    string `json:"text"`
	Speaker string `json:"speaker"`
}

type translateResponse struct {
	BaseResp struct {
		StatusCode int `json:"status_code"` // 0=成功, 400=参数错误
	} `json:"base_resp"`
	Audio struct {
		Data string `json:"data"` // base64 编码的音频数据
	} `json:"audio"`
}

// New 创建火山翻译 TTS 提供商，使用默认配置
// 使用 With* 方法自定义配置：
//
//	provider := volcengine.New().
//	    WithBaseURL("https://...").
//	    WithHTTPTimeout(30*time.Second).
//	    WithMaxAttempts(3)
func New() *Provider {
	return &Provider{
		client:           &http.Client{Timeout: tts.DefaultHTTPTimeout},
		baseURL:          defaultAPIURL,
		maxTextBytes:     defaultMaxTextBytes,
		maxRetryAttempts: tts.DefaultMaxRetries,
	}
}

// WithHTTPTimeout 设置 HTTP 客户端超时
func (p *Provider) WithHTTPTimeout(timeout time.Duration) *Provider {
	p.client.Timeout = timeout
	return p
}

// WithBaseURL 设置 API 基础 URL
func (p *Provider) WithBaseURL(url string) *Provider {
	if url != "" {
		p.baseURL = url
	}
	return p
}

// WithMaxTextBytes 设置单次请求最大文本字节数
func (p *Provider) WithMaxTextBytes(maxBytes int) *Provider {
	if maxBytes > 0 {
		p.maxTextBytes = maxBytes
	}
	return p
}

// WithMaxAttempts 设置最大重试次数（包括首次尝试）
func (p *Provider) WithMaxAttempts(attempts int) *Provider {
	if attempts > 0 {
		p.maxRetryAttempts = attempts
	}
	return p
}

// WithProxy 设置代理 URL
func (p *Provider) WithProxy(proxyURL string) *Provider {
	if proxyURL != "" {
		if parsedURL, err := url.Parse(proxyURL); err == nil {
			p.client.Transport = &http.Transport{Proxy: http.ProxyURL(parsedURL)}
		}
	}
	return p
}

// Name 返回提供商名称
func (p *Provider) Name() string {
	return providerName
}

// Synthesize 同步合成语音
func (p *Provider) Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	if opts == nil || opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}

	if ctx == nil {
		ctx = context.Background()
	}

	cleanedText := textutils.CleanText(opts.Text, &textutils.CleanOptions{
		RemoveControlChars: true,
		TrimSpaces:         true,
	})
	if cleanedText == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after cleaning", Provider: providerName}
	}

	chunks := p.splitText(cleanedText)
	if len(chunks) == 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "failed to split text into chunks", Provider: providerName}
	}

	var voiceAudio []byte

	// 如果只有一个块，直接返回
	if len(chunks) == 1 {
		audio, err := p.synthesizeChunk(ctx, opts)
		if err != nil {
			return nil, err
		}
		voiceAudio = audio
	} else {
		// 多个块：收集所有 PCM 数据并重新构建 WAV
		var pcmData bytes.Buffer
		var firstHeader []byte

		for i, chunk := range chunks {
			chunkOpts := *opts
			chunkOpts.Text = chunk

			audioChunk, err := p.synthesizeChunk(ctx, &chunkOpts)
			if err != nil {
				return nil, err
			}

			if len(audioChunk) < 44 {
				// 音频数据太短，跳过（异常情况）
				continue
			}

			if i == 0 {
				// 保存第一个 WAV header（前44字节）
				firstHeader = make([]byte, 44)
				copy(firstHeader, audioChunk[:44])
				pcmData.Write(audioChunk[44:])
			} else {
				// 后续块：跳过 header，只取 PCM 数据
				pcmData.Write(audioChunk[44:])
			}
		}

		// 重建 WAV：更新 header 中的文件大小
		voiceAudio = rebuildWAV(firstHeader, pcmData.Bytes())
	}

	if opts.BackgroundMusic != nil && opts.BackgroundMusic.MusicPath != "" {
		mixedAudio, err := audio.MixWithBackgroundMusic(ctx, voiceAudio, providerName, opts.Voice, opts.BackgroundMusic)
		if err != nil {
			return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "background music mixing failed", Provider: providerName, Err: err}
		}
		voiceAudio = mixedAudio
	}

	return voiceAudio, nil
}

// truncateBody 截断 body 内容用于错误消息（最多 256 字符）
func truncateBody(body []byte) string {
	const maxLen = 256
	s := string(body)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// synthesizeChunk 合成单个文本块
func (p *Provider) synthesizeChunk(ctx context.Context, opts *SynthesizeOptions) ([]byte, error) {
	speaker := p.convertVoiceToSpeaker(opts.Voice)

	reqData := translateRequest{
		Text:    opts.Text,
		Speaker: speaker,
	}

	payload, err := json.Marshal(reqData)
	if err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  "failed to marshal request",
			Provider: providerName,
			Err:      err,
		}
	}

	var audioData []byte
	err = retry.Do(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(payload))
		if err != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to create request",
				Provider: providerName,
				Err:      err,
			}
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("Origin", "chrome-extension://klgfhbdadaspgppeadghjjemk")

		resp, err := p.client.Do(req)
		if err != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to send request",
				Provider: providerName,
				Err:      err,
			}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to read response",
				Provider: providerName,
				Err:      err,
			}
		}

		// 可重试的 HTTP 状态码（429 和 5xx）
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return &tts.Error{
				Code:     tts.ErrCodeProviderUnavail,
				Message:  fmt.Sprintf("retryable status %d: %s", resp.StatusCode, truncateBody(body)),
				Provider: providerName,
			}
		}

		// 非 200 状态码的其他错误（不可重试）
		if resp.StatusCode != http.StatusOK {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, truncateBody(body)),
				Provider: providerName,
			}
		}

		var apiResp translateResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  fmt.Sprintf("failed to parse response: %s", truncateBody(body)),
				Provider: providerName,
				Err:      err,
			}
		}

		// 参数错误（不可重试）
		if apiResp.BaseResp.StatusCode == 400 {
			return &tts.Error{
				Code:     tts.ErrCodeInvalidInput,
				Message:  "参数错误，不支持的发音人/文本",
				Provider: providerName,
			}
		}

		// 其他 API 错误（不可重试）
		if apiResp.BaseResp.StatusCode != 0 {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  fmt.Sprintf("API error: code=%d", apiResp.BaseResp.StatusCode),
				Provider: providerName,
			}
		}

		data, err := base64.StdEncoding.DecodeString(apiResp.Audio.Data)
		if err != nil {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "failed to decode audio data",
				Provider: providerName,
				Err:      err,
			}
		}

		audioData = data
		return nil
	}, tts.RetryOptions(ctx, p.maxRetryAttempts)...)

	if err != nil {
		if retry.IsRetryError(err) {
			return nil, &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  fmt.Sprintf("synthesis failed after %d attempts", p.maxRetryAttempts),
				Provider: providerName,
				Err:      err,
			}
		}
		return nil, err
	}

	return audioData, nil
}

// SynthesizeStream 流式合成语音
func (p *Provider) SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (tts.AudioStream, error) {
	audioData, err := p.Synthesize(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &audioStream{
		data:   audioData,
		offset: 0,
	}, nil
}

// ListVoices 列出可用的语音
func (p *Provider) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	allVoices := GetAllVoices()

	if locale == "" {
		return allVoices, nil
	}

	filteredVoices := make([]tts.Voice, 0)
	for _, voice := range allVoices {
		// 使用前缀匹配，与 EdgeTTS 保持一致
		if strings.HasPrefix(string(voice.Language), locale) {
			filteredVoices = append(filteredVoices, voice)
		}
	}

	return filteredVoices, nil
}

// IsAvailable 检查提供商是否可用
func (p *Provider) IsAvailable(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	// synthesizeChunk 内部已经有重试逻辑，这里不需要再次包装
	_, err := p.synthesizeChunk(ctx, &SynthesizeOptions{
		Text:  "测试",
		Voice: "BV700_streaming",
	})

	return err == nil
}

// convertVoiceToSpeaker 将语音 ID 转换为 speaker 参数
// API 需要带 tts.other. 前缀的格式
func (p *Provider) convertVoiceToSpeaker(voice string) string {
	if voice == "" {
		return "tts.other.BV700_streaming" // 默认：灿灿
	}

	// 如果已经带前缀，直接返回
	if strings.HasPrefix(voice, "tts.other.") {
		return voice
	}

	// 如果是 BV 开头的标准格式，添加前缀
	if strings.HasPrefix(voice, "BV") && strings.HasSuffix(voice, "_streaming") {
		return "tts.other." + voice
	}

	// 其他格式保持不变（如旧的 zh_female_zhubo 格式）
	return voice
}

func (p *Provider) splitText(cleanedText string) []string {
	if cleanedText == "" {
		return nil
	}

	if len([]byte(cleanedText)) <= p.maxTextBytes {
		return []string{cleanedText}
	}

	return textutils.SplitByByteLength(cleanedText, &textutils.SplitOptions{
		MaxBytes:             p.maxTextBytes,
		PreserveHTMLEntities: false,
	})
}

// audioStream 音频流
type audioStream struct {
	data   []byte
	offset int
	closed bool
}

func (s *audioStream) Read() ([]byte, error) {
	if s.closed {
		return nil, io.EOF
	}

	if s.offset >= len(s.data) {
		s.closed = true
		return nil, io.EOF
	}

	chunkSize := 4096
	remaining := len(s.data) - s.offset
	if remaining < chunkSize {
		chunkSize = remaining
	}

	chunk := s.data[s.offset : s.offset+chunkSize]
	s.offset += chunkSize

	return chunk, nil
}

func (s *audioStream) Close() error {
	s.closed = true
	return nil
}

// rebuildWAV 重建 WAV 文件，更新 header 中的数据大小
func rebuildWAV(header []byte, pcmData []byte) []byte {
	if len(header) < 44 {
		// header 不完整，直接返回 PCM 数据
		return pcmData
	}

	// 复制 header
	newHeader := make([]byte, 44)
	copy(newHeader, header)

	// 计算新的文件大小
	dataSize := len(pcmData)
	fileSize := dataSize + 36 // 不含 RIFF header 的 8 字节

	// 更新 RIFF chunk size (字节 4-7)
	newHeader[4] = byte(fileSize & 0xff)
	newHeader[5] = byte((fileSize >> 8) & 0xff)
	newHeader[6] = byte((fileSize >> 16) & 0xff)
	newHeader[7] = byte((fileSize >> 24) & 0xff)

	// 更新 data chunk size (字节 40-43)
	newHeader[40] = byte(dataSize & 0xff)
	newHeader[41] = byte((dataSize >> 8) & 0xff)
	newHeader[42] = byte((dataSize >> 16) & 0xff)
	newHeader[43] = byte((dataSize >> 24) & 0xff)

	// 拼接 header 和 PCM 数据
	result := make([]byte, 0, 44+dataSize)
	result = append(result, newHeader...)
	result = append(result, pcmData...)

	return result
}
