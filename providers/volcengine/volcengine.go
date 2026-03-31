package volcengine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/simp-lee/retry"
	"github.com/simp-lee/ttsbridge/tts"
	"github.com/simp-lee/ttsbridge/tts/textutils"
)

const (
	providerName         = "volcengine"
	defaultAPIURL        = "https://translate.volcengine.com/crx/tts/v1/" // 备用地址：https://translate.volcengine.com/web/tts/v1
	defaultMaxTextBytes  = 1024
	defaultUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	maxResponseBodyBytes = 10 << 20 // 10 MB limit for HTTP response body
)

// synthesizeOptions stores provider-native Volcengine request fields.
type synthesizeOptions struct {
	Text  string
	Voice string

	// Volcengine 不支持 Rate/Volume/Pitch 参数
	// 只支持固定的语音和语言

	ProgressCallback func(completed, total int) // 合成进度回调
}

// Provider 火山翻译 TTS 提供商
type Provider struct {
	client           *http.Client
	baseURL          string
	maxTextBytes     int
	maxRetryAttempts int
	formatRegistry   *tts.FormatRegistry
	baseURLErr       error
	proxyErr         error
}

var _ tts.Provider = (*Provider)(nil)

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
		formatRegistry:   newDefaultFormatRegistry(),
	}
}

// WithHTTPTimeout 设置 HTTP 客户端超时
func (p *Provider) WithHTTPTimeout(timeout time.Duration) *Provider {
	p.client.Timeout = timeout
	return p
}

// WithBaseURL 设置 API 基础 URL
func (p *Provider) WithBaseURL(url string) *Provider {
	if url == "" {
		return p
	}

	parsedURL, err := urlpkg.Parse(url)
	if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		p.baseURLErr = &tts.Error{
			Code:     tts.ErrCodeInvalidInput,
			Message:  fmt.Sprintf("invalid base URL %q", url),
			Provider: providerName,
			Err:      err,
		}
		return p
	}

	p.baseURL = url
	p.baseURLErr = nil
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
	if proxyURL == "" {
		p.client.Transport = nil
		p.proxyErr = nil
		return p
	}

	parsedURL, err := urlpkg.Parse(proxyURL)
	if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		p.client.Transport = nil
		p.proxyErr = &tts.Error{
			Code:     tts.ErrCodeInvalidInput,
			Message:  fmt.Sprintf("invalid proxy URL %q", proxyURL),
			Provider: providerName,
			Err:      err,
		}
		return p
	}

	p.client.Transport = &http.Transport{Proxy: http.ProxyURL(parsedURL)}
	p.proxyErr = nil
	return p
}

// Name 返回提供商名称
func (p *Provider) Name() string {
	return providerName
}

// FormatRegistry returns the provider's format registry.
func (p *Provider) FormatRegistry() *tts.FormatRegistry {
	if p.formatRegistry == nil {
		p.formatRegistry = newDefaultFormatRegistry()
	}
	return p.formatRegistry
}

// SupportedFormats returns all formats verified as available in the registry.
func (p *Provider) SupportedFormats() []tts.OutputFormat {
	return p.FormatRegistry().Available()
}

func (p *Provider) synthesizeOptions(ctx context.Context, opts *synthesizeOptions) ([]byte, error) {
	if opts == nil || opts.Text == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty", Provider: providerName}
	}
	if err := p.runtimeConfigError(); err != nil {
		return nil, err
	}
	ctx = normalizeVolcengineContext(ctx)
	chunks, err := splitSynthesisChunks(opts.Text, p.splitText)
	if err != nil {
		return nil, err
	}
	voiceAudio, err := p.synthesizeChunks(ctx, opts, chunks)
	if err != nil {
		return nil, err
	}
	return voiceAudio, nil
}

// Synthesize 同步合成语音
func (p *Provider) Synthesize(ctx context.Context, request tts.SynthesisRequest) (*tts.SynthesisResult, error) {
	opts, voiceID, err := p.buildRequest(request)
	if err != nil {
		return nil, err
	}
	audio, err := p.synthesizeOptions(ctx, opts)
	if err != nil {
		return nil, err
	}
	return buildResult(audio, voiceID)
}

func normalizeVolcengineContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func splitSynthesisChunks(text string, split func(string) []string) ([]string, error) {
	cleanedText := textutils.CleanText(text, &textutils.CleanOptions{
		RemoveControlChars: true,
		TrimSpaces:         true,
	})
	if cleanedText == "" {
		return nil, &tts.Error{Code: tts.ErrCodeInvalidInput, Message: "text cannot be empty after cleaning", Provider: providerName}
	}
	chunks := split(cleanedText)
	if len(chunks) == 0 {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "failed to split text into chunks", Provider: providerName}
	}
	return chunks, nil
}

func (p *Provider) synthesizeChunks(ctx context.Context, opts *synthesizeOptions, chunks []string) ([]byte, error) {
	if len(chunks) == 1 {
		return p.synthesizeSingleChunk(ctx, opts, chunks[0])
	}
	return p.synthesizeMultipleChunks(ctx, opts, chunks)
}

func (p *Provider) synthesizeSingleChunk(ctx context.Context, opts *synthesizeOptions, chunk string) ([]byte, error) {
	chunkOpts := *opts
	chunkOpts.Text = chunk
	audioData, _, err := p.synthesizeChunkWithWAVValidation(ctx, &chunkOpts, 0, false)
	if err != nil {
		return nil, err
	}
	if opts.ProgressCallback != nil {
		opts.ProgressCallback(1, 1)
	}
	return audioData, nil
}

func (p *Provider) synthesizeMultipleChunks(ctx context.Context, opts *synthesizeOptions, chunks []string) ([]byte, error) {
	var pcmData bytes.Buffer
	var firstHeader []byte
	var firstProfile wavChunkProfile

	for i, chunk := range chunks {
		chunkOpts := *opts
		chunkOpts.Text = chunk

		audioChunk, profile, err := p.synthesizeChunkWithWAVValidation(ctx, &chunkOpts, i, firstHeader != nil)
		if err != nil {
			return nil, wrapChunkError(err, i, len(chunks))
		}
		if firstHeader == nil {
			firstProfile = profile
		} else if !firstProfile.matches(profile) {
			return nil, wrapChunkError(&tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "wav profile mismatch with first chunk",
				Provider: providerName,
			}, i, len(chunks))
		}
		firstHeader = appendPCMChunk(&pcmData, firstHeader, audioChunk)
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(i+1, len(chunks))
		}
	}
	if firstHeader == nil {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "no valid wav chunk header found", Provider: providerName}
	}
	rebuilt, err := rebuildWAV(firstHeader, pcmData.Bytes())
	if err != nil {
		return nil, &tts.Error{Code: tts.ErrCodeInternalError, Message: "rebuilt wav exceeds format limits", Provider: providerName, Err: err}
	}
	return rebuilt, nil
}

func appendPCMChunk(pcmData *bytes.Buffer, firstHeader, audioChunk []byte) []byte {
	if firstHeader == nil {
		firstHeader = make([]byte, 44)
		copy(firstHeader, audioChunk[:44])
	}
	_, _ = pcmData.Write(audioChunk[44:])
	return firstHeader
}

func (p *Provider) synthesizeChunkWithWAVValidation(ctx context.Context, opts *synthesizeOptions, chunkIndex int, hasValidChunk bool) ([]byte, wavChunkProfile, error) {
	audioChunk, err := p.synthesizeChunk(ctx, opts)
	if err != nil {
		return nil, wavChunkProfile{}, err
	}

	_, _, profile, ok := parseCanonicalWAV(audioChunk)
	if !ok {
		if !hasValidChunk {
			return nil, wavChunkProfile{}, &tts.Error{Code: tts.ErrCodeInternalError, Message: "no valid wav chunk header found", Provider: providerName}
		}

		return nil, wavChunkProfile{}, &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  fmt.Sprintf("invalid wav header for chunk %d", chunkIndex+1),
			Provider: providerName,
		}
	}

	return audioChunk, profile, nil
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
func (p *Provider) synthesizeChunk(ctx context.Context, opts *synthesizeOptions) ([]byte, error) {
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
		httpReq, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(payload))
		if requestErr != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to create request",
				Provider: providerName,
				Err:      requestErr,
			}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/plain, */*")
		httpReq.Header.Set("User-Agent", defaultUserAgent)
		httpReq.Header.Set("Origin", "chrome-extension://klgfhbdadaspgppeadghjjemk")

		resp, doErr := p.client.Do(httpReq)
		if doErr != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to send request",
				Provider: providerName,
				Err:      doErr,
			}
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
		if readErr != nil {
			return &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  "failed to read response",
				Provider: providerName,
				Err:      readErr,
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
		if unmarshalErr := json.Unmarshal(body, &apiResp); unmarshalErr != nil {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  fmt.Sprintf("failed to parse response: %s", truncateBody(body)),
				Provider: providerName,
				Err:      unmarshalErr,
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

		data, decodeErr := base64.StdEncoding.DecodeString(apiResp.Audio.Data)
		if decodeErr != nil {
			return &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "failed to decode audio data",
				Provider: providerName,
				Err:      decodeErr,
			}
		}

		audioData = data
		return nil
	}, tts.RetryOptions(ctx, p.maxRetryAttempts)...)
	if err != nil {
		if retry.IsRetryError(err) {
			cause := unwrapRetryCause(err)
			var ttsErr *tts.Error
			if errors.As(cause, &ttsErr) {
				return nil, cause
			}
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
func (p *Provider) SynthesizeStream(ctx context.Context, request tts.SynthesisRequest) (tts.AudioStream, error) {
	if err := request.ValidateStreamAgainst(providerName, p.Capabilities()); err != nil {
		return nil, err
	}

	return nil, &tts.Error{Code: tts.ErrCodeUnsupportedCapability, Message: "streaming synthesis is not supported by provider", Provider: providerName}
}

// ListVoices 列出可用的语音
func (p *Provider) ListVoices(ctx context.Context, filter tts.VoiceFilter) ([]tts.Voice, error) {
	return tts.FilterVoices(GetAllVoices(), filter), nil
}

// IsAvailable 检查提供商是否可用
func (p *Provider) IsAvailable(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.runtimeConfigError() != nil {
		return false
	}

	_, _, err := p.synthesizeChunkWithWAVValidation(ctx, &synthesizeOptions{
		Text:  "测试",
		Voice: "BV700_streaming",
	}, 0, false)

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

func (p *Provider) runtimeConfigError() error {
	if p.baseURLErr != nil {
		return p.baseURLErr
	}
	if p.proxyErr != nil {
		return p.proxyErr
	}
	return nil
}

func wrapChunkError(err error, chunkIndex, totalChunks int) error {
	var ttsErr *tts.Error
	if errors.As(err, &ttsErr) {
		return &tts.Error{
			Code:     ttsErr.Code,
			Message:  fmt.Sprintf("chunk %d/%d failed: %s", chunkIndex+1, totalChunks, ttsErr.Message),
			Provider: ttsErr.Provider,
			Err:      ttsErr.Err,
		}
	}
	return fmt.Errorf("chunk %d/%d failed: %w", chunkIndex+1, totalChunks, err)
}

func unwrapRetryCause(err error) error {
	if !retry.IsRetryError(err) {
		return err
	}
	retryErrors := retry.GetRetryErrors(err)
	for index := len(retryErrors) - 1; index >= 0; index-- {
		if retryErrors[index] != nil {
			return retryErrors[index]
		}
	}
	return err
}

type wavChunkProfile struct {
	audioFormat   uint16
	channels      uint16
	sampleRate    uint32
	byteRate      uint32
	blockAlign    uint16
	bitsPerSample uint16
}

func (profile wavChunkProfile) matches(other wavChunkProfile) bool {
	return profile.audioFormat == other.audioFormat &&
		profile.channels == other.channels &&
		profile.sampleRate == other.sampleRate &&
		profile.byteRate == other.byteRate &&
		profile.blockAlign == other.blockAlign &&
		profile.bitsPerSample == other.bitsPerSample
}

func parseCanonicalWAV(chunk []byte) ([]byte, []byte, wavChunkProfile, bool) {
	if len(chunk) < 44 {
		return nil, nil, wavChunkProfile{}, false
	}
	if !hasCanonicalWAVSignature(chunk) || !hasPCMFormatChunk(chunk) {
		return nil, nil, wavChunkProfile{}, false
	}

	profile := readWAVChunkProfile(chunk)
	if !isValidWAVChunkProfile(profile) || !hasValidWAVChunkData(chunk, profile) {
		return nil, nil, wavChunkProfile{}, false
	}

	return chunk[:44], chunk[44:], profile, true
}

func hasCanonicalWAVSignature(chunk []byte) bool {
	return string(chunk[0:4]) == "RIFF" &&
		string(chunk[8:12]) == "WAVE" &&
		string(chunk[12:16]) == "fmt " &&
		string(chunk[36:40]) == "data"
}

func hasPCMFormatChunk(chunk []byte) bool {
	return binary.LittleEndian.Uint32(chunk[16:20]) == 16
}

func readWAVChunkProfile(chunk []byte) wavChunkProfile {
	return wavChunkProfile{
		audioFormat:   binary.LittleEndian.Uint16(chunk[20:22]),
		channels:      binary.LittleEndian.Uint16(chunk[22:24]),
		sampleRate:    binary.LittleEndian.Uint32(chunk[24:28]),
		byteRate:      binary.LittleEndian.Uint32(chunk[28:32]),
		blockAlign:    binary.LittleEndian.Uint16(chunk[32:34]),
		bitsPerSample: binary.LittleEndian.Uint16(chunk[34:36]),
	}
}

func isValidWAVChunkProfile(profile wavChunkProfile) bool {
	if profile.audioFormat != 1 || profile.channels == 0 || profile.sampleRate == 0 || profile.bitsPerSample == 0 || profile.bitsPerSample%8 != 0 {
		return false
	}

	expectedBlockAlign64 := uint64(profile.channels) * uint64(profile.bitsPerSample) / 8
	if expectedBlockAlign64 == 0 || expectedBlockAlign64 > tts.MaxWAVUint16 {
		return false
	}
	expectedBlockAlign := uint16(expectedBlockAlign64)
	if expectedBlockAlign == 0 || profile.blockAlign != expectedBlockAlign {
		return false
	}
	return profile.byteRate == profile.sampleRate*uint32(profile.blockAlign)
}

func hasValidWAVChunkData(chunk []byte, profile wavChunkProfile) bool {
	riffSize := binary.LittleEndian.Uint32(chunk[4:8])
	dataSize := binary.LittleEndian.Uint32(chunk[40:44])
	chunkSize := uint64(len(chunk))
	actualDataSize := chunkSize - 44
	expectedRiffSize := chunkSize - 8

	if dataSize == 0 || actualDataSize == 0 {
		return false
	}
	if expectedRiffSize > tts.MaxWAVUint32 || actualDataSize > tts.MaxWAVUint32 {
		return false
	}
	if riffSize != uint32(expectedRiffSize) || dataSize != uint32(actualDataSize) {
		return false
	}
	return actualDataSize%uint64(profile.blockAlign) == 0
}

// rebuildWAV 重建 WAV 文件，更新 header 中的数据大小。
func rebuildWAV(header []byte, pcmData []byte) ([]byte, error) {
	if len(header) < 44 {
		// header 不完整，直接返回 PCM 数据
		return pcmData, nil
	}

	// 复制 header
	newHeader := make([]byte, 44)
	copy(newHeader, header)

	// 计算新的文件大小
	dataSize := uint64(len(pcmData))
	if dataSize+36 > tts.MaxWAVUint32 {
		return nil, fmt.Errorf("wav data exceeds size limit")
	}
	fileSize := dataSize + 36 // 不含 RIFF header 的 8 字节

	// 更新 RIFF chunk size (字节 4-7)
	fileSize32 := uint32(fileSize)
	binary.LittleEndian.PutUint32(newHeader[4:8], fileSize32)

	// 更新 data chunk size (字节 40-43)
	binary.LittleEndian.PutUint32(newHeader[40:44], fileSize32-36)

	// 拼接 header 和 PCM 数据
	result := make([]byte, 0, 44+len(pcmData))
	result = append(result, newHeader...)
	result = append(result, pcmData...)

	return result, nil
}
