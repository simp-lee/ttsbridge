package audio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

func init() {
	tts.RegisterVoiceProfileResolver(testProviderEdge, func(string) (tts.VoiceAudioProfile, bool) {
		return tts.VoiceAudioProfile{
			Format:     tts.AudioFormatMP3,
			SampleRate: tts.SampleRate24kHz,
			Channels:   1,
			Bitrate:    48,
		}, true
	})

	tts.RegisterVoiceProfileResolver(testProviderEdgeAlias, func(string) (tts.VoiceAudioProfile, bool) {
		return tts.VoiceAudioProfile{
			Format:     tts.AudioFormatMP3,
			SampleRate: tts.SampleRate24kHz,
			Channels:   1,
			Bitrate:    48,
		}, true
	})

	tts.RegisterVoiceProfileResolver(testProviderVolcengine, func(string) (tts.VoiceAudioProfile, bool) {
		return tts.VoiceAudioProfile{
			Format:     tts.AudioFormatWAV,
			SampleRate: 24000,
			Channels:   1,
			Lossless:   true,
		}, true
	})
}

// TestAudioQualityComparison 音质对比测试
// 生成不同格式和质量的混音文件，用于人工对比
func TestAudioQualityComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过音质对比测试")
	}

	if !IsFFmpegInstalled() {
		t.Skip("跳过音质对比测试：ffmpeg 未安装")
	}

	// 准备测试音频数据（模拟 TTS 输出）
	testVoiceAudio := generateTestWAV(t, 3*time.Second, 440.0) // 3秒 440Hz 正弦波

	// 创建输出目录
	outputDir := "test_audio_quality_output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("创建输出目录失败: %v", err)
	}
	t.Logf("输出目录: %s", outputDir)

	// 准备背景音乐（如果有的话，这里用测试音频代替）
	testMusicPath := filepath.Join(outputDir, "test_music.mp3")
	testMusicAudio := generateTestWAV(t, 10*time.Second, 220.0) // 10秒 220Hz 正弦波
	if err := os.WriteFile(testMusicPath, testMusicAudio, 0644); err != nil {
		t.Fatalf("写入测试音乐失败: %v", err)
	}

	ctx := context.Background()

	// 测试不同的输出格式和质量
	testCases := []struct {
		name         string
		provider     string
		voiceID      string
		description  string
		expectedSize string
	}{
		{
			name:         "edgetts_mp3_48k",
			provider:     testProviderEdge,
			voiceID:      testVoiceEdgeDefault,
			description:  "EdgeTTS MP3 48kbps - 语音质量",
			expectedSize: "~150KB",
		},
		{
			name:         "volcengine_wav_lossless",
			provider:     testProviderVolcengine,
			voiceID:      testVoiceVolcDefault,
			description:  "Volcengine WAV 无损 - 最高音质",
			expectedSize: "~900KB",
		},
	}

	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("音质对比测试")
	t.Log(strings.Repeat("=", 80))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := &tts.BackgroundMusicOptions{
				MusicPath:       testMusicPath,
				Volume:          0.3,
				MainAudioVolume: 1.0,
			}

			startTime := time.Now()
			mixedAudio, err := MixWithBackgroundMusic(ctx, testVoiceAudio, tc.provider, tc.voiceID, opts, nil)
			duration := time.Since(startTime)

			if err != nil {
				t.Fatalf("混音失败: %v", err)
			}

			// 保存输出文件
			outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.bin", tc.name))
			if err := os.WriteFile(outputPath, mixedAudio, 0644); err != nil {
				t.Fatalf("保存输出文件失败: %v", err)
			}

			fileSize := len(mixedAudio)
			t.Logf("✓ %s", tc.description)
			t.Logf("  文件: %s", outputPath)
			t.Logf("  大小: %d 字节 (%.2f KB) %s", fileSize, float64(fileSize)/1024, tc.expectedSize)
			t.Logf("  耗时: %v", duration)
		})
	}

	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("测试完成！音质完全由 TTS 语音决定")
	t.Log(outputDir)
	t.Log(strings.Repeat("=", 80))
}

// generateTestWAV 生成测试用的 WAV 音频（正弦波）
func generateTestWAV(t *testing.T, duration time.Duration, frequency float64) []byte {
	t.Helper()
	// 这里简化处理，实际应该生成真实的 WAV 数据
	// 为了测试，我们返回一个最小的 WAV header
	sampleRate := 24000
	numSamples := int(duration.Seconds() * float64(sampleRate))
	dataSize := numSamples * 2 // 16-bit samples

	// WAV header (44 bytes)
	header := []byte{
		// RIFF chunk
		'R', 'I', 'F', 'F',
		byte((dataSize + 36) & 0xff), byte((dataSize + 36) >> 8 & 0xff),
		byte((dataSize + 36) >> 16 & 0xff), byte((dataSize + 36) >> 24 & 0xff),
		'W', 'A', 'V', 'E',
		// fmt chunk
		'f', 'm', 't', ' ',
		16, 0, 0, 0, // fmt chunk size
		1, 0, // PCM format
		1, 0, // mono
		byte(sampleRate & 0xff), byte(sampleRate >> 8 & 0xff),
		byte(sampleRate >> 16 & 0xff), byte(sampleRate >> 24 & 0xff),
		byte((sampleRate * 2) & 0xff), byte((sampleRate * 2) >> 8 & 0xff),
		byte((sampleRate * 2) >> 16 & 0xff), byte((sampleRate * 2) >> 24 & 0xff),
		2, 0, // block align
		16, 0, // 16 bits per sample
		// data chunk
		'd', 'a', 't', 'a',
		byte(dataSize & 0xff), byte(dataSize >> 8 & 0xff),
		byte(dataSize >> 16 & 0xff), byte(dataSize >> 24 & 0xff),
	}

	// 生成正弦波数据
	data := make([]byte, dataSize)
	for i := 0; i < numSamples; i++ {
		// 生成正弦波样本
		t := float64(i) / float64(sampleRate)
		sample := int16(32767 * 0.5 * sinApprox(2*3.14159*frequency*t))
		data[i*2] = byte(sample & 0xff)
		data[i*2+1] = byte(sample >> 8)
	}

	return append(header, data...)
}

// sinApprox 快速正弦近似
func sinApprox(x float64) float64 {
	// 简化的正弦近似，仅用于测试
	for x > 3.14159 {
		x -= 2 * 3.14159
	}
	for x < -3.14159 {
		x += 2 * 3.14159
	}
	if x < 0 {
		return 1.27323954*x + 0.405284735*x*x
	}
	return 1.27323954*x - 0.405284735*x*x
}

// TestVoiceAudioProfile 测试 TTS 提供商的音质配置
func TestVoiceAudioProfile(t *testing.T) {
	testCases := []struct {
		name               string
		provider           string
		voiceID            string
		expectedFormat     string
		expectedSampleRate int
		expectedChannels   int
		expectedLossless   bool
	}{
		{
			name:               "EdgeTTS 默认配置",
			provider:           testProviderEdge,
			voiceID:            testVoiceEdgeDefault,
			expectedFormat:     tts.AudioFormatMP3,
			expectedSampleRate: tts.SampleRate24kHz,
			expectedChannels:   1,
			expectedLossless:   false,
		},
		{
			name:               "Volcengine 默认配置",
			provider:           testProviderVolcengine,
			voiceID:            testVoiceVolcDefault,
			expectedFormat:     tts.AudioFormatWAV,
			expectedSampleRate: 24000,
			expectedChannels:   1,
			expectedLossless:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile, found := tts.LookupVoiceAudioProfile(tc.provider, tc.voiceID)
			if !found {
				t.Fatalf("未找到 %s 的音质配置", tc.provider)
			}

			if profile.Format != tc.expectedFormat {
				t.Errorf("格式不匹配: got %s, want %s", profile.Format, tc.expectedFormat)
			}
			if profile.SampleRate != tc.expectedSampleRate {
				t.Errorf("采样率不匹配: got %d, want %d", profile.SampleRate, tc.expectedSampleRate)
			}
			if profile.Channels != tc.expectedChannels {
				t.Errorf("声道数不匹配: got %d, want %d", profile.Channels, tc.expectedChannels)
			}
			if profile.Lossless != tc.expectedLossless {
				t.Errorf("无损标志不匹配: got %v, want %v", profile.Lossless, tc.expectedLossless)
			}
		})
	}
}

// TestBuildOutputConfig 测试输出配置构建
func TestBuildOutputConfig(t *testing.T) {
	testCases := []struct {
		name             string
		profile          tts.VoiceAudioProfile
		expectCodec      string
		expectFormat     string
		expectSampleRate string
		expectChannels   string
	}{
		{
			name: "MP3 格式",
			profile: tts.VoiceAudioProfile{
				Format:     tts.AudioFormatMP3,
				SampleRate: 24000,
				Channels:   1,
				Bitrate:    48,
			},
			expectCodec:      "libmp3lame",
			expectFormat:     "mp3",
			expectSampleRate: "24000",
			expectChannels:   "1",
		},
		{
			name: "WAV 无损",
			profile: tts.VoiceAudioProfile{
				Format:     tts.AudioFormatWAV,
				SampleRate: 24000,
				Channels:   1,
				Lossless:   true,
			},
			expectCodec:      "pcm_s16le",
			expectFormat:     "wav",
			expectSampleRate: "24000",
			expectChannels:   "1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildOutputConfig(&tc.profile)
			if args == nil {
				t.Fatal("buildOutputConfig 返回 nil")
			}

			// Helper to find value for a flag in args slice
			findArg := func(flag string) (string, bool) {
				for i, v := range args {
					if v == flag && i+1 < len(args) {
						return args[i+1], true
					}
				}
				return "", false
			}

			if codec, ok := findArg("-codec:a"); !ok || codec != tc.expectCodec {
				t.Errorf("编解码器不匹配: got %s, want %s", codec, tc.expectCodec)
			}
			if f, ok := findArg("-f"); !ok || f != tc.expectFormat {
				t.Errorf("格式不匹配: got %s, want %s", f, tc.expectFormat)
			}
			if ar, ok := findArg("-ar"); !ok || ar != tc.expectSampleRate {
				t.Errorf("采样率不匹配: got %s, want %s", ar, tc.expectSampleRate)
			}
			if ac, ok := findArg("-ac"); !ok || ac != tc.expectChannels {
				t.Errorf("声道数不匹配: got %s, want %s", ac, tc.expectChannels)
			}
		})
	}
}
