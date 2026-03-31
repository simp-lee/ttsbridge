package tts

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	canonicalPCMBytesPerSample = 2
	canonicalPCMBitsPerSample  = canonicalPCMBytesPerSample * 8
)

const (
	MaxWAVUint16 = 1<<16 - 1
	MaxWAVUint32 = 1<<32 - 1
)

// PCMToWAV wraps raw 16-bit PCM data in a canonical WAV container.
func PCMToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) ([]byte, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("tts: sample rate must be > 0")
	}
	if channels <= 0 {
		return nil, fmt.Errorf("tts: channels must be > 0")
	}
	if bitsPerSample != canonicalPCMBitsPerSample {
		return nil, fmt.Errorf("tts: pcm helper only supports %d-bit samples", canonicalPCMBitsPerSample)
	}

	if channels > MaxWAVUint16 {
		return nil, fmt.Errorf("tts: audio profile exceeds wav header limits")
	}

	dataSize := uint64(len(pcm))
	if dataSize > MaxWAVUint32 {
		return nil, fmt.Errorf("tts: pcm data exceeds wav size limit")
	}

	blockAlign := uint64(channels) * uint64(bitsPerSample) / 8
	if blockAlign == 0 || blockAlign > MaxWAVUint16 {
		return nil, fmt.Errorf("tts: computed block align exceeds wav header limits")
	}
	if dataSize%blockAlign != 0 {
		return nil, fmt.Errorf("tts: pcm data must contain whole audio frames")
	}
	if uint64(sampleRate) > MaxWAVUint32 {
		return nil, fmt.Errorf("tts: sample rate exceeds wav header limits")
	}
	byteRate := uint64(sampleRate) * blockAlign
	if byteRate > MaxWAVUint32 {
		return nil, fmt.Errorf("tts: computed byte rate exceeds wav header limits")
	}
	riffSize := dataSize + 36
	if riffSize > MaxWAVUint32 {
		return nil, fmt.Errorf("tts: wav data exceeds riff size limit")
	}

	result := make([]byte, 44+len(pcm))
	copy(result[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(result[4:8], uint32(riffSize))
	copy(result[8:12], []byte("WAVE"))
	copy(result[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(result[16:20], 16)
	binary.LittleEndian.PutUint16(result[20:22], 1)
	binary.LittleEndian.PutUint16(result[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(result[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(result[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(result[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(result[34:36], uint16(bitsPerSample))
	copy(result[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(result[40:44], uint32(dataSize))
	copy(result[44:], pcm)

	return result, nil
}

// InferDuration derives duration from audio bytes and a known audio profile.
func InferDuration(audio []byte, profile VoiceAudioProfile) (time.Duration, error) {
	if len(audio) == 0 {
		return 0, nil
	}

	switch normalizeAudioFormat(profile.Format) {
	case AudioFormatWAV:
		_, duration, err := inspectWAV(audio)
		return duration, err
	case AudioFormatPCM:
		channels := profile.Channels
		if channels <= 0 {
			channels = 1
		}
		if profile.SampleRate <= 0 {
			return 0, fmt.Errorf("tts: sample rate required for pcm duration inference")
		}
		bytesPerFrame := channels * canonicalPCMBytesPerSample
		if bytesPerFrame <= 0 {
			return 0, fmt.Errorf("tts: invalid pcm channel count")
		}
		if len(audio)%bytesPerFrame != 0 {
			return 0, fmt.Errorf("tts: pcm data must contain whole audio frames")
		}
		frames := float64(len(audio)) / float64(bytesPerFrame)
		seconds := frames / float64(profile.SampleRate)
		return time.Duration(seconds * float64(time.Second)), nil
	case AudioFormatMP3:
		if profile.Bitrate <= 0 {
			return 0, fmt.Errorf("tts: bitrate required for mp3 duration inference")
		}
		seconds := float64(len(audio)*8) / float64(profile.Bitrate*1000)
		return time.Duration(seconds * float64(time.Second)), nil
	default:
		return 0, fmt.Errorf("tts: unsupported format %q for duration inference", profile.Format)
	}
}

func inspectWAV(audio []byte) (VoiceAudioProfile, time.Duration, error) {
	if len(audio) < 44 {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: wav data is too short")
	}
	if string(audio[0:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: invalid wav header")
	}
	if binary.LittleEndian.Uint32(audio[16:20]) != 16 {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: unsupported wav fmt chunk size")
	}

	channels := int(binary.LittleEndian.Uint16(audio[22:24]))
	sampleRate := int(binary.LittleEndian.Uint32(audio[24:28]))
	byteRate := int(binary.LittleEndian.Uint32(audio[28:32]))
	bitsPerSample := int(binary.LittleEndian.Uint16(audio[34:36]))
	dataSize := int(binary.LittleEndian.Uint32(audio[40:44]))
	actualDataSize := len(audio) - 44

	if channels <= 0 || sampleRate <= 0 || bitsPerSample <= 0 || bitsPerSample%8 != 0 {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: invalid wav audio profile")
	}
	if dataSize != actualDataSize {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: invalid wav data size")
	}
	if byteRate <= 0 {
		return VoiceAudioProfile{}, 0, fmt.Errorf("tts: invalid wav byte rate")
	}

	duration := time.Duration(float64(dataSize) / float64(byteRate) * float64(time.Second))
	profile := VoiceAudioProfile{
		Format:     AudioFormatWAV,
		SampleRate: sampleRate,
		Channels:   channels,
		Lossless:   true,
	}
	return profile, duration, nil
}
