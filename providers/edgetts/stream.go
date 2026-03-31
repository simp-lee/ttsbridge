package edgetts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/simp-lee/retry"
	"github.com/simp-lee/ttsbridge/tts"
)

var errChunkCommitted = errors.New("edgetts: chunk committed")

// edgeAudioStream implements streaming audio reader
type edgeAudioStream struct {
	conn              *websocket.Conn
	ctx               context.Context
	closed            bool
	opts              *synthesizeOptions
	provider          *Provider
	textChunks        []string
	chunkIndex        int
	chunkBytesEmitted int
	boundaryEmitted   bool
}

var _ tts.AudioStream = (*edgeAudioStream)(nil)

func (s *edgeAudioStream) resetConnection() {
	if s.conn != nil {
		closeNowIgnoreError(s.conn)
		s.conn = nil
	}
}

type streamChunkProgress struct {
	chunkIndex        int
	chunkBytesEmitted int
	boundaryEmitted   bool
}

func (s *edgeAudioStream) snapshotChunkProgress() streamChunkProgress {
	return streamChunkProgress{
		chunkIndex:        s.chunkIndex,
		chunkBytesEmitted: s.chunkBytesEmitted,
		boundaryEmitted:   s.boundaryEmitted,
	}
}

func (s *edgeAudioStream) restoreChunkProgress(progress streamChunkProgress) {
	s.chunkIndex = progress.chunkIndex
	s.chunkBytesEmitted = progress.chunkBytesEmitted
	s.boundaryEmitted = progress.boundaryEmitted
}

func (s *edgeAudioStream) nextAudioChunk(receiveTimeout time.Duration) ([]byte, error) {
	progress := s.snapshotChunkProgress()
	return retry.DoWithResult(func() ([]byte, error) {
		s.restoreChunkProgress(progress)

		if err := s.prepareCurrentChunk(); err != nil {
			return nil, err
		}

		return s.readChunkLoop(receiveTimeout)
	}, tts.RetryOptions(s.ctx, s.provider.maxAttempts)...)
}

func (s *edgeAudioStream) prepareCurrentChunk() error {
	if s.chunkIndex >= len(s.textChunks) {
		return io.EOF
	}
	if s.conn != nil {
		return nil
	}
	return s.initializeChunk()
}

func (s *edgeAudioStream) readChunkLoop(receiveTimeout time.Duration) ([]byte, error) {
	for {
		if err := s.ctx.Err(); err != nil {
			return nil, err
		}

		messageType, message, err := readMessageWithTimeout(s.ctx, s.conn, receiveTimeout)
		if err != nil {
			return s.failChunkRead(err, receiveTimeout)
		}

		audioChunk, err := s.handleChunkMessage(messageType, message)
		if audioChunk != nil || err != nil {
			return audioChunk, err
		}
	}
}

func (s *edgeAudioStream) failChunkRead(err error, receiveTimeout time.Duration) ([]byte, error) {
	readErr := s.handleReadError(err, receiveTimeout)
	s.resetConnection()
	return nil, readErr
}

func (s *edgeAudioStream) handleChunkMessage(messageType websocket.MessageType, message []byte) ([]byte, error) {
	switch messageType {
	case websocket.MessageBinary:
		return s.handleBinaryChunkMessage(message), nil
	case websocket.MessageText:
		return nil, s.handleTextChunkMessage(message)
	default:
		return nil, nil
	}
}

func (s *edgeAudioStream) handleBinaryChunkMessage(message []byte) []byte {
	audioChunk := extractAudioData(message)
	if len(audioChunk) == 0 {
		return nil
	}

	s.chunkBytesEmitted += len(audioChunk)
	return audioChunk
}

func (s *edgeAudioStream) handleTextChunkMessage(message []byte) error {
	msgStr := string(message)
	s.maybeEmitBoundary(message, msgStr)
	if !strings.Contains(msgStr, "Path:turn.end") {
		return nil
	}
	return s.finishCurrentChunk()
}

func (s *edgeAudioStream) maybeEmitBoundary(message []byte, msgStr string) {
	if s.opts.BoundaryCallback != nil && strings.Contains(msgStr, "Path:audio.metadata") {
		if s.provider.parseAndCallbackMetadata(message, s.opts.BoundaryCallback, s.chunkIndex) {
			s.boundaryEmitted = true
		}
	}
}

func (s *edgeAudioStream) finishCurrentChunk() error {
	if s.chunkBytesEmitted == 0 {
		s.resetConnection()
		if s.boundaryEmitted {
			return boundaryEmissionConflictError(nil)
		}
		return noAudioReceivedError()
	}

	s.resetConnection()
	s.chunkIndex++
	s.chunkBytesEmitted = 0
	s.boundaryEmitted = false
	if s.chunkIndex >= len(s.textChunks) {
		return io.EOF
	}
	return errChunkCommitted
}

func (s *edgeAudioStream) handleReadError(err error, receiveTimeout time.Duration) error {
	var parentCtxErr error
	if s.ctx != nil {
		parentCtxErr = s.ctx.Err()
	}
	if errors.Is(parentCtxErr, context.Canceled) || errors.Is(parentCtxErr, context.DeadlineExceeded) {
		return parentCtxErr
	}

	if s.boundaryEmitted {
		return boundaryEmissionConflictError(err)
	}

	if s.chunkBytesEmitted > 0 {
		return &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  "cannot resume stream after partial chunk emission",
			Provider: providerName,
			Err:      err,
		}
	}

	return classifyWebsocketReadError(err, parentCtxErr, receiveTimeout, "websocket read error in stream")
}

func (s *edgeAudioStream) Read() ([]byte, error) {
	if s.closed {
		return nil, io.EOF
	}

	select {
	case <-s.ctx.Done():
		s.closed = true
		s.resetConnection()
		return nil, s.ctx.Err()
	default:
	}

	receiveTimeout := s.provider.receiveTimeout

	for {
		audioChunk, err := s.nextAudioChunk(receiveTimeout)
		if errors.Is(err, errChunkCommitted) {
			continue
		}
		if err != nil {
			s.closed = true
			s.resetConnection()

			// EOF is normal completion
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}

			// Context errors should be returned as-is
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}

			// Wrap retry errors with more context
			if retry.IsRetryError(err) {
				cause := unwrapRetryCause(err)
				var ttsErr *tts.Error
				if errors.As(cause, &ttsErr) {
					return nil, cause
				}
				return nil, &tts.Error{
					Code:     tts.ErrCodeNetworkError,
					Message:  fmt.Sprintf("stream read failed after %d attempts", s.provider.maxAttempts),
					Provider: providerName,
					Err:      err,
				}
			}

			// Return other errors as-is (already wrapped in nextAudioChunk)
			return nil, err
		}

		return audioChunk, nil
	}
}

// initializeChunk prepares the websocket connection for the current chunk without internal retries.
func (s *edgeAudioStream) initializeChunk() error {
	if s.chunkIndex >= len(s.textChunks) {
		return io.EOF
	}

	chunkOpts := *s.opts
	chunkOpts.Text = s.textChunks[s.chunkIndex]

	conn, err := s.provider.connect(s.ctx)
	if err != nil {
		return err
	}

	if err := s.provider.sendConfig(s.ctx, conn, s.opts, false); err != nil {
		closeNowIgnoreError(conn)
		return err
	}

	if err := s.provider.sendSSML(s.ctx, conn, &chunkOpts); err != nil {
		closeNowIgnoreError(conn)
		return err
	}

	// Close old connection before replacing
	if s.conn != nil {
		closeNowIgnoreError(s.conn)
	}
	s.conn = conn
	s.boundaryEmitted = false
	return nil
}

func (s *edgeAudioStream) Close() error {
	if s.closed {
		return nil
	}

	s.closed = true
	if s.conn == nil {
		return nil
	}
	err := s.conn.CloseNow()
	s.conn = nil
	return err
}
