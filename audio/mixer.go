package audio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MixWithBackgroundMusic 将背景音乐混合到语音音频中
// voiceAudio: 语音音频数据（支持 MP3/WAV 等格式,ffmpeg 会自动识别）
// providerName: TTS 提供商名称，用于确定语音质量
// voiceID: TTS voice 标识，便于解析语音音质
// opts: 背景音乐选项
// 返回混音后的音频数据
func MixWithBackgroundMusic(ctx context.Context, voiceAudio []byte, providerName, voiceID string, opts *tts.BackgroundMusicOptions) ([]byte, error) {
	// 检查 context 是否已取消
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("操作已取消: %w", err)
	}

	if opts == nil {
		return nil, fmt.Errorf("背景音乐选项不能为空")
	}

	if opts.MusicPath == "" {
		return nil, fmt.Errorf("背景音乐文件路径不能为空")
	}

	// 检查背景音乐文件是否存在
	if _, err := os.Stat(opts.MusicPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("背景音乐文件不存在: %s", opts.MusicPath)
	}

	// 检查 ffmpeg 是否安装
	if !IsFFmpegInstalled() {
		return nil, fmt.Errorf("ffmpeg 未安装，请参考文档安装: https://ffmpeg.org/download.html")
	}

	// 检查 ffprobe 是否安装
	if !IsFFprobeInstalled() {
		return nil, fmt.Errorf("ffprobe 未安装，混音功能需要 ffprobe 来获取音频时长，请参考文档安装: https://ffmpeg.org/download.html")
	}

	// 拷贝配置以避免修改调用方的原始配置
	optsCopy := *opts
	opts = &optsCopy

	// 设置默认值
	if opts.Volume <= 0 {
		opts.Volume = tts.DefaultBackgroundMusicVolume
	}
	if opts.MainAudioVolume <= 0 {
		opts.MainAudioVolume = tts.DefaultMainAudioVolume
	}
	// Loop 默认为 true
	if opts.Loop == nil {
		defaultLoop := true
		opts.Loop = &defaultLoop
	}

	// 获取 TTS 语音的音质配置
	profile, found := tts.LookupVoiceAudioProfile(providerName, voiceID)
	if !found {
		// 如果未找到配置，使用保守的默认值
		profile = tts.VoiceAudioProfile{
			Format:     tts.AudioFormatMP3,
			SampleRate: tts.SampleRate24kHz,
			Channels:   1,
			Bitrate:    tts.MP3BitrateBalanced,
			Lossless:   false,
		}
	}

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "ttsbridge_mixer_*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 将语音音频写入临时文件
	// 使用 .dat 扩展名避免混淆，ffmpeg 会自动检测实际格式（WAV/MP3/等）
	voiceFile := filepath.Join(tempDir, "voice.dat")
	if err := os.WriteFile(voiceFile, voiceAudio, 0644); err != nil {
		return nil, fmt.Errorf("写入语音文件失败: %w", err)
	}

	// 输出文件（扩展名根据格式设置）
	outputExt := profile.Format
	if outputExt == tts.AudioFormatAAC {
		outputExt = "m4a" // AAC 通常用 m4a 容器
	}
	outputFile := filepath.Join(tempDir, "mixed."+outputExt)

	// 构建 ffmpeg 混音命令
	if err := mixAudio(voiceFile, opts.MusicPath, outputFile, opts, &profile); err != nil {
		return nil, fmt.Errorf("混音失败: %w", err)
	}

	// 读取混音后的文件
	mixedAudio, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("读取混音文件失败: %w", err)
	}

	return mixedAudio, nil
}

// mixAudio 执行音频混音
func mixAudio(voiceFile, musicFile, outputFile string, opts *tts.BackgroundMusicOptions, profile *tts.VoiceAudioProfile) error {
	// 获取语音音频时长
	voiceDuration, err := getAudioDuration(voiceFile)
	if err != nil {
		return fmt.Errorf("获取语音时长失败: %w", err)
	}

	// 构建背景音乐滤镜
	musicFilters := buildMusicFilters(opts, voiceDuration)

	// 使用 ffmpeg-go 构建混音流程
	voice := ffmpeg.Input(voiceFile)
	music := ffmpeg.Input(musicFile)

	// 应用主音频音量
	voiceStream := voice.Audio()
	if opts.MainAudioVolume != 1.0 {
		voiceStream = voiceStream.Filter("volume", ffmpeg.Args{fmt.Sprintf("%.2f", opts.MainAudioVolume)})
	}

	// 应用背景音乐滤镜
	musicStream := music.Audio()
	for _, filter := range musicFilters {
		musicStream = musicStream.Filter(filter.Name, filter.Args)
	}

	// 混合两个音频流
	// 使用 amix 滤镜混音
	merged := ffmpeg.Filter(
		[]*ffmpeg.Stream{voiceStream, musicStream},
		"amix",
		ffmpeg.Args{
			"inputs=2",
			"duration=longest",     // 使用最长的输入作为输出长度
			"dropout_transition=2", // 平滑的音频过渡
			"normalize=0",          // 禁用自动归一化，使用手动音量控制以保持更好的动态范围
		},
	)

	// 根据格式配置输出参数
	outputKwArgs := buildOutputConfig(profile)

	// 输出音频
	var errBuf bytes.Buffer
	err = merged.
		Output(outputFile, outputKwArgs).
		OverWriteOutput().
		WithErrorOutput(&errBuf).
		Run()

	if err != nil {
		// 将 ffmpeg 的 stderr 输出附加到错误信息中
		if errBuf.Len() > 0 {
			return fmt.Errorf("ffmpeg 执行失败: %w\nffmpeg 输出:\n%s", err, errBuf.String())
		}
		return fmt.Errorf("ffmpeg 执行失败: %w", err)
	}

	return nil
}

// buildOutputConfig 根据音质选项构建 ffmpeg 输出配置
func buildOutputConfig(profile *tts.VoiceAudioProfile) ffmpeg.KwArgs {
	// 确定采样率
	sampleRate := profile.SampleRate
	if sampleRate <= 0 {
		sampleRate = tts.SampleRate44kHz // 默认 44.1kHz
	}
	// 限制最低采样率（语音场景通常不低于 16kHz）
	if sampleRate < 16000 {
		sampleRate = 16000
	}

	// 确定声道数
	channels := profile.Channels
	if channels <= 0 {
		channels = 1
	}

	// 验证和限制声道数
	if channels < 1 {
		channels = 1
	}
	// 大多数有损格式不支持超过 2 声道
	if channels > 2 && profile.Format != tts.AudioFormatWAV && profile.Format != tts.AudioFormatFLAC {
		channels = 2
	}

	config := ffmpeg.KwArgs{
		"ar": fmt.Sprintf("%d", sampleRate),
		"ac": fmt.Sprintf("%d", channels),
	}

	switch profile.Format {
	case tts.AudioFormatWAV:
		// WAV 格式 - 无损，最高音质
		config["codec:a"] = "pcm_s16le" // 16位 PCM
		config["f"] = "wav"

	case tts.AudioFormatFLAC:
		// FLAC 格式 - 无损压缩，高音质，文件较小
		config["codec:a"] = "flac"
		config["compression_level"] = "8" // 0-12，8是推荐的平衡值
		config["f"] = "flac"

	case tts.AudioFormatM4A, tts.AudioFormatAAC:
		// AAC/M4A 格式 - 有损压缩，中高音质，文件小
		config["codec:a"] = "aac"
		config["f"] = "mp4" // M4A 使用 MP4 容器
		// VBR 质量模式: 0-9，0最高质量
		if profile.Bitrate > 0 && profile.Bitrate <= 9 {
			config["q:a"] = fmt.Sprintf("%d", profile.Bitrate)
		} else {
			config["q:a"] = "1" // 默认高质量
		}

	case tts.AudioFormatMP3:
		fallthrough
	default:
		// MP3 格式 - 有损压缩，通用性好
		config["codec:a"] = "libmp3lame"
		config["f"] = "mp3"

		// 设置比特率（优先保持与 TTS 输出一致）
		bitrate := profile.Bitrate
		if bitrate > 0 {
			config["b:a"] = fmt.Sprintf("%dk", bitrate)
		} else {
			config["q:a"] = "0" // 无明确比特率时退回最高质量
		}
	}

	return config
}

// filterConfig 滤镜配置
type filterConfig struct {
	Name string
	Args ffmpeg.Args
}

// buildMusicFilters 构建背景音乐滤镜链
func buildMusicFilters(opts *tts.BackgroundMusicOptions, voiceDuration float64) []filterConfig {
	filters := make([]filterConfig, 0)

	// 如果需要循环，使用 aloop 滤镜
	if opts.Loop != nil && *opts.Loop {
		// 计算需要循环的次数（向上取整）
		// aloop 的 loop 参数：-1 表示无限循环，但我们需要设置确切的次数
		// 先使用一个足够大的循环次数，然后用 atrim 裁剪到正确的长度
		filters = append(filters, filterConfig{
			Name: "aloop",
			Args: ffmpeg.Args{
				"loop=-1", // 无限循环
				"size=2e+09",
			},
		})
	}

	// 裁剪到语音长度（如果需要延迟，需要调整裁剪长度）
	musicDuration := voiceDuration
	if opts.StartTime > 0 {
		musicDuration = voiceDuration - opts.StartTime
		if musicDuration <= 0 {
			// 如果延迟时间大于等于语音长度，背景音乐不会播放
			musicDuration = 0.1 // 最小0.1秒
		}
	}
	filters = append(filters, filterConfig{
		Name: "atrim",
		Args: ffmpeg.Args{fmt.Sprintf("duration=%.2f", musicDuration)},
	})

	// 淡入效果
	if opts.FadeIn > 0 {
		filters = append(filters, filterConfig{
			Name: "afade",
			Args: ffmpeg.Args{
				"t=in",
				"st=0",
				fmt.Sprintf("d=%.2f", opts.FadeIn),
			},
		})
	}

	// 淡出效果
	if opts.FadeOut > 0 {
		fadeOutStart := musicDuration - opts.FadeOut
		if fadeOutStart < 0 {
			fadeOutStart = 0
		}
		filters = append(filters, filterConfig{
			Name: "afade",
			Args: ffmpeg.Args{
				"t=out",
				fmt.Sprintf("st=%.2f", fadeOutStart),
				fmt.Sprintf("d=%.2f", opts.FadeOut),
			},
		})
	}

	// 音量调节
	filters = append(filters, filterConfig{
		Name: "volume",
		Args: ffmpeg.Args{fmt.Sprintf("%.2f", opts.Volume)},
	})

	// 如果指定了起始时间，添加延迟效果（在混音前）
	if opts.StartTime > 0 {
		filters = append(filters, filterConfig{
			Name: "adelay",
			Args: ffmpeg.Args{fmt.Sprintf("delays=%.0f:all=1", opts.StartTime*1000)}, // 转换为毫秒
		})
	}

	return filters
}

// getAudioDuration 获取音频文件时长（秒）
func getAudioDuration(filePath string) (float64, error) {
	// 使用 ffprobe 获取音频时长
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("执行 ffprobe 失败: %w", err)
	}

	var duration float64
	_, err = fmt.Sscanf(string(output), "%f", &duration)
	if err != nil {
		return 0, fmt.Errorf("解析时长失败: %w", err)
	}

	return duration, nil
}

// IsSupportedAudioExtension 检查文件扩展名是否为支持的音频格式
func IsSupportedAudioExtension(ext string) bool {
	supportedExts := map[string]bool{
		".mp3":  true,
		".wav":  true,
		".ogg":  true,
		".flac": true,
		".m4a":  true,
		".aac":  true,
		".wma":  true,
	}
	// 转换为小写以忽略大小写
	return supportedExts[strings.ToLower(ext)]
}

// GetSupportedAudioExtensions 返回支持的音频格式列表（用于显示）
func GetSupportedAudioExtensions() string {
	return "mp3, wav, ogg, flac, m4a, aac, wma"
}

// ValidateBackgroundMusicFile 验证背景音乐文件格式
func ValidateBackgroundMusicFile(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("文件路径不能为空")
	}

	// 检查文件是否存在
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}
	if err != nil {
		return fmt.Errorf("访问文件失败: %w", err)
	}

	// 检查是否是普通文件
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不是有效的文件")
	}

	// 检查文件大小（限制为 50MB）
	const maxSize = 50 * 1024 * 1024
	if info.Size() > maxSize {
		return fmt.Errorf("文件过大，最大支持 50MB")
	}

	// 检查扩展名
	ext := filepath.Ext(filePath)
	if !IsSupportedAudioExtension(ext) {
		return fmt.Errorf("不支持的音频格式: %s（支持的格式: %s）", ext, GetSupportedAudioExtensions())
	}

	// 使用 ffprobe 验证实际的音频格式（如果可用）
	if IsFFprobeInstalled() {
		if err := validateAudioFormat(filePath); err != nil {
			return fmt.Errorf("音频文件格式验证失败: %w", err)
		}
	}

	return nil
}

// IsFFprobeInstalled 检查 ffprobe 是否已安装
func IsFFprobeInstalled() bool {
	cmd := exec.Command("ffprobe", "-version")
	return cmd.Run() == nil
}

// validateAudioFormat 使用 ffprobe 验证音频文件格式
func validateAudioFormat(filePath string) error {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("无法解析音频文件，请确保文件格式正确")
	}

	// 检查是否包含音频流
	if len(output) == 0 || string(bytes.TrimSpace(output)) != "audio" {
		return fmt.Errorf("文件不包含有效的音频流")
	}

	return nil
}

// SaveUploadedFile 保存上传的文件
// 如果 filename 是完整路径，直接使用；否则保存到临时目录
func SaveUploadedFile(data []byte, filename string) (string, error) {
	var filePath string

	// 如果是绝对路径，直接使用
	if filepath.IsAbs(filename) {
		filePath = filename
		// 确保父目录存在
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %w", err)
		}
	} else {
		// 否则保存到临时目录
		tempDir := filepath.Join(os.TempDir(), "ttsbridge_music")
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %w", err)
		}
		filePath = filepath.Join(tempDir, filepath.Base(filename))
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %w", err)
	}

	return filePath, nil
}

// CleanupOldFiles 清理指定目录中超过指定时间的文件
// dir: 要清理的目录
// maxAge: 文件最大保留时间
func CleanupOldFiles(dir string, maxAge time.Duration) error {
	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // 目录不存在，无需清理
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取目录失败: %w", err)
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue // 跳过无法获取信息的文件
		}

		// 如果文件超过指定时间，删除它
		if now.Sub(info.ModTime()) > maxAge {
			os.Remove(filePath)
		}
	}

	return nil
}
