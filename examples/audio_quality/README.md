# 音频质量选择示例

本示例只展示统一的输出能力 contract：

1. 读取 provider.Capabilities() 查看共享输出能力
2. 使用 tts.SynthesisRequest.OutputFormat 选择最终输出容器
3. 观察不支持格式在本地能力校验阶段的 fail-fast 行为

## 设计原则

调用方只依赖统一 capability 模型：

1. SupportedFormats 表示当前 provider 通过统一请求可成功返回的输出格式
2. PreferredAudioFormat 表示默认输出格式
3. tts.SynthesisRequest.OutputFormat 用于显式选择最终输出容器

provider-native 的格式目录、探测缓存和内部映射不属于 caller-facing contract，不应作为上层格式发现入口。

## 当前共享输出能力

### EdgeTTS

- SupportedFormats: mp3
- PreferredAudioFormat: mp3

当前 live Edge 服务只对共享层声明 mp3。请求 wav 或 pcm 会在能力校验阶段返回 UNSUPPORTED_FORMAT。

### Volcengine

- SupportedFormats: wav
- PreferredAudioFormat: wav

Volcengine 免费 API 固定返回 WAV 24kHz 16-bit 单声道无损音频，不支持切换。

## 使用示例

### 1. 查询共享输出能力

```go
edgeCaps := edgetts.New().Capabilities()
fmt.Println(edgeCaps.SupportedFormats, edgeCaps.PreferredAudioFormat)

volcCaps := volcengine.New().Capabilities()
fmt.Println(volcCaps.SupportedFormats, volcCaps.PreferredAudioFormat)
```

### 2. 使用统一请求选择输出格式

```go
result, err := provider.Synthesize(ctx, tts.SynthesisRequest{
    InputMode:    tts.InputModePlainText,
    Text:         "这是测试文本",
    VoiceID:      "zh-CN-XiaoxiaoNeural",
    OutputFormat: tts.AudioFormatMP3,
})
```

不设置 OutputFormat 时使用 provider.Capabilities().PreferredAudioFormat 对应的默认格式。

### 3. 验证不支持格式的 fail-fast

```go
_, err := provider.Synthesize(ctx, tts.SynthesisRequest{
    InputMode:    tts.InputModePlainText,
    Text:         "这段文本不会发送到服务端",
    VoiceID:      "zh-CN-XiaoxiaoNeural",
    OutputFormat: tts.AudioFormatWAV,
})
```

对当前 live Edge provider，这个请求会在本地返回 UNSUPPORTED_FORMAT，而不是等服务端拒绝后再失败。

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

### Q: EdgeTTS 为什么不能通过统一请求拿到 WAV 或 PCM？

因为当前 live Edge 服务的 caller-facing 共享能力只声明 mp3。库会在本地先做 capability 校验，避免把明知不支持的格式请求发到服务端。

### Q: Volcengine 为什么只有一个格式？

Volcengine 免费 API 始终输出固定的 WAV 24kHz 16-bit 无损音频，不支持切换。
