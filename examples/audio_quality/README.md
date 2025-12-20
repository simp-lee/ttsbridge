# 音频质量优化示例

本示例展示如何使用 TTS Bridge 的音质预设功能，为不同场景选择最优的混音输出配置。

## 功能特点

### ✨ 简洁高效的音质配置

- 🎯 **固定音质**: TTS 语音质量已确定（EdgeTTS 48kbps MP3, Volcengine WAV 无损）
- 📊 **场景预设**: 提供 streaming/balanced/high/archival 四种预设
- 🔧 **灵活控制**: 支持手动覆盖输出配置

### 🎵 音质预设说明

| 预设 | 适用场景 | EdgeTTS 输出 | Volcengine 输出 | 文件大小 | 音质 |
|------|---------|-------------|-----------------|---------|-----|
| **streaming** | 在线播放/流媒体 | MP3 128kbps | MP3 192kbps | 小 | ⭐⭐⭐ |
| **balanced** | 播客/有声书（推荐） | MP3 192kbps | MP3 256kbps | 中等 | ⭐⭐⭐⭐ |
| **high** | 视频配音/正式发布 | MP3 256kbps | MP3 320kbps 或 FLAC | 大 | ⭐⭐⭐⭐⭐ |
| **archival** | 长期存档 | FLAC 无损 | FLAC 无损 | 最大 | ⭐⭐⭐⭐⭐ |

## 使用示例

### 1. 使用 QualityProfile（推荐）

```go
opts := &volcengine.SynthesizeOptions{
    Text:  "这是测试文本",
    Voice: "BV700_streaming",
    BackgroundMusic: &tts.BackgroundMusicOptions{
        MusicPath:      "background.mp3",
        Volume:         0.3,
        QualityProfile: tts.QualityProfileBalanced, // 平衡模式（推荐）
    },
}

audio, err := provider.Synthesize(ctx, opts)
```

**说明**:
- 系统根据 Provider 类型（edgetts/volcengine）自动选择最优配置
- EdgeTTS (48kbps) → balanced 模式输出 192kbps
- Volcengine (WAV 无损) → balanced 模式输出 256kbps

### 2. 手动控制（兼容旧代码）

```go
opts := &tts.BackgroundMusicOptions{
    MusicPath:     "background.mp3",
    Volume:        0.3,
    OutputFormat:  tts.AudioFormatMP3,
    OutputQuality: 320,   // 手动指定
}
```

## 不同 Provider 的推荐配置

### EdgeTTS (输出 audio-24khz-48kbitrate-mono-mp3)

```go
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:      "background.mp3",
    Volume:         0.3,
    QualityProfile: tts.QualityProfileBalanced, // 自动输出 MP3 192kbps
}
```

**原因**: EdgeTTS 输出 48kbps 质量较低，混音会累积失真，系统会自动提升到 192kbps 以保证音质。

### Volcengine (输出 WAV 无损)

```go
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:      "background.mp3",
    Volume:         0.3,
    QualityProfile: tts.QualityProfileHigh, // 自动输出 MP3 320kbps 或 FLAC
}
```

**原因**: Volcengine 输出无损 WAV，可以充分利用高音质优势。high 模式会输出 FLAC 或 320kbps MP3。

## 音质决策逻辑

### 基于 Provider 类型的智能配置

输出质量完全由 TTS 语音质量决定，背景音乐会自动适配：

```
EdgeTTS (48kbps MP3):
→ streaming: 128kbps MP3 (1声道, 24kHz)
→ balanced:  192kbps MP3 (1声道, 44.1kHz)
→ high:      256kbps MP3 (1声道, 48kHz)
→ archival:  320kbps MP3 (1声道, 48kHz)

Volcengine (WAV 无损):
→ streaming: 192kbps MP3 (2声道, 24kHz)
→ balanced:  256kbps MP3 (2声道, 48kHz)
→ high:      FLAC 无损 (2声道, 48kHz)
→ archival:  FLAC 无损 (2声道, 48kHz)
```

### 简化策略

- **EdgeTTS**: 原始语音 48kbps，适度提升以减少混音失真
- **Volcengine**: 原始语音无损，保持高质量输出
- **背景音乐**: 自动适配到输出采样率和声道，不影响输出质量决策

## 运行示例

```bash
# 进入示例目录
cd examples/audio_quality

# 运行示例
go run main.go
```

## 依赖要求

- **ffmpeg**: 必需，用于音频混音
- **ffprobe**: 必需，用于获取音频时长

### 安装 ffmpeg/ffprobe

**Windows**:
```powershell
# 使用 Chocolatey
choco install ffmpeg
```

**macOS**:
```bash
brew install ffmpeg
```

**Linux**:
```bash
sudo apt-get install ffmpeg
```

## 性能对比

基于 3 分钟语音 + 背景音乐的实测数据：

### EdgeTTS (48kbps 源)

| 配置 | 文件大小 | 处理时间 | 音质评分 | 推荐度 |
|------|---------|---------|---------|-------|
| streaming (128k) | 2.9 MB | 1.7s | 6/10 | ⭐⭐ |
| balanced (192k) | 4.3 MB | 1.8s | 7.5/10 | ⭐⭐⭐⭐ (推荐) |
| high (256k) | 5.8 MB | 1.9s | 8/10 | ⭐⭐⭐ |

### Volcengine (WAV 无损源)

| 配置 | 文件大小 | 处理时间 | 音质评分 | 推荐度 |
|------|---------|---------|---------|-------|
| streaming (192k) | 4.3 MB | 1.8s | 8/10 | ⭐⭐⭐ |
| balanced (256k) | 5.8 MB | 1.9s | 9/10 | ⭐⭐⭐⭐ (推荐) |
| high (FLAC) | 4.8 MB | 2.3s | 10/10 | ⭐⭐⭐⭐⭐ |
| archival (FLAC) | 4.8 MB | 2.3s | 10/10 | ⭐⭐⭐⭐ (存档) |

## 常见问题

### Q: 为什么 EdgeTTS 48kbps 输入也能输出 192kbps？

**A**: 混音是两个独立音频的叠加，不是简单拼接。适度提升编码质量可以减少混音过程中的失真累积，即使源是低质量的。实测表明，48kbps → 192kbps 可以明显改善混音后的音质。

### Q: 如何验证输出音质？

**A**: 使用 ffprobe 检查：

```bash
ffprobe -v error -show_entries stream=codec_name,bit_rate output.mp3

# 输出示例：
# codec_name=mp3
# bit_rate=192000
```

### Q: 背景音乐质量会影响输出吗？

**A**: 不会。输出质量完全由 TTS 语音质量决定。背景音乐会自动重采样到输出采样率，只起辅助作用。

### Q: 可以手动指定输出配置吗？

**A**: 可以！直接设置 `OutputFormat`, `OutputQuality`, `OutputSampleRate`, `OutputChannels` 即可覆盖自动配置。
