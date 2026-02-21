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

// edgeAudioStream implements streaming audio reader
type edgeAudioStream struct {
	conn               *websocket.Conn
	ctx                context.Context
	closed             bool
	opts               *SynthesizeOptions
	provider           *Provider
	textChunks         []string
	chunkIndex         int
	offsetCompensation int64
	chunkBytesEmitted  int
}

func (s *edgeAudioStream) resetConnection() {
	if s.conn != nil {
		s.conn.CloseNow()
		s.conn = nil
	}
}

func (s *edgeAudioStream) nextAudioChunk(receiveTimeout time.Duration) ([]byte, error) {
	// Capture current state for potential retry
	currentChunkIndex := s.chunkIndex
	currentOffsetCompensation := s.offsetCompensation
	currentChunkBytesEmitted := s.chunkBytesEmitted

	result, err := retry.DoWithResult(func() ([]byte, error) {
		// Restore state in case of retry
		s.chunkIndex = currentChunkIndex
		s.offsetCompensation = currentOffsetCompensation
		s.chunkBytesEmitted = currentChunkBytesEmitted

		// Check if all chunks are processed
		if s.chunkIndex >= len(s.textChunks) {
			return nil, io.EOF
		}

		// Initialize connection for current chunk if needed
		if s.conn == nil {
			if err := s.initializeChunk(); err != nil {
				return nil, err
			}
		}

		for {
			select {
			case <-s.ctx.Done():
				return nil, s.ctx.Err()
			default:
			}

			readCtx, readCancel := context.WithTimeout(s.ctx, receiveTimeout)
			messageType, message, err := s.conn.Read(readCtx)
			readCancel()
			if err != nil {
				readErr := s.handleReadError(err, receiveTimeout)
				s.resetConnection()
				return nil, readErr
			}

			switch messageType {
			case websocket.MessageBinary:
				if audioChunk := extractAudioData(message); len(audioChunk) > 0 {
					s.chunkBytesEmitted += len(audioChunk)
					return audioChunk, nil
				}
			case websocket.MessageText:
				msgStr := string(message)

				if s.opts.BoundaryCallback != nil && strings.Contains(msgStr, "Path:audio.metadata") {
					s.provider.parseAndCallbackMetadata(message, s.opts.BoundaryCallback, s.offsetCompensation, s.chunkIndex)
				}

				if strings.Contains(msgStr, "Path:turn.end") {
					s.resetConnection()
					// Move to next chunk
					s.chunkIndex++
					s.offsetCompensation += defaultOffsetPadding
					s.chunkBytesEmitted = 0

					// Check if more chunks to process
					if s.chunkIndex >= len(s.textChunks) {
						return nil, io.EOF
					}

					// Initialize next chunk
					if err := s.initializeChunk(); err != nil {
						return nil, err
					}
					continue
				}
			}
		}
	}, tts.RetryOptions(s.ctx, s.provider.maxAttempts)...)

	return result, err
}

func (s *edgeAudioStream) handleReadError(err error, receiveTimeout time.Duration) error {
	var parentCtxErr error
	if s.ctx != nil {
		parentCtxErr = s.ctx.Err()
	}
	if errors.Is(parentCtxErr, context.Canceled) || errors.Is(parentCtxErr, context.DeadlineExceeded) {
		return parentCtxErr
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

	audioChunk, err := s.nextAudioChunk(receiveTimeout)
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

	if err := s.provider.sendConfig(s.ctx, conn, s.opts); err != nil {
		conn.CloseNow()
		return err
	}

	if err := s.provider.sendSSML(s.ctx, conn, &chunkOpts); err != nil {
		conn.CloseNow()
		return err
	}

	// Close old connection before replacing
	if s.conn != nil {
		s.conn.CloseNow()
	}
	s.conn = conn
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
