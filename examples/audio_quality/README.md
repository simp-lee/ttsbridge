# 音频质量选择示例

本示例展示如何使用 TTS Bridge 的 OutputOptions API 查看 Provider 的推荐格式目录，并在需要时通过显式 probe 确认当前环境已验证可用的格式。

## 设计理念

不同 TTS Provider 支持的输出格式数量和种类各不相同，无法用统一的"质量等级"枚举覆盖。
因此本库采用 **目录 + 显式验证** 模式：

1. 每个 Provider 通过 `OutputOptions()` 方法返回稳定的推荐格式目录
2. 每个 `OutputOption` 包含 `FormatID`、可读标签、描述、音频特征（Profile）
3. 如需确认当前环境中真正已验证可用的格式，调用 `FormatRegistry().ProbeAll(ctx)` 后再查看 `SupportedFormats()`
4. 用户选择 `FormatID` 传入 `SynthesizeOptions.OutputFormat` 即可

## 各 Provider 支持的格式

### EdgeTTS

| FormatID | 标签 | 描述 |
|----------|------|------|
| `audio-24khz-48kbitrate-mono-mp3` | MP3 24kHz 48kbps | 默认格式，最小文件体积 |
| `audio-24khz-96kbitrate-mono-mp3` | MP3 24kHz 96kbps | 平衡音质与体积 |
| `audio-48khz-192kbitrate-mono-mp3` | MP3 48kHz 192kbps | 高音质，适合播客/视频配音 |
| `audio-48khz-320kbitrate-mono-mp3` | MP3 48kHz 320kbps | 最高 MP3 音质 |
| `raw-24khz-16bit-mono-pcm` | PCM 24kHz 无损 | 无损音频，适合存档/后期加工 |

> EdgeTTS 还有其他输出格式常量（如 Opus、Webm 等），也可直接传入 `OutputFormat` 使用。
> `OutputOptions()` 列出的是推荐目录项，不等于当前环境已经验证通过的格式列表。

### Volcengine

| FormatID | 标签 | 描述 |
|----------|------|------|
| `wav-24khz-16bit-mono` | WAV 24kHz 无损 | 固定输出，不可更改 |

## 使用示例

### 1. 查询推荐格式目录

```go
provider := edgetts.New()

for _, opt := range provider.OutputOptions() {
    fmt.Printf("%-45s %-25s %s\n", opt.FormatID, opt.Label, opt.Description)
}
```

### 2. 显式 probe 当前环境已验证格式

```go
registry := provider.FormatRegistry()
_, _, err := registry.ProbeAll(ctx)
if err != nil {
    log.Fatal(err)
}

for _, format := range provider.SupportedFormats() {
    fmt.Println("verified:", format.ID)
}
```

### 3. 选择输出格式

```go
opts := &edgetts.SynthesizeOptions{
    Text:         "这是测试文本",
    Voice:        "zh-CN-XiaoxiaoNeural",
    OutputFormat: edgetts.OutputFormatMP3_48khz_192k, // 从 OutputOptions() 中选择
}

audio, err := provider.Synthesize(ctx, opts)
```

不设置 `OutputFormat` 时使用 Provider 默认格式。

### 4. OutputOption 结构体字段

```go
type OutputOption struct {
    FormatID    string            // 传入 SynthesizeOptions.OutputFormat 的标识符
    Label       string            // 人类可读的短标签
    Description string            // 使用场景说明
    Profile     VoiceAudioProfile // 音频特征（编码格式、采样率、声道数、比特率等）
    IsDefault   bool              // 是否为默认格式
}
```

## 运行示例

```bash
cd examples/audio_quality
go run main.go
```

## 常见问题

### Q: 如何手工验证输出音质？

以下命令仅用于本地手工检查输出文件信息，不是 TTS Bridge 库或 CLI 的运行时依赖。

```bash
ffprobe -v error -show_entries stream=codec_name,bit_rate,sample_rate output.mp3
```

### Q: Volcengine 为什么只有一个格式？

Volcengine 免费 API 始终输出固定的 WAV 24kHz 16-bit 无损音频，不支持切换。

### Q: `OutputOptions()` 返回的格式是不是都已经验证可用？

不是。`OutputOptions()` 是推荐目录面，便于展示和选择；当前环境里的真实可用性需要通过 `FormatRegistry().ProbeAll(ctx)` 后再看 `SupportedFormats()`。

### Q: Provider 还有 `OutputOptions()` 之外的格式可用吗？

EdgeTTS 有更多格式常量（如 `OutputFormatOpus_24khz`、`OutputFormatWebm_24khz` 等），
可直接传入 `OutputFormat` 使用。`OutputOptions()` 列出的是推荐目录子集，不是实时验证结果。
