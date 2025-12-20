package audio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

func TestValidateBackgroundMusicFile(t *testing.T) {
	tests := []struct {
		name      string
		filePath  string
		wantError bool
	}{
		{
			name:      "空路径",
			filePath:  "",
			wantError: true,
		},
		{
			name:      "不存在的文件",
			filePath:  "nonexistent.mp3",
			wantError: true,
		},
		{
			name:      "支持的格式 - mp3",
			filePath:  "test.mp3",
			wantError: false,
		},
		{
			name:      "支持的格式 - wav",
			filePath:  "test.wav",
			wantError: false,
		},
		{
			name:      "不支持的格式",
			filePath:  "test.txt",
			wantError: true,
		},
	}

	// 创建临时目录和测试文件
	tempDir := t.TempDir()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testFilePath string

			// 如果不是空路径或不存在的文件，创建测试文件
			if tt.filePath != "" && tt.filePath != "nonexistent.mp3" {
				testFilePath = filepath.Join(tempDir, tt.filePath)

				// 如果有 ffmpeg 且文件是音频格式，生成真实音频文件
				if IsFFmpegInstalled() && IsSupportedAudioExtension(filepath.Ext(tt.filePath)) {
					// 生成真实的音频文件用于测试
					ext := filepath.Ext(tt.filePath)
					var cmd *exec.Cmd
					switch ext {
					case ".mp3":
						cmd = exec.Command("ffmpeg", "-f", "lavfi", "-i", "anullsrc=r=24000:cl=mono",
							"-t", "0.1", "-c:a", "libmp3lame", "-b:a", "128k", testFilePath, "-y")
					case ".wav":
						cmd = exec.Command("ffmpeg", "-f", "lavfi", "-i", "anullsrc=r=24000:cl=mono",
							"-t", "0.1", "-c:a", "pcm_s16le", testFilePath, "-y")
					}
					if cmd != nil {
						if err := cmd.Run(); err != nil {
							t.Logf("无法生成真实音频文件，使用假文件: %v", err)
							// 回退：创建假文件
							if err := os.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
								t.Fatalf("创建测试文件失败: %v", err)
							}
						}
					}
				} else {
					// 创建一个小的测试文件（假文件）
					if err := os.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
						t.Fatalf("创建测试文件失败: %v", err)
					}
				}
			} else if tt.filePath != "" {
				testFilePath = filepath.Join(tempDir, tt.filePath)
			} else {
				testFilePath = tt.filePath
			}

			err := ValidateBackgroundMusicFile(testFilePath)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateBackgroundMusicFile() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSaveUploadedFile(t *testing.T) {
	testData := []byte("test audio data")

	t.Run("使用相对文件名", func(t *testing.T) {
		filename := "test_music.mp3"

		// 保存文件
		savedPath, err := SaveUploadedFile(testData, filename)
		if err != nil {
			t.Fatalf("SaveUploadedFile() error = %v", err)
		}

		// 清理
		defer os.Remove(savedPath)

		// 验证文件存在
		if _, err := os.Stat(savedPath); os.IsNotExist(err) {
			t.Error("文件未创建")
		}

		// 验证文件内容
		content, err := os.ReadFile(savedPath)
		if err != nil {
			t.Fatalf("读取保存的文件失败: %v", err)
		}

		if string(content) != string(testData) {
			t.Errorf("文件内容不匹配: got %v, want %v", string(content), string(testData))
		}
	})

	t.Run("使用绝对路径", func(t *testing.T) {
		tempDir := t.TempDir()
		filename := "test_music.mp3"
		fullPath := filepath.Join(tempDir, filename)

		// 保存文件
		savedPath, err := SaveUploadedFile(testData, fullPath)
		if err != nil {
			t.Fatalf("SaveUploadedFile() error = %v", err)
		}

		// 验证返回的路径
		if savedPath != fullPath {
			t.Errorf("SaveUploadedFile() path = %v, want %v", savedPath, fullPath)
		}

		// 验证文件内容
		content, err := os.ReadFile(savedPath)
		if err != nil {
			t.Fatalf("读取保存的文件失败: %v", err)
		}

		if string(content) != string(testData) {
			t.Errorf("文件内容不匹配: got %v, want %v", string(content), string(testData))
		}
	})
}

func TestMixWithBackgroundMusic_NilOptions(t *testing.T) {
	voiceAudio := []byte("test voice audio")
	ctx := context.Background()

	_, err := MixWithBackgroundMusic(ctx, voiceAudio, testProviderEdge, "", nil)
	if err == nil {
		t.Error("MixWithBackgroundMusic() 应该返回错误当选项为 nil")
	}
}

func TestMixWithBackgroundMusic_EmptyMusicPath(t *testing.T) {
	voiceAudio := []byte("test voice audio")
	ctx := context.Background()
	opts := &tts.BackgroundMusicOptions{
		MusicPath: "",
	}

	_, err := MixWithBackgroundMusic(ctx, voiceAudio, testProviderEdge, "", opts)
	if err == nil {
		t.Error("MixWithBackgroundMusic() 应该返回错误当音乐路径为空")
	}
}

func TestMixWithBackgroundMusic_NonExistentFile(t *testing.T) {
	voiceAudio := []byte("test voice audio")
	ctx := context.Background()
	opts := &tts.BackgroundMusicOptions{
		MusicPath: "nonexistent_music.mp3",
	}

	_, err := MixWithBackgroundMusic(ctx, voiceAudio, testProviderEdge, "", opts)
	if err == nil {
		t.Error("MixWithBackgroundMusic() 应该返回错误当文件不存在")
	}
}

// 注意：实际的混音测试需要 ffmpeg 安装，这里只测试错误情况
// 完整的集成测试应该在有 ffmpeg 的环境中运行

// TestMixWithBackgroundMusic_Integration 集成测试（需要 ffmpeg）
func TestMixWithBackgroundMusic_Integration(t *testing.T) {
	// 检查 ffmpeg 是否可用
	if !IsFFmpegInstalled() {
		t.Skip("跳过集成测试：ffmpeg 未安装")
	}

	// 创建临时目录
	tempDir := t.TempDir()

	// 创建一个简单的测试音频文件（使用 ffmpeg 生成静音）
	voiceFile := filepath.Join(tempDir, "voice.mp3")
	musicFile := filepath.Join(tempDir, "music.mp3")

	// 生成 2 秒音调作为语音（500Hz 正弦波）
	cmdVoice := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "sine=frequency=500:duration=2:sample_rate=24000",
		"-c:a", "libmp3lame",
		"-b:a", "48k",
		voiceFile,
		"-y",
	)
	if err := cmdVoice.Run(); err != nil {
		t.Skip("无法生成测试音频文件")
	}

	// 生成 5 秒音调作为背景音乐（300Hz 正弦波）
	cmdMusic := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "sine=frequency=300:duration=5:sample_rate=24000",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		musicFile,
		"-y",
	)
	if err := cmdMusic.Run(); err != nil {
		t.Skip("无法生成测试音频文件")
	}

	// 读取语音文件
	voiceData, err := os.ReadFile(voiceFile)
	if err != nil {
		t.Fatalf("读取语音文件失败: %v", err)
	}

	// 测试混音
	ctx := context.Background()
	loopEnabled := true
	opts := &tts.BackgroundMusicOptions{
		MusicPath:       musicFile,
		Volume:          0.3,
		FadeIn:          0.5,
		FadeOut:         0.5,
		StartTime:       0,
		Loop:            &loopEnabled,
		MainAudioVolume: 1.0,
	}

	mixedAudio, err := MixWithBackgroundMusic(ctx, voiceData, testProviderEdge, "", opts)
	if err != nil {
		t.Fatalf("混音失败: %v", err)
	}

	// 验证输出
	if len(mixedAudio) == 0 {
		t.Error("混音后的音频数据为空")
	}

	// 验证输出大小合理（应该大于原始语音）
	if len(mixedAudio) < len(voiceData)/2 {
		t.Error("混音后的音频数据异常小")
	}
}

// TestDefaultValues 测试零值字段的默认值处理
func TestDefaultValues(t *testing.T) {
	// 检查 ffmpeg 是否可用
	if !IsFFmpegInstalled() {
		t.Skip("跳过测试：ffmpeg 未安装")
	}

	// 创建临时目录
	tempDir := t.TempDir()

	// 创建测试音频文件
	voiceFile := filepath.Join(tempDir, "voice.mp3")
	musicFile := filepath.Join(tempDir, "music.mp3")

	// 生成 1 秒静音作为语音
	cmdVoice := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "anullsrc=r=24000:cl=mono",
		"-t", "1",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		voiceFile,
	)
	if err := cmdVoice.Run(); err != nil {
		t.Skip("无法生成测试音频文件")
	}

	// 生成 3 秒静音作为背景音乐
	cmdMusic := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "anullsrc=r=24000:cl=mono",
		"-t", "3",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		musicFile,
	)
	if err := cmdMusic.Run(); err != nil {
		t.Skip("无法生成测试音频文件")
	}

	// 读取语音文件
	voiceData, err := os.ReadFile(voiceFile)
	if err != nil {
		t.Fatalf("读取语音文件失败: %v", err)
	}

	// 测试：只设置 MusicPath，其他字段保持零值
	// 这模拟了最常见的用法场景
	ctx := context.Background()
	opts := &tts.BackgroundMusicOptions{
		MusicPath: musicFile,
		// Volume: 0 (零值，应该默认为 0.3)
		// MainAudioVolume: 0 (零值，应该默认为 1.0)
		// Loop: nil (应该默认为 true)
	}

	mixedAudio, err := MixWithBackgroundMusic(ctx, voiceData, testProviderEdge, "", opts)
	if err != nil {
		t.Fatalf("混音失败: %v", err)
	}

	// 验证输出不为空
	if len(mixedAudio) == 0 {
		t.Error("混音后的音频数据为空")
	}

	// 验证输出大小合理
	if len(mixedAudio) < 1000 {
		t.Error("混音后的音频数据异常小，可能静音了")
	}

	// 保存混音后的文件以便手动验证（可选）
	outputFile := filepath.Join(tempDir, "mixed_with_defaults.mp3")
	if err := os.WriteFile(outputFile, mixedAudio, 0644); err != nil {
		t.Logf("警告：无法保存混音文件用于检查: %v", err)
	} else {
		t.Logf("混音文件已保存到: %s", outputFile)
	}
}

// TestCleanupOldFiles 测试文件清理功能
func TestCleanupOldFiles(t *testing.T) {
	tempDir := t.TempDir()

	// 创建一些测试文件
	oldFile := filepath.Join(tempDir, "old.txt")
	newFile := filepath.Join(tempDir, "new.txt")

	// 创建旧文件（修改时间为 25 小时前）
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("创建旧文件失败: %v", err)
	}
	oldTime := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("修改文件时间失败: %v", err)
	}

	// 创建新文件
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("创建新文件失败: %v", err)
	}

	// 执行清理（删除超过 24 小时的文件）
	if err := CleanupOldFiles(tempDir, 24*time.Hour); err != nil {
		t.Fatalf("清理失败: %v", err)
	}

	// 验证旧文件被删除
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("旧文件应该被删除")
	}

	// 验证新文件仍然存在
	if _, err := os.Stat(newFile); err != nil {
		t.Error("新文件不应该被删除")
	}
}

// TestValidateAudioFormat 测试音频格式验证
func TestValidateAudioFormat(t *testing.T) {
	if !IsFFprobeInstalled() {
		t.Skip("跳过测试：ffprobe 未安装")
	}

	// 创建临时目录
	tempDir := t.TempDir()

	// 创建一个有效的音频文件
	audioFile := filepath.Join(tempDir, "test.mp3")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "anullsrc=r=24000:cl=mono",
		"-t", "1",
		"-c:a", "libmp3lame",
		audioFile,
	)
	if err := cmd.Run(); err != nil {
		t.Skip("无法生成测试音频文件")
	}

	// 测试有效音频文件
	if err := validateAudioFormat(audioFile); err != nil {
		t.Errorf("有效音频文件验证失败: %v", err)
	}

	// 创建一个无效的文件（纯文本）
	textFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(textFile, []byte("not audio"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 测试无效文件
	if err := validateAudioFormat(textFile); err == nil {
		t.Error("无效文件应该验证失败")
	}
}
