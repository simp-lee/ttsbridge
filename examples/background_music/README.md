# 背景音乐示例使用说明

本示例展示如何使用 TTSBridge 的背景音乐混音功能。

## 快速开始

### 1. 准备背景音乐文件

您需要准备一个音频文件作为背景音乐。支持的格式包括：
- MP3
- WAV
- OGG
- FLAC
- M4A
- AAC
- WMA

### 2. 运行示例

```bash
# 在项目根目录下
cd examples/background_music

# 运行示例（替换为您的音乐文件路径）
go run main.go /path/to/your/music.mp3
```

### 3. 查看结果

示例程序会在当前目录生成 `output_with_music.mp3`，这是混音后的最终音频文件。

## 自定义配置

编辑 `main.go` 文件，修改 `BackgroundMusic` 配置：

```go
loop := true // nil=默认 true；显式 false 需传指针
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:       musicPath,    // 背景音乐文件路径
    Volume:          0.3,           // 背景音乐音量（0.0-1.0）
    FadeIn:          2.0,           // 淡入时长（秒）
    FadeOut:         3.0,           // 淡出时长（秒）
    StartTime:       0.0,           // 起始时间（秒）
    Loop:            &loop,         // 是否循环播放
    MainAudioVolume: 1.0,           // 主音频音量（0.0-1.0）
},
```

## 示例场景

### 场景 1：播客开场白

```go
loop := true
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:       "intro_music.mp3",
    Volume:          0.4,    // 较高音量
    FadeIn:          1.0,    // 快速淡入
    FadeOut:         2.0,    // 平滑淡出
    StartTime:       0.0,
    Loop:            &loop,
    MainAudioVolume: 1.0,
},
```

### 场景 2：轻柔背景音

```go
loop := true
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:       "ambient.mp3",
    Volume:          0.2,    // 很低音量
    FadeIn:          3.0,    // 缓慢淡入
    FadeOut:         4.0,    // 缓慢淡出
    StartTime:       5.0,    // 从第5秒开始
    Loop:            &loop,
    MainAudioVolume: 1.0,
},
```

### 场景 3：音乐片段（不循环）

```go
loop := false
BackgroundMusic: &tts.BackgroundMusicOptions{
    MusicPath:       "short_clip.mp3",
    Volume:          0.3,
    FadeIn:          0.0,    // 不淡入
    FadeOut:         0.0,    // 不淡出
    StartTime:       0.0,
    Loop:            &loop,  // 不循环
    MainAudioVolume: 1.0,
},
```

## 注意事项

1. **确保 ffmpeg 已安装**：运行 `ffmpeg -version` 验证
2. **文件路径**：使用绝对路径或相对于程序运行目录的路径
3. **音量平衡**：建议背景音乐音量设置在 0.2-0.4 之间
4. **文件大小**：避免使用过大的背景音乐文件（建议 < 50MB）

## 获取免费音乐

以下是一些获取免版权背景音乐的网站：

- [YouTube Audio Library](https://www.youtube.com/audiolibrary)
- [Free Music Archive](https://freemusicarchive.org/)
- [Incompetech](https://incompetech.com/music/)
- [Bensound](https://www.bensound.com/)

## 故障排除

### 问题：找不到背景音乐文件

确保文件路径正确，使用绝对路径：

```bash
# Windows
go run main.go C:\Users\YourName\Music\background.mp3

# macOS/Linux
go run main.go /Users/YourName/Music/background.mp3
```

### 问题：ffmpeg 错误

确保 ffmpeg 已正确安装并在 PATH 中：

```bash
# 验证安装
ffmpeg -version

# 如果未安装，请参考主文档的安装说明
```

### 问题：音量太大或太小

调整 `Volume` 参数：
- 音乐太大声：降低到 0.1-0.2
- 音乐太小声：提高到 0.4-0.5
- 语音太小声：降低 `Volume`，保持 `MainAudioVolume` 为 1.0

## 更多信息

详细文档请参考：[混音音质最佳实践](../../docs/混音音质最佳实践.md)
