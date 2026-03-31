package edgetts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/ttsbridge/tts"
)

var websocketDial = websocket.Dial

func noAudioReceivedError() error {
	return &tts.Error{
		Code:     tts.ErrCodeNoAudioReceived,
		Message:  "no audio data received",
		Provider: providerName,
	}
}

func boundaryEmissionConflictError(err error) error {
	return &tts.Error{
		Code:     tts.ErrCodeInternalError,
		Message:  "cannot retry chunk after boundary callback emission",
		Provider: providerName,
		Err:      err,
	}
}

var clockSkewIndicators = []string{
	"clock skew",
	"clockskew",
	"invalid timestamp",
	"request timestamp",
	"timestamp expired",
	"x-timestamp",
}

var unsupportedOutputFormatIndicators = []string{
	"unsupported edge output format",
	"unsupported output format",
}

const handshakeErrorBodyReadLimit = 4096

// maxResponseBodyReadBytes is a defense-in-depth limit for readResponseBody.
// Current callers already pass io.LimitReader, but this prevents future callers
// from accidentally reading unbounded responses.
const maxResponseBodyReadBytes = 1 << 20 // 1 MB

func hasClockSkewSignal(detail string) bool {
	lower := strings.ToLower(strings.TrimSpace(detail))
	if lower == "" {
		return false
	}
	for _, indicator := range clockSkewIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func hasUnsupportedOutputFormatSignal(detail string) bool {
	lower := strings.ToLower(strings.TrimSpace(detail))
	if lower == "" {
		return false
	}
	for _, indicator := range unsupportedOutputFormatIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func unsupportedOutputFormatDetail(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	var ce websocket.CloseError
	if errors.As(err, &ce) && hasUnsupportedOutputFormatSignal(ce.Reason) {
		return strings.TrimSpace(ce.Reason), true
	}

	if hasUnsupportedOutputFormatSignal(err.Error()) {
		return strings.TrimSpace(err.Error()), true
	}

	return "", false
}

func authFailedError(err error) error {
	return &tts.Error{
		Code:     tts.ErrCodeAuthFailed,
		Message:  "authentication failed",
		Provider: providerName,
		Err:      err,
	}
}

func (p *Provider) classifyForbiddenResponse(serverDate, detail string) error {
	if !hasClockSkewSignal(detail) {
		return authFailedError(nil)
	}
	if serverDate == "" {
		return authFailedError(errors.New("clock skew indicated but server date header missing"))
	}
	if err := p.adjustClockSkew(serverDate); err != nil {
		return authFailedError(fmt.Errorf("clock skew indicated but failed to parse server date: %w", err))
	}
	return &tts.Error{
		Code:     tts.ErrCodeClockSkew,
		Message:  "clock skew detected, adjusted and retrying",
		Provider: providerName,
	}
}

func readResponseBody(body io.Reader) string {
	if body == nil {
		return ""
	}
	payload, err := io.ReadAll(io.LimitReader(body, maxResponseBodyReadBytes))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

// connect establishes WebSocket connection with automatic clock skew handling.
// Returns ErrCodeClockSkew on 403 to signal retry with adjusted clock offset.
func (p *Provider) connect(ctx context.Context) (*websocket.Conn, error) {
	if p.connectHook != nil {
		return p.connectHook(ctx)
	}
	if err := p.runtimeConfigError(); err != nil {
		return nil, err
	}

	wsURL := fmt.Sprintf(wsURLTemplate,
		baseURL, p.clientToken, generateConnectionID(),
		p.generateSecMsGec(), secMsGecVersion)

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

	conn, resp, err := websocketDial(dialCtx, wsURL, dialOpts)
	if resp != nil && resp.Body != nil {
		defer closeIgnoreError(resp.Body)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Clock skew detection and adjustment on 403 Forbidden
		if resp != nil && resp.StatusCode == 403 {
			responseBody := ""
			if resp.Body != nil {
				responseBody = readResponseBody(io.LimitReader(resp.Body, handshakeErrorBodyReadLimit))
			}
			detail := err.Error()
			if responseBody != "" {
				detail = strings.TrimSpace(detail + "\n" + responseBody)
			}
			return nil, p.classifyForbiddenResponse(resp.Header.Get("Date"), detail)
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
func (p *Provider) sendConfig(ctx context.Context, conn *websocket.Conn, opts *synthesizeOptions, allowUndeclaredProbeFormat bool) error {
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

	resolveFormat := p.resolveOutputFormat
	if allowUndeclaredProbeFormat {
		resolveFormat = p.resolveProbeOutputFormat
	}

	outputFormat, err := resolveFormat(opts.OutputFormat)
	if err != nil {
		return err
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
func (p *Provider) sendSSML(ctx context.Context, conn *websocket.Conn, opts *synthesizeOptions) error {
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

// receiveAudio receives audio data and handles metadata.
// Boundary offsets are chunk-local. Callers should use ChunkIndex to rebuild a
// cross-chunk timeline if needed.
func (p *Provider) receiveAudio(ctx context.Context, conn *websocket.Conn, opts *synthesizeOptions, chunkIndex int) ([]byte, error) {
	state := audioReceiveState{}
	receiveTimeout := p.receiveTimeout

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		messageType, message, err := readMessageWithTimeout(ctx, conn, receiveTimeout)
		if err != nil {
			return handleReceiveAudioReadError(err, state, ctx.Err(), receiveTimeout)
		}

		turnEnded, err := p.handleReceiveAudioMessage(messageType, message, opts, chunkIndex, &state)
		if err != nil {
			return nil, err
		}
		if turnEnded {
			return finishReceivedAudio(state, nil)
		}
	}
}

type audioReceiveState struct {
	audioData       []byte
	audioReceived   bool
	boundaryEmitted bool
}

func readMessageWithTimeout(ctx context.Context, conn *websocket.Conn, receiveTimeout time.Duration) (websocket.MessageType, []byte, error) {
	readCtx, readCancel := context.WithTimeout(ctx, receiveTimeout)
	defer readCancel()
	return conn.Read(readCtx)
}

func handleReceiveAudioReadError(err error, state audioReceiveState, parentCtxErr error, receiveTimeout time.Duration) ([]byte, error) {
	var ce websocket.CloseError
	if errors.As(err, &ce) && ce.Code == websocket.StatusNormalClosure {
		return finishReceivedAudio(state, err)
	}
	if state.boundaryEmitted {
		return nil, boundaryEmissionConflictError(err)
	}
	return nil, classifyWebsocketReadError(err, parentCtxErr, receiveTimeout, "websocket read error while receiving audio")
}

func (p *Provider) handleReceiveAudioMessage(messageType websocket.MessageType, message []byte, opts *synthesizeOptions, chunkIndex int, state *audioReceiveState) (bool, error) {
	switch messageType {
	case websocket.MessageBinary:
		appendReceivedAudio(state, message)
		return false, nil
	case websocket.MessageText:
		return p.handleReceiveAudioTextMessage(message, opts, chunkIndex, state)
	default:
		return false, nil
	}
}

func appendReceivedAudio(state *audioReceiveState, message []byte) {
	if audioChunk := extractAudioData(message); len(audioChunk) > 0 {
		state.audioData = append(state.audioData, audioChunk...)
		state.audioReceived = true
	}
}

func (p *Provider) handleReceiveAudioTextMessage(message []byte, opts *synthesizeOptions, chunkIndex int, state *audioReceiveState) (bool, error) {
	msg := string(message)
	if opts.BoundaryCallback != nil && strings.Contains(msg, "Path:audio.metadata") {
		if p.parseAndCallbackMetadata(message, opts.BoundaryCallback, chunkIndex) {
			state.boundaryEmitted = true
		}
	}
	if strings.Contains(msg, "Path:turn.end") {
		return true, nil
	}
	return false, nil
}

func finishReceivedAudio(state audioReceiveState, err error) ([]byte, error) {
	if state.audioReceived {
		return state.audioData, nil
	}
	if state.boundaryEmitted {
		return nil, boundaryEmissionConflictError(err)
	}
	return nil, noAudioReceivedError()
}

// parseAndCallbackMetadata parses metadata and invokes callback.
// It returns true when at least one boundary event was emitted.
func (p *Provider) parseAndCallbackMetadata(message []byte, callback func(tts.BoundaryEvent), chunkIndex int) bool {
	parts := strings.Split(string(message), "\r\n\r\n")
	if len(parts) < 2 {
		return false
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
		return false
	}

	emitted := false
	for _, meta := range metadata.Metadata {
		if meta.Type == "WordBoundary" || meta.Type == "SentenceBoundary" {
			event := tts.BoundaryEvent{
				Type:       meta.Type,
				Text:       html.UnescapeString(meta.Data.Text.Text),
				Offset:     time.Duration(meta.Data.Offset) * 100,
				Duration:   time.Duration(meta.Data.Duration) * 100,
				OffsetMs:   meta.Data.Offset / 10000,
				DurationMs: meta.Data.Duration / 10000,
				ChunkIndex: chunkIndex,
			}
			callback(event)
			emitted = true
		}
	}
	return emitted
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

	if detail, ok := unsupportedOutputFormatDetail(err); ok {
		return &tts.Error{
			Code:     tts.ErrCodeUnsupportedFormat,
			Message:  fmt.Sprintf("service rejected output format: %s", detail),
			Provider: providerName,
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
