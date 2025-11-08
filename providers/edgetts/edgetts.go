package edgetts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/simp-lee/ttsbridge/audio"
	"github.com/simp-lee/ttsbridge/tts"
)

const (
	providerName       = "edge"
	defaultClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"

	// Updated API endpoints (from api.msedgeservices.com)
	baseURL           = "api.msedgeservices.com/tts/cognitiveservices"
	wsURLTemplate     = "wss://%s/websocket/v1?Ocp-Apim-Subscription-Key=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s"
	voicesURLTemplate = "https://%s/voices/list?Ocp-Apim-Subscription-Key=%s"

	// Chromium version information
	chromiumMajorVersion = "140"
	secMsGecVersion      = "1-140.0.3485.14"

	// Maximum retry attempts for failed connections
	maxRetryAttempts = 2
)

// ProviderOptions Provider 配置选项
type ProviderOptions struct {
	// ClientToken 客户端 token (默认使用 defaultClientToken)
	ClientToken string
	// HTTPTimeout HTTP 客户端超时时间 (默认 30 秒)
	HTTPTimeout time.Duration
	// ProxyURL 代理地址 (如 "http://proxy.example.com:8080")
	ProxyURL string
	// ConnectTimeout WebSocket 连接超时时间 (默认 10 秒)
	ConnectTimeout time.Duration
	// MaxRetryAttempts 最大重试次数 (默认 2 次)
	MaxRetryAttempts int
	// ReceiveTimeout WebSocket 接收超时时间 (默认 60 秒)
	ReceiveTimeout time.Duration
}

// Provider Edge TTS 提供商实现
type Provider struct {
	client           *http.Client
	clientToken      string
	proxyURL         string
	connectTimeout   time.Duration
	maxRetryAttempts int
	receiveTimeout   time.Duration
}

// New 创建 Edge TTS 提供商
func New() *Provider {
	return NewWithOptions(&ProviderOptions{})
}

// NewWithToken 创建带自定义 token 的 Edge TTS 提供商
func NewWithToken(token string) *Provider {
	return NewWithOptions(&ProviderOptions{
		ClientToken: token,
	})
}

// NewWithOptions 创建带配置选项的 Edge TTS 提供商
func NewWithOptions(opts *ProviderOptions) *Provider {
	if opts == nil {
		opts = &ProviderOptions{}
	}

	// 设置默认值
	if opts.ClientToken == "" {
		opts.ClientToken = defaultClientToken
	}
	if opts.HTTPTimeout == 0 {
		opts.HTTPTimeout = 30 * time.Second
	}
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = 10 * time.Second
	}
	if opts.MaxRetryAttempts == 0 {
		opts.MaxRetryAttempts = maxRetryAttempts
	}
	if opts.ReceiveTimeout == 0 {
		opts.ReceiveTimeout = 60 * time.Second
	}

	// 配置 HTTP 客户端
	client := &http.Client{
		Timeout: opts.HTTPTimeout,
	}

	// 如果指定了代理，配置 Transport
	if opts.ProxyURL != "" {
		client.Transport = &http.Transport{
			Proxy: http.ProxyURL(mustParseProxyURL(opts.ProxyURL)),
		}
	}

	return &Provider{
		client:           client,
		clientToken:      opts.ClientToken,
		proxyURL:         opts.ProxyURL,
		connectTimeout:   opts.ConnectTimeout,
		maxRetryAttempts: opts.MaxRetryAttempts,
		receiveTimeout:   opts.ReceiveTimeout,
	}
}

// mustParseProxyURL 解析代理 URL，失败返回 nil
func mustParseProxyURL(proxyURL string) *url.URL {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	return u
}

// Name 返回提供商名称
func (p *Provider) Name() string {
	return providerName
}

// Synthesize 同步合成语音
func (p *Provider) Synthesize(ctx context.Context, opts *tts.SynthesizeOptions) ([]byte, error) {
	// 准备文本（清理和转义）
	preparedText := PrepareTextForSSML(opts.Text)

	// 分割长文本
	textChunks := SplitTextByByteLength(preparedText, maxSSMLBytes)

	var voiceAudio []byte
	var err error

	// 如果只有一个块，直接合成
	if len(textChunks) == 1 {
		voiceAudio, err = p.synthesizeChunk(ctx, opts, textChunks[0], 0)
		if err != nil {
			return nil, err
		}
	} else {
		// 多个块需要合并音频
		var allAudio []byte
		var offsetCompensation int64

		for _, chunk := range textChunks {
			chunkAudio, err := p.synthesizeChunk(ctx, opts, chunk, offsetCompensation)
			if err != nil {
				return nil, err
			}
			allAudio = append(allAudio, chunkAudio...)

			// 累加偏移量补偿 (8,750,000 ticks)
			offsetCompensation += 8750000
		}
		voiceAudio = allAudio
	}

	// 如果配置了背景音乐，进行混音
	if opts.BackgroundMusic != nil && opts.BackgroundMusic.MusicPath != "" {
		mixedAudio, err := audio.MixWithBackgroundMusic(ctx, voiceAudio, opts.BackgroundMusic)
		if err != nil {
			return nil, &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "background music mixing failed",
				Provider: providerName,
				Err:      err,
			}
		}
		return mixedAudio, nil
	}

	return voiceAudio, nil
}

// synthesizeChunk 合成单个文本块，支持 403 重试
func (p *Provider) synthesizeChunk(ctx context.Context, opts *tts.SynthesizeOptions, text string, offsetCompensation int64) ([]byte, error) {
	var lastErr error

	// 创建临时选项，使用处理后的文本
	chunkOpts := *opts
	chunkOpts.Text = text

	// 尝试合成，遇到 403 时重试
	for attempt := 0; attempt <= p.maxRetryAttempts; attempt++ {
		conn, err := p.connect(ctx)
		if err != nil {
			// 如果是连接错误且包含 403，继续重试
			if strings.Contains(err.Error(), "403") && attempt < p.maxRetryAttempts {
				lastErr = err
				continue
			}
			return nil, &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  fmt.Sprintf("failed to connect (attempt %d/%d)", attempt+1, p.maxRetryAttempts+1),
				Provider: providerName,
				Err:      err,
			}
		}

		// 发送配置消息
		if err := p.sendConfig(ctx, conn, opts); err != nil {
			conn.Close()
			return nil, &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "failed to send config",
				Provider: providerName,
				Err:      err,
			}
		}

		// 发送 SSML 消息
		if err := p.sendSSML(ctx, conn, &chunkOpts); err != nil {
			conn.Close()
			return nil, &tts.Error{
				Code:     tts.ErrCodeInternalError,
				Message:  "failed to send SSML",
				Provider: providerName,
				Err:      err,
			}
		}

		// 接收音频数据
		audio, err := p.receiveAudio(ctx, conn, opts, offsetCompensation)
		conn.Close()

		if err == nil {
			return audio, nil
		}

		lastErr = err
	}

	// 所有重试都失败
	return nil, &tts.Error{
		Code:     tts.ErrCodeNetworkError,
		Message:  fmt.Sprintf("all retry attempts failed (%d)", p.maxRetryAttempts+1),
		Provider: providerName,
		Err:      lastErr,
	}
}

// SynthesizeStream 流式合成语音
func (p *Provider) SynthesizeStream(ctx context.Context, opts *tts.SynthesizeOptions) (tts.AudioStream, error) {
	// 准备文本（清理和转义）
	preparedText := PrepareTextForSSML(opts.Text)

	// 分割长文本
	textChunks := SplitTextByByteLength(preparedText, maxSSMLBytes)

	return &edgeAudioStream{
		ctx:         ctx,
		closed:      false,
		opts:        opts,
		provider:    p,
		textChunks:  textChunks,
		chunkIndex:  0,
		conn:        nil,
		initialized: false,
	}, nil
}

// ListVoices 列出可用的语音
func (p *Provider) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	return p.listVoicesWithRetry(ctx, locale, 0)
}

// listVoicesWithRetry 带重试的语音列表获取
func (p *Provider) listVoicesWithRetry(ctx context.Context, locale string, attempt int) ([]tts.Voice, error) {
	// 构建 URL，添加 DRM token
	secMsGec := GenerateSecMsGec(p.clientToken)
	voiceURL := fmt.Sprintf(
		"%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		fmt.Sprintf(voicesURLTemplate, baseURL, p.clientToken),
		secMsGec,
		secMsGecVersion,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", voiceURL, nil)
	if err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeNetworkError,
			Message:  "failed to create request",
			Provider: providerName,
			Err:      err,
		}
	}

	// 设置请求头
	req.Header.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", chromiumMajorVersion, chromiumMajorVersion))
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeNetworkError,
			Message:  "failed to fetch voices",
			Provider: providerName,
			Err:      err,
		}
	}
	defer resp.Body.Close()

	// 处理 403 错误（时钟不同步）
	if resp.StatusCode == 403 {
		if attempt < p.maxRetryAttempts {
			// 尝试时钟偏差校正
			if serverDate := resp.Header.Get("Date"); serverDate != "" {
				if adjustErr := AdjustClockSkew(serverDate); adjustErr == nil {
					// 重试
					return p.listVoicesWithRetry(ctx, locale, attempt+1)
				}
			}
		}
		return nil, &tts.Error{
			Code:     tts.ErrCodeClockSkew,
			Message:  fmt.Sprintf("clock skew error after %d attempts", attempt+1),
			Provider: providerName,
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &tts.Error{
			Code:     tts.ErrCodeProviderUnavail,
			Message:  fmt.Sprintf("failed to get voices: status %d", resp.StatusCode),
			Provider: providerName,
		}
	}

	// 解析 JSON 响应
	var rawVoices []struct {
		Name           string `json:"Name"`
		ShortName      string `json:"ShortName"`
		Gender         string `json:"Gender"`
		Locale         string `json:"Locale"`
		SuggestedCodec string `json:"SuggestedCodec"`
		FriendlyName   string `json:"FriendlyName"`
		Status         string `json:"Status"`
		VoiceTag       struct {
			ContentCategories  []string `json:"ContentCategories"`
			VoicePersonalities []string `json:"VoicePersonalities"`
		} `json:"VoiceTag"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawVoices); err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  "failed to parse voices JSON",
			Provider: providerName,
			Err:      err,
		}
	}

	// 转换为 tts.Voice 格式
	voices := make([]tts.Voice, 0, len(rawVoices))
	for _, rv := range rawVoices {
		// 跳过已废弃的语音
		if rv.Status == "Deprecated" {
			continue
		}

		voice := tts.Voice{
			ID:          rv.ShortName,
			Name:        rv.Name,
			DisplayName: rv.FriendlyName,
			Locale:      rv.Locale,
			Gender:      rv.Gender,
			ShortName:   rv.ShortName,
			Provider:    providerName,
			Styles:      rv.VoiceTag.VoicePersonalities,
		}

		// 根据 locale 过滤
		if locale == "" || strings.HasPrefix(rv.Locale, locale) {
			voices = append(voices, voice)
		}
	}

	return voices, nil
}

// IsAvailable 检查提供商是否可用
func (p *Provider) IsAvailable(ctx context.Context) bool {
	// 尝试连接以验证 token 是否有效
	conn, err := p.connect(ctx)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// connect 连接到 WebSocket
func (p *Provider) connect(ctx context.Context) (*websocket.Conn, error) {
	return p.connectWithRetry(ctx, 0)
}

// connectWithRetry 带重试的连接方法
func (p *Provider) connectWithRetry(ctx context.Context, attempt int) (*websocket.Conn, error) {
	// 生成 DRM token 和 Connection ID
	secMsGec := GenerateSecMsGec(p.clientToken)
	connectionID := generateConnectionID()

	// 构建 WebSocket URL
	wsURL := fmt.Sprintf(
		wsURLTemplate,
		baseURL,
		p.clientToken,
		connectionID,
		secMsGec,
		secMsGecVersion,
	)

	// 设置请求头（参考 edge-tts 项目）
	header := http.Header{
		"Origin":                 []string{"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"Accept-Encoding":        []string{"gzip, deflate, br"},
		"Accept-Language":        []string{"en-US,en;q=0.9"},
		"Pragma":                 []string{"no-cache"},
		"Cache-Control":          []string{"no-cache"},
		"User-Agent":             []string{fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", chromiumMajorVersion, chromiumMajorVersion)},
		"Sec-WebSocket-Protocol": []string{"synthesize"},
		"Sec-WebSocket-Version":  []string{"13"},
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: p.connectTimeout,
	}

	// 如果配置了代理，使用代理
	if p.proxyURL != "" {
		proxyURL := mustParseProxyURL(p.proxyURL)
		if proxyURL != nil {
			dialer.Proxy = http.ProxyURL(proxyURL)
		}
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		// 处理 403 错误（时钟不同步）
		if resp != nil && resp.StatusCode == 403 {
			if attempt < p.maxRetryAttempts {
				// 尝试时钟偏差校正
				if serverDate := resp.Header.Get("Date"); serverDate != "" {
					if adjustErr := AdjustClockSkew(serverDate); adjustErr == nil {
						// 重试连接
						return p.connectWithRetry(ctx, attempt+1)
					}
				}
			}
			return nil, &tts.Error{
				Code:     tts.ErrCodeClockSkew,
				Message:  fmt.Sprintf("clock skew error after %d attempts", attempt+1),
				Provider: providerName,
				Err:      err,
			}
		}
		return nil, fmt.Errorf("failed to connect to Edge TTS (attempt %d): %w", attempt+1, err)
	}

	return conn, nil
}

// sendConfig 发送配置消息
func (p *Provider) sendConfig(ctx context.Context, conn *websocket.Conn, opts *tts.SynthesizeOptions) error {
	timestamp := getTimestamp()

	// 使用官方默认格式（与 edge-tts 项目保持一致）
	outputFormat := "audio-24khz-48kbitrate-mono-mp3"

	// 根据选项确定边界元数据设置
	wordBoundary := "false"
	if opts.WordBoundaryEnabled {
		wordBoundary = "true"
	}

	sentenceBoundary := "false"
	if opts.SentenceBoundaryEnabled {
		sentenceBoundary = "true"
	}

	config := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"%s","wordBoundaryEnabled":"%s"},"outputFormat":"%s"}}}}`,
		timestamp,
		sentenceBoundary,
		wordBoundary,
		outputFormat,
	)

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return conn.WriteMessage(websocket.TextMessage, []byte(config))
}

// sendSSML 发送 SSML 消息
func (p *Provider) sendSSML(ctx context.Context, conn *websocket.Conn, opts *tts.SynthesizeOptions) error {
	requestID := generateRequestID()
	timestamp := getTimestamp()

	// 构建 SSML
	ssml := buildSSML(opts)

	// 注意：时间戳后需要添加 'Z'，这是 Microsoft Edge 的要求
	message := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%sZ\r\nPath:ssml\r\n\r\n%s",
		requestID,
		timestamp,
		ssml,
	)

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return conn.WriteMessage(websocket.TextMessage, []byte(message))
}

// receiveAudio 接收音频数据并处理元数据
func (p *Provider) receiveAudio(ctx context.Context, conn *websocket.Conn, opts *tts.SynthesizeOptions, offsetCompensation int64) ([]byte, error) {
	audioData := make([]byte, 0)
	audioReceived := false

	// 获取接收超时配置
	receiveTimeout := p.receiveTimeout
	// 允许通过 Extra 参数覆盖默认超时
	if opts.Extra != nil {
		if timeout, ok := opts.Extra["ReceiveTimeout"].(time.Duration); ok && timeout > 0 {
			receiveTimeout = timeout
		}
	}

	// 设置读取超时
	if err := conn.SetReadDeadline(time.Now().Add(receiveTimeout)); err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "failed to set read deadline",
			Provider: providerName,
			Err:      err,
		}
	}

	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				if !audioReceived {
					return nil, &tts.Error{
						Code:     tts.ErrCodeNoAudioReceived,
						Message:  "no audio data received",
						Provider: providerName,
					}
				}
				return audioData, nil
			}
			// 检查是否是超时错误
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				return nil, &tts.Error{
					Code:     tts.ErrCodeTimeout,
					Message:  fmt.Sprintf("receive timeout after %v", receiveTimeout),
					Provider: providerName,
				}
			}
			return nil, &tts.Error{
				Code:     tts.ErrCodeWebSocketError,
				Message:  "websocket read error",
				Provider: providerName,
				Err:      err,
			}
		}

		switch messageType {
		case websocket.BinaryMessage:
			// 解析帧头，提取音频数据
			audioChunk := extractAudioData(message)
			if len(audioChunk) > 0 {
				audioData = append(audioData, audioChunk...)
				audioReceived = true
			}
		case websocket.TextMessage:
			msg := string(message)

			// 解析元数据（带偏移量补偿）
			if opts.MetadataCallback != nil && strings.Contains(msg, "Path:audio.metadata") {
				p.parseAndCallbackMetadataWithCompensation(message, opts.MetadataCallback, offsetCompensation)
			}

			// 检查是否是结束消息
			if strings.Contains(msg, "Path:turn.end") {
				if !audioReceived {
					return nil, &tts.Error{
						Code:     tts.ErrCodeNoAudioReceived,
						Message:  "no audio data received",
						Provider: providerName,
					}
				}
				return audioData, nil
			}
		}
	}
}

// parseAndCallbackMetadataWithCompensation 解析元数据并应用偏移量补偿后调用回调函数
func (p *Provider) parseAndCallbackMetadataWithCompensation(message []byte, callback func(string, int64, int64, string), offsetCompensation int64) {
	// 解析消息体，提取 JSON 部分
	parts := strings.Split(string(message), "\r\n\r\n")
	if len(parts) < 2 {
		return
	}

	jsonData := parts[1]

	// 解析 JSON
	var metadata struct {
		Metadata []struct {
			Type string `json:"Type"`
			Data struct {
				Offset   int64 `json:"Offset"`
				Duration int64 `json:"Duration"`
				Text     struct {
					Text string `json:"Text"`
				} `json:"text"`
			} `json:"Data"`
		} `json:"Metadata"`
	}

	if err := json.Unmarshal([]byte(jsonData), &metadata); err != nil {
		return
	}

	// 调用回调函数，应用偏移量补偿
	for _, meta := range metadata.Metadata {
		if meta.Type == "WordBoundary" || meta.Type == "SentenceBoundary" {
			adjustedOffset := meta.Data.Offset + offsetCompensation
			callback(meta.Type, adjustedOffset, meta.Data.Duration, meta.Data.Text.Text)
		}
	}
}

// buildSSML 构建 SSML
func buildSSML(opts *tts.SynthesizeOptions) string {
	voice := opts.Voice
	if voice == "" {
		voice = "zh-CN-XiaoxiaoNeural"
	}

	rate := formatRate(opts.Rate)
	volume := formatVolume(opts.Volume)
	pitch := formatPitch(opts.Pitch)

	// 注意：文本应该已经在 Synthesize 函数中被清理和转义过
	// 这里直接使用传入的文本
	text := opts.Text

	ssml := fmt.Sprintf(`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>
    <voice name='%s'>
        <prosody rate='%s' volume='%s' pitch='%s'>
            %s
        </prosody>
    </voice>
</speak>`, voice, rate, volume, pitch, text)

	return ssml
}

// formatRate 格式化语速
func formatRate(rate float64) string {
	if rate == 1.0 {
		return "+0%"
	}
	percent := (rate - 1.0) * 100
	if percent > 0 {
		return fmt.Sprintf("+%.0f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// formatVolume 格式化音量
func formatVolume(volume float64) string {
	if volume == 1.0 {
		return "+0%"
	}
	percent := (volume - 1.0) * 100
	if percent > 0 {
		return fmt.Sprintf("+%.0f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// formatPitch 格式化音调
// 根据 SSML 规范，音调应该使用百分比（%）或半音（st）作为单位
// pitch = 1.0 表示原始音调，2.0 表示提高 100%
func formatPitch(pitch float64) string {
	if pitch == 1.0 {
		return "+0%"
	}
	percent := (pitch - 1.0) * 100
	if percent > 0 {
		return fmt.Sprintf("+%.0f%%", percent)
	}
	return fmt.Sprintf("%.0f%%", percent)
}

// generateRequestID 生成请求 ID
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 理论上不应该失败，但如果失败就使用时间戳作为后备
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// generateConnectionID 生成连接 ID（UUID 格式但不带破折号）
func generateConnectionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 理论上不应该失败，但如果失败就使用时间戳作为后备
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	// 设置 UUID version 4 的标准位
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getTimestamp 获取时间戳
func getTimestamp() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
}

// edgeAudioStream Edge TTS 音频流
type edgeAudioStream struct {
	conn               *websocket.Conn
	ctx                context.Context
	closed             bool
	opts               *tts.SynthesizeOptions
	provider           *Provider
	textChunks         []string // 分块后的文本列表
	chunkIndex         int      // 当前处理的块索引
	initialized        bool     // 是否已初始化第一个块
	offsetCompensation int64    // 偏移量补偿（用于多块元数据时间连续）
}

func (s *edgeAudioStream) Read() ([]byte, error) {
	if s.closed {
		return nil, io.EOF
	}

	// 检查 context 是否已取消
	select {
	case <-s.ctx.Done():
		s.closed = true
		if s.conn != nil {
			s.conn.Close()
		}
		return nil, s.ctx.Err()
	default:
	}

	// 初始化第一个块的连接
	if !s.initialized {
		if err := s.initializeChunk(); err != nil {
			s.closed = true
			return nil, err
		}
		s.initialized = true
	}

	// 获取接收超时配置
	receiveTimeout := s.provider.receiveTimeout
	// 允许通过 Extra 参数覆盖默认超时
	if s.opts.Extra != nil {
		if timeout, ok := s.opts.Extra["ReceiveTimeout"].(time.Duration); ok && timeout > 0 {
			receiveTimeout = timeout
		}
	}

	// 设置读取超时
	if err := s.conn.SetReadDeadline(time.Now().Add(receiveTimeout)); err != nil {
		s.closed = true
		return nil, &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "failed to set read deadline",
			Provider: providerName,
			Err:      err,
		}
	}

	for {
		messageType, message, err := s.conn.ReadMessage()
		if err != nil {
			s.closed = true
			// 检查是否是超时错误
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				return nil, &tts.Error{
					Code:     tts.ErrCodeTimeout,
					Message:  fmt.Sprintf("stream read timeout after %v", receiveTimeout),
					Provider: providerName,
				}
			}
			return nil, &tts.Error{
				Code:     tts.ErrCodeWebSocketError,
				Message:  "websocket read error in stream",
				Provider: providerName,
				Err:      err,
			}
		}

		switch messageType {
		case websocket.BinaryMessage:
			// 解析帧头，提取音频数据
			audioChunk := extractAudioData(message)
			if len(audioChunk) > 0 {
				return audioChunk, nil
			}
		case websocket.TextMessage:
			msgStr := string(message)

			// 解析元数据（带偏移量补偿）
			if s.opts.MetadataCallback != nil && strings.Contains(msgStr, "Path:audio.metadata") {
				s.provider.parseAndCallbackMetadataWithCompensation(message, s.opts.MetadataCallback, s.offsetCompensation)
			}

			// 检查是否是结束消息
			if strings.Contains(msgStr, "Path:turn.end") {
				// 如果还有更多块，处理下一块
				if s.chunkIndex < len(s.textChunks)-1 {
					// 累加偏移量补偿 (8,750,000 ticks)
					s.offsetCompensation += 8750000

					// 关闭当前连接
					s.conn.Close()

					// 移动到下一块
					s.chunkIndex++
					if err := s.initializeChunk(); err != nil {
						s.closed = true
						return nil, err
					}
					// 继续读取下一块的数据
					continue
				}

				// 所有块都处理完毕
				s.closed = true
				return nil, io.EOF
			}
		}
	}
}

// initializeChunk 初始化当前块的 WebSocket 连接并发送 SSML
func (s *edgeAudioStream) initializeChunk() error {
	conn, err := s.provider.connect(s.ctx)
	if err != nil {
		return err
	}
	s.conn = conn

	// 发送配置消息
	if err := s.provider.sendConfig(s.ctx, conn, s.opts); err != nil {
		conn.Close()
		return err
	}

	// 创建临时选项，使用当前块的文本
	chunkOpts := *s.opts
	chunkOpts.Text = s.textChunks[s.chunkIndex]

	// 发送 SSML 消息
	if err := s.provider.sendSSML(s.ctx, conn, &chunkOpts); err != nil {
		conn.Close()
		return err
	}

	return nil
}

func (s *edgeAudioStream) Close() error {
	if !s.closed {
		s.closed = true
		if s.conn != nil {
			return s.conn.Close()
		}
	}
	return nil
}

// extractAudioData 从 WebSocket 二进制消息中提取音频数据
// Edge TTS 的二进制消息格式：前两字节为头部长度（大端序），然后是头部内容，最后是音频数据
func extractAudioData(message []byte) []byte {
	if len(message) < 2 {
		return nil
	}

	// 前两字节是头部长度（大端序）
	headerLen := int(message[0])<<8 | int(message[1])

	// 头部总长度 = 2字节长度标识 + headerLen
	totalHeaderLen := 2 + headerLen

	if len(message) <= totalHeaderLen {
		return nil
	}

	// 返回头部之后的音频数据
	return message[totalHeaderLen:]
}
