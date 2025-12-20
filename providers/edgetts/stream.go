package edgetts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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
}

func (s *edgeAudioStream) resetConnection() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

func (s *edgeAudioStream) nextAudioChunk(receiveTimeout time.Duration) ([]byte, error) {
	// Capture current state for potential retry
	currentChunkIndex := s.chunkIndex
	currentOffsetCompensation := s.offsetCompensation

	result, err := retry.DoWithResult(func() ([]byte, error) {
		// Restore state in case of retry
		s.chunkIndex = currentChunkIndex
		s.offsetCompensation = currentOffsetCompensation

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

			if err := s.conn.SetReadDeadline(time.Now().Add(receiveTimeout)); err != nil {
				s.resetConnection()
				return nil, &tts.Error{
					Code:     tts.ErrCodeWebSocketError,
					Message:  "failed to set read deadline",
					Provider: providerName,
					Err:      err,
				}
			}

			messageType, message, err := s.conn.ReadMessage()
			if err != nil {
				readErr := classifyWebsocketReadError(err, receiveTimeout, "websocket read error in stream")
				s.resetConnection()
				return nil, readErr
			}

			switch messageType {
			case websocket.BinaryMessage:
				if audioChunk := extractAudioData(message); len(audioChunk) > 0 {
					return audioChunk, nil
				}
			case websocket.TextMessage:
				msgStr := string(message)

				if s.opts.MetadataCallback != nil && strings.Contains(msgStr, "Path:audio.metadata") {
					s.provider.parseAndCallbackMetadata(message, s.opts.MetadataCallback, s.offsetCompensation)
				}

				if strings.Contains(msgStr, "Path:turn.end") {
					s.resetConnection()
					// Move to next chunk
					s.chunkIndex++
					s.offsetCompensation += defaultOffsetPadding

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
		conn.Close()
		return err
	}

	if err := s.provider.sendSSML(s.ctx, conn, &chunkOpts); err != nil {
		conn.Close()
		return err
	}

	// Close old connection before replacing
	if s.conn != nil {
		s.conn.Close()
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
	err := s.conn.Close()
	s.conn = nil
	return err
}
