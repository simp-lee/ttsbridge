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

	"github.com/gorilla/websocket"
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

	dialer := websocket.Dialer{HandshakeTimeout: p.connectTimeout}
	if p.proxyURL != "" {
		if proxyURL, _ := url.Parse(p.proxyURL); proxyURL != nil {
			dialer.Proxy = http.ProxyURL(proxyURL)
		}
	}

	header := makeWSHeaders()
	conn, resp, err := dialer.DialContext(ctx, wsURL, header)
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
		"User-Agent":             {fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", chromiumMajorVersion, chromiumMajorVersion)},
		"Accept-Encoding":        {"gzip, deflate, br, zstd"},
		"Accept-Language":        {"en-US,en;q=0.9"},
		"Origin":                 {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"Pragma":                 {"no-cache"},
		"Cache-Control":          {"no-cache"},
		"Sec-WebSocket-Protocol": {"synthesize"},
		"Sec-WebSocket-Version":  {"13"},
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

	config := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"%s","wordBoundaryEnabled":"%s"},"outputFormat":"%s"}}}}`,
		getTimestamp(), sentenceBoundary, wordBoundary, defaultOutputFormat,
	)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(config)); err != nil {
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

	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
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
func (p *Provider) receiveAudio(ctx context.Context, conn *websocket.Conn, opts *SynthesizeOptions, offsetCompensation int64) ([]byte, error) {
	var audioData []byte
	audioReceived := false

	receiveTimeout := p.receiveTimeout

	if err := conn.SetReadDeadline(time.Now().Add(receiveTimeout)); err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeWebSocketError,
			Message:  "failed to set read deadline",
			Provider: providerName,
			Err:      err,
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		messageType, message, err := conn.ReadMessage()
		if err != nil {
			// Normal closure without audio is an error
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
			return nil, classifyWebsocketReadError(err, receiveTimeout, "websocket read error while receiving audio")
		}

		switch messageType {
		case websocket.BinaryMessage:
			if audioChunk := extractAudioData(message); len(audioChunk) > 0 {
				audioData = append(audioData, audioChunk...)
				audioReceived = true
			}
		case websocket.TextMessage:
			msg := string(message)
			if opts.MetadataCallback != nil && strings.Contains(msg, "Path:audio.metadata") {
				p.parseAndCallbackMetadata(message, opts.MetadataCallback, offsetCompensation)
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
func (p *Provider) parseAndCallbackMetadata(message []byte, callback func(string, int64, int64, string), offsetCompensation int64) {
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
			text := html.UnescapeString(meta.Data.Text.Text)
			callback(meta.Type, adjustedOffset, meta.Data.Duration, text)
		}
	}
}

func classifyWebsocketReadError(err error, receiveTimeout time.Duration, message string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
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

	return &tts.Error{
		Code:     tts.ErrCodeWebSocketError,
		Message:  message,
		Provider: providerName,
		Err:      err,
	}
}
