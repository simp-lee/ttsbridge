package tts

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func assertWAVHeader(t *testing.T, wav []byte, sampleRate, channels, bitsPerSample, dataSize int) {
	t.Helper()

	if got := string(wav[0:4]); got != "RIFF" {
		t.Fatalf("riff header = %q, want %q", got, "RIFF")
	}
	if got := string(wav[8:12]); got != "WAVE" {
		t.Fatalf("wave header = %q, want %q", got, "WAVE")
	}
	if got := int(binary.LittleEndian.Uint16(wav[22:24])); got != channels {
		t.Fatalf("channels = %d, want %d", got, channels)
	}
	if got := int(binary.LittleEndian.Uint32(wav[24:28])); got != sampleRate {
		t.Fatalf("sample rate = %d, want %d", got, sampleRate)
	}
	if got := int(binary.LittleEndian.Uint16(wav[34:36])); got != bitsPerSample {
		t.Fatalf("bits per sample = %d, want %d", got, bitsPerSample)
	}
	if got := int(binary.LittleEndian.Uint32(wav[40:44])); got != dataSize {
		t.Fatalf("data size = %d, want %d", got, dataSize)
	}
	if got := string(wav[36:40]); got != "data" {
		t.Fatalf("data chunk header = %q, want %q", got, "data")
	}
}

func TestPCMToWAV_CorrectInputProducesCanonicalWAVBytes(t *testing.T) {
	const (
		sampleRate    = 24000
		channels      = 1
		bitsPerSample = 16
	)

	pcm := make([]byte, sampleRate*channels*(bitsPerSample/8))
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}

	wav, err := PCMToWAV(pcm, sampleRate, channels, bitsPerSample)
	if err != nil {
		t.Fatalf("PCMToWAV() error: %v", err)
	}

	assertWAVHeader(t, wav, sampleRate, channels, bitsPerSample, len(pcm))
	if !bytes.Equal(wav[44:], pcm) {
		t.Fatal("wav payload does not match input pcm")
	}
}

func TestPCMToWAV_EmptyInputProducesHeaderOnlyWAV(t *testing.T) {
	wav, err := PCMToWAV([]byte{}, 24000, 1, 16)
	if err != nil {
		t.Fatalf("PCMToWAV() error: %v", err)
	}

	if len(wav) != 44 {
		t.Fatalf("len(wav) = %d, want 44", len(wav))
	}
	assertWAVHeader(t, wav, 24000, 1, 16, 0)
}

func TestPCMToWAV_InvalidParametersReturnErrors(t *testing.T) {
	tests := []struct {
		name          string
		sampleRate    int
		channels      int
		bitsPerSample int
		wantErr       string
	}{
		{
			name:          "sample rate",
			sampleRate:    0,
			channels:      1,
			bitsPerSample: 16,
			wantErr:       "sample rate",
		},
		{
			name:          "channels",
			sampleRate:    24000,
			channels:      -1,
			bitsPerSample: 16,
			wantErr:       "channels",
		},
		{
			name:          "8-bit pcm not supported",
			sampleRate:    24000,
			channels:      1,
			bitsPerSample: 8,
			wantErr:       "16-bit",
		},
		{
			name:          "24-bit pcm not supported",
			sampleRate:    24000,
			channels:      1,
			bitsPerSample: 24,
			wantErr:       "16-bit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PCMToWAV([]byte{1, 2, 3, 4}, tt.sampleRate, tt.channels, tt.bitsPerSample)
			if err == nil {
				t.Fatal("PCMToWAV() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("PCMToWAV() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPCMToWAV_RejectsFrameMisalignedPCM(t *testing.T) {
	tests := []struct {
		name          string
		pcm           []byte
		channels      int
		bitsPerSample int
	}{
		{
			name:          "mono 16-bit odd byte count",
			pcm:           []byte{0x01, 0x02, 0x03},
			channels:      1,
			bitsPerSample: 16,
		},
		{
			name:          "stereo 16-bit truncated frame",
			pcm:           []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			channels:      2,
			bitsPerSample: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PCMToWAV(tt.pcm, 24000, tt.channels, tt.bitsPerSample)
			if err == nil {
				t.Fatal("PCMToWAV() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), "frame") {
				t.Fatalf("PCMToWAV() error = %q, want frame-alignment error", err.Error())
			}
		})
	}
}

func TestInferDuration(t *testing.T) {
	t.Run("wav derives duration from final returned bytes", func(t *testing.T) {
		const (
			sampleRate    = 24000
			channels      = 1
			bitsPerSample = 16
		)

		pcm := make([]byte, sampleRate*channels*(bitsPerSample/8))
		wav, err := PCMToWAV(pcm, sampleRate, channels, bitsPerSample)
		if err != nil {
			t.Fatalf("PCMToWAV() error: %v", err)
		}

		duration, err := InferDuration(wav, VoiceAudioProfile{
			Format:     AudioFormatWAV,
			SampleRate: 1,
			Channels:   99,
		})
		if err != nil {
			t.Fatalf("InferDuration() error: %v", err)
		}
		if duration != time.Second {
			t.Fatalf("InferDuration() = %v, want %v", duration, time.Second)
		}
	})

	t.Run("pcm derives duration from raw pcm bytes", func(t *testing.T) {
		const (
			sampleRate = 24000
			channels   = 2
		)

		pcm := make([]byte, sampleRate*channels*canonicalPCMBytesPerSample)
		duration, err := InferDuration(pcm, VoiceAudioProfile{
			Format:     AudioFormatPCM,
			SampleRate: sampleRate,
			Channels:   channels,
		})
		if err != nil {
			t.Fatalf("InferDuration() error: %v", err)
		}
		if duration != time.Second {
			t.Fatalf("InferDuration() = %v, want %v", duration, time.Second)
		}
	})

	t.Run("pcm rejects partial audio frames", func(t *testing.T) {
		_, err := InferDuration([]byte{0x01, 0x02, 0x03}, VoiceAudioProfile{
			Format:     AudioFormatPCM,
			SampleRate: 24000,
			Channels:   1,
		})
		if err == nil {
			t.Fatal("InferDuration() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "frame") {
			t.Fatalf("InferDuration() error = %q, want frame-alignment error", err.Error())
		}
	})

	t.Run("mp3 derives duration from payload size and bitrate", func(t *testing.T) {
		mp3 := make([]byte, 16000)
		duration, err := InferDuration(mp3, VoiceAudioProfile{
			Format:  AudioFormatMP3,
			Bitrate: 128,
		})
		if err != nil {
			t.Fatalf("InferDuration() error: %v", err)
		}
		if duration != time.Second {
			t.Fatalf("InferDuration() = %v, want %v", duration, time.Second)
		}
	})
}
