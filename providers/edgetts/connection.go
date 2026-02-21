package edgetts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/ttsbridge/tts"
)

// handleClockSkewError creates appropriate error for 403 responses with clock skew detection
func handleClockSkewError(serverDate string) error {
	if serverDate != "" {
		// Try to adjust clock skew, always return retryable clock skew error
		// The adjustment will take effect on next retry attempt
		_ = AdjustClockSkew(serverDate)
		return &tts.Error{
			Code:     tts.ErrCodeClockSkew,
			Message:  "clock skew detected, adjusted and retrying",
			Provider: providerName,
		}
	}
	// 403 without Date header - authentication issue, not retryable
	return &tts.Error{
		Code:     tts.ErrCodeAuthFailed,
		Message:  "authentication failed",
		Provider: providerName,
	}
}

// connect establishes WebSocket connection with automatic clock skew handling.
// Returns ErrCodeClockSkew on 403 to signal retry with adjusted clock offset.
func (p *Provider) connect(ctx context.Context) (*websocket.Conn, error) {
	wsURL := fmt.Sprintf(wsURLTemplate,
		baseURL, p.clientToken, generateConnectionID(),
		GenerateSecMsGec(p.clientToken), secMsGecVersion)

	dialOpts := &websocket.DialOptions{
		HTTPHeader:   makeWSHeaders(),
		Subprotocols: []string{"synthesize"},
	}
	if p.proxyURL != "" {
		if proxyURL, _ := url.Parse(p.proxyURL); proxyURL != nil {
			dialOpts.HTTPClient = &http.Client{
				Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
			}
		}
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, p.connectTimeout)
	defer dialCancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, dialOpts)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Clock skew detection and adjustment on 403 Forbidden
		if resp != nil && resp.StatusCode == 403 {
			return nil, handleClockSkewError(resp.Header.Get("Date"))
		}
		return nil, &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "websocket connection failed",
			Provider: providerName,
			Err:      err,
		}
	}

	return conn, nil
}

func makeWSHeaders() http.Header {
	header := http.Header{
		"User-Agent":      {fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", chromiumMajorVersion, chromiumMajorVersion)},
		"Accept-Encoding": {"gzip, deflate, br, zstd"},
		"Accept-Language": {"en-US,en;q=0.9"},
		"Origin":          {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"Pragma":          {"no-cache"},
		"Cache-Control":   {"no-cache"},
	}
	header.Set("Cookie", makeMUIDCookie())
	return header
}

// sendConfig sends configuration message
func (p *Provider) sendConfig(ctx context.Context, conn *websocket.Conn, opts *SynthesizeOptions) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	wordBoundary := "false"
	if opts.WordBoundaryEnabled {
		wordBoundary = "true"
	}
	sentenceBoundary := "false"
	if opts.SentenceBoundaryEnabled {
		sentenceBoundary = "true"
	}

	outputFormat := defaultOutputFormat
	if opts.OutputFormat != "" {
		outputFormat = opts.OutputFormat
	}

	config := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"%s","wordBoundaryEnabled":"%s"},"outputFormat":"%s"}}}}`,
		getTimestamp(), sentenceBoundary, wordBoundary, outputFormat,
	)

	if err := conn.Write(ctx, websocket.MessageText, []byte(config)); err != nil {
		return &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "failed to send config message",
			Provider: providerName,
			Err:      err,
		}
	}
	return nil
}

// sendSSML sends SSML message
func (p *Provider) sendSSML(ctx context.Context, conn *websocket.Conn, opts *SynthesizeOptions) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ssml := buildSSML(opts)
	// Note: Timestamp needs 'Z' suffix (Microsoft Edge bug)
	message := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%sZ\r\nPath:ssml\r\n\r\n%s",
		generateRequestID(), getTimestamp(), ssml,
	)

	if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		return &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "failed to send ssml message",
			Provider: providerName,
			Err:      err,
		}
	}
	return nil
}

// receiveAudio receives audio data and handles metadata
func (p *Provider) receiveAudio(ctx context.Context, conn *websocket.Conn, opts *SynthesizeOptions, offsetCompensation int64, chunkIndex int) ([]byte, error) {
	var audioData []byte
	audioReceived := false

	receiveTimeout := p.receiveTimeout

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		readCtx, readCancel := context.WithTimeout(ctx, receiveTimeout)
		messageType, message, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			// Normal closure without audio is an error
			var ce websocket.CloseError
			if errors.As(err, &ce) && ce.Code == websocket.StatusNormalClosure {
				if !audioReceived {
					return nil, &tts.Error{
						Code:     tts.ErrCodeNoAudioReceived,
						Message:  "no audio data received",
						Provider: providerName,
					}
				}
				return audioData, nil
			}
			return nil, classifyWebsocketReadError(err, ctx.Err(), receiveTimeout, "websocket read error while receiving audio")
		}

		switch messageType {
		case websocket.MessageBinary:
			if audioChunk := extractAudioData(message); len(audioChunk) > 0 {
				audioData = append(audioData, audioChunk...)
				audioReceived = true
			}
		case websocket.MessageText:
			msg := string(message)
			if opts.BoundaryCallback != nil && strings.Contains(msg, "Path:audio.metadata") {
				p.parseAndCallbackMetadata(message, opts.BoundaryCallback, offsetCompensation, chunkIndex)
			}
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

// parseAndCallbackMetadata parses metadata and invokes callback with offset compensation
func (p *Provider) parseAndCallbackMetadata(message []byte, callback func(tts.BoundaryEvent), offsetCompensation int64, chunkIndex int) {
	parts := strings.Split(string(message), "\r\n\r\n")
	if len(parts) < 2 {
		return
	}

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

	if err := json.Unmarshal([]byte(parts[1]), &metadata); err != nil {
		return
	}

	for _, meta := range metadata.Metadata {
		if meta.Type == "WordBoundary" || meta.Type == "SentenceBoundary" {
			adjustedOffset := meta.Data.Offset + offsetCompensation
			event := tts.BoundaryEvent{
				Type:       meta.Type,
				Text:       html.UnescapeString(meta.Data.Text.Text),
				Offset:     time.Duration(adjustedOffset) * 100,
				Duration:   time.Duration(meta.Data.Duration) * 100,
				OffsetMs:   adjustedOffset / 10000,
				DurationMs: meta.Data.Duration / 10000,
				ChunkIndex: chunkIndex,
			}
			callback(event)
		}
	}
}

func classifyWebsocketReadError(err error, parentCtxErr error, receiveTimeout time.Duration, message string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if errors.Is(parentCtxErr, context.Canceled) || errors.Is(parentCtxErr, context.DeadlineExceeded) {
			return parentCtxErr
		}
		return &tts.Error{
			Code:     tts.ErrCodeTimeout,
			Message:  fmt.Sprintf("%s: timeout after %v", message, receiveTimeout),
			Provider: providerName,
			Err:      errors.New(err.Error()),
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &tts.Error{
			Code:     tts.ErrCodeTimeout,
			Message:  fmt.Sprintf("%s: timeout after %v", message, receiveTimeout),
			Provider: providerName,
			Err:      err,
		}
	}

	var ce websocket.CloseError
	if errors.As(err, &ce) {
		return &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  fmt.Sprintf("%s: connection closed with status %d: %s", message, ce.Code, ce.Reason),
			Provider: providerName,
			Err:      err,
		}
	}

	return &tts.Error{
		Code:     tts.ErrCodeWebSocketError,
		Message:  message,
		Provider: providerName,
		Err:      err,
	}
}
