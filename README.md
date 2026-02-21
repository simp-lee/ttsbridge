# TTSBridge

Go 语言通用文字转语音 (TTS) 库，为多个 TTS 服务提供统一的泛型接口。

## 特性

- **统一泛型接口** — `Provider[T]` 泛型接口屏蔽底层差异，每个提供商拥有类型安全的合成选项
- **多提供商** — 内置 Edge TTS（免费，400+ 语音）和火山引擎（免费，21 语音）
- **流式合成** — `SynthesizeStream` 逐块返回音频数据，降低首字节延迟
- **长文本** — 自动分块合成，无长度上限；支持分块进度回调
- **背景音乐混音** — 通过 FFmpeg 将语音与背景音乐混合，支持淡入淡出、循环播放、音量控制
- **多格式输出** — Edge TTS 支持 9 种已验证的音频格式（MP3 / Opus / PCM / WebM / Ogg），可通过 `OutputOptions()` 查询
- **语音缓存** — `VoiceCache` 支持 TTL 过期和后台自动刷新，首次请求后读缓存
- **健康检查** — `ProviderHealth` 周期性监测可用性，连续失败计数与冷却恢复
- **故障转移** — `SynthesizeWithFallback` 按顺序尝试多个提供商，首个成功即返回
- **语音筛选** — `FilterVoices` 按语言、性别、提供商及自定义函数筛选
- **格式探测** — `FormatRegistry` 管理格式声明，支持运行时自动探测与缓存
- **生产级稳定** — DRM 保护、403 自动恢复、时钟偏移检测、指数退避重试
- **CLI 与 Web UI** — 命令行工具和内置 Web 界面，开箱即用

## 提供商

| 提供商 | 费用 | 语音数 | 输出格式 | 流式 | 语速/音量/音调 |
|--------|------|--------|----------|------|----------------|
| **Edge TTS** | 免费 | 400+ | 9 种（MP3/Opus/PCM/WebM/Ogg） | ✅ | ✅ |
| **火山引擎** | 免费 | 21 | WAV 24kHz | ✅* | — |

> \*火山引擎的流式输出为内部缓冲模拟，非真正的分块传输。

**Edge TTS** 基于 [edge-tts](https://github.com/rany2/edge-tts) 最佳实践，具备 DRM 保护、长文本自动分割、UTF-8 安全清理等生产级特性。

**火山引擎** 基于火山翻译 API，完全免费，无需 APP_ID 或 ACCESS_TOKEN。支持中文、英文、日文等多语言，方言音色包括东北话、四川话、广西话等。

## 安装

```bash
go get github.com/simp-lee/ttsbridge
```

需要 Go 1.24+。

## 快速开始

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/simp-lee/ttsbridge/providers/edgetts"
)

func main() {
    provider := edgetts.New()

    opts := &edgetts.SynthesizeOptions{
        Text:   "你好，欢迎使用 TTSBridge！",
        Voice:  "zh-CN-XiaoxiaoNeural",
        Rate:   1.0,   // 语速 0.5-2.0
        Volume: 1.0,   // 音量 0.0-1.0
        Pitch:  1.0,   // 音调 0.5-2.0
    }

    audio, err := provider.Synthesize(context.Background(), opts)
    if err != nil {
        log.Fatal(err)
    }

    os.WriteFile("output.mp3", audio, 0644)
}
```

## 使用指南

### 获取语音列表

```go
voices, err := provider.ListVoices(ctx, "zh-CN")
if err != nil {
    log.Fatal(err)
}

for _, v := range voices {
    fmt.Printf("%s (%s) — %s [%s]\n", v.Name, v.Gender, v.ID, v.Provider)
}
```

### 访问语音扩展信息

每个提供商的语音携带特有的 `Extra` 字段，通过泛型函数 `tts.GetExtra[T]()` 类型安全地访问：

```go
// EdgeTTS: 风格、角色、分类等
if extra, ok := tts.GetExtra[*edgetts.VoiceExtra](&voice); ok {
    fmt.Println("分类:", extra.Categories)
    fmt.Println("个性:", extra.Personalities)
    fmt.Println("状态:", extra.Status)        // "GA", "Preview", "Deprecated"
}

// Volcengine: 场景标签、情感标签等
if extra, ok := tts.GetExtra[*volcengine.VoiceExtra](&voice); ok {
    fmt.Println("分类:", extra.Category)
    fmt.Println("场景:", extra.SceneTags)
    fmt.Println("情感:", extra.EmotionTags)
}
```

### 选择输出格式

通过 `OutputOptions()` 查询提供商支持的输出格式（所有格式均经过实际探测验证）：

```go
// 查看所有支持的输出格式
for _, opt := range provider.OutputOptions() {
    mark := ""
    if opt.IsDefault {
        mark = " (默认)"
    }
    fmt.Printf("%-45s %s%s\n", opt.FormatID, opt.Label, mark)
}

// 使用高音质格式
opts := &edgetts.SynthesizeOptions{
    Text:         "高音质测试文本",
    Voice:        "zh-CN-XiaoxiaoNeural",
    OutputFormat: edgetts.OutputFormatMP3_48khz_192k, // MP3 48kHz 192kbps
}
```

**Edge TTS 已验证可用的输出格式：**

| 常量 | 格式 ID | 说明 |
|------|---------|------|
| `OutputFormatMP3_24khz_48k` | `audio-24khz-48kbitrate-mono-mp3` | **默认**，48kbps |
| `OutputFormatMP3_24khz_96k` | `audio-24khz-96kbitrate-mono-mp3` | 96kbps |
| `OutputFormatMP3_24khz_160k` | `audio-24khz-160kbitrate-mono-mp3` | 160kbps |
| `OutputFormatMP3_48khz_192k` | `audio-48khz-192kbitrate-mono-mp3` | 192kbps |
| `OutputFormatMP3_48khz_320k` | `audio-48khz-320kbitrate-mono-mp3` | 320kbps，最高品质 MP3 |
| `OutputFormatOpus_24khz` | `audio-24khz-16bit-mono-opus` | Opus |
| `OutputFormatPCM_24khz` | `raw-24khz-16bit-mono-pcm` | 无损 PCM |
| `OutputFormatWebM_24khz` | `webm-24khz-16bit-mono-opus` | WebM 容器 |
| `OutputFormatOgg_24khz` | `ogg-24khz-16bit-mono-opus` | Ogg 容器 |

> 以上 9 种格式经实际探测验证（2026-02-21）。Azure 文档中的其余 28 种格式均已确认不可用。

**Volcengine** 固定输出 WAV 24kHz 16-bit mono，API 无格式选择参数。

### 流式合成

```go
stream, err := provider.SynthesizeStream(ctx, opts)
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for {
    chunk, err := stream.Read()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    // 处理音频块...
}
```

### 边界事件回调（字幕）

```go
opts := &edgetts.SynthesizeOptions{
    Text:                "这是一段测试文本。",
    Voice:               "zh-CN-XiaoxiaoNeural",
    WordBoundaryEnabled: true,
    BoundaryCallback: func(event tts.BoundaryEvent) {
        fmt.Printf("[%s] %dms-%dms: %s\n",
            event.Type, event.OffsetMs, event.OffsetMs+event.DurationMs, event.Text)
    },
}

audio, err := provider.Synthesize(ctx, opts)
```

### 高级配置

```go
provider := edgetts.New().
    WithProxy("http://proxy.example.com:8080").
    WithHTTPTimeout(60 * time.Second).
    WithConnectTimeout(15 * time.Second).
    WithReceiveTimeout(90 * time.Second).
    WithMaxAttempts(5).
    WithClientToken("custom-token")
```

### 火山引擎

```go
provider := volcengine.New()

voices, _ := provider.ListVoices(ctx, "zh-CN")
for _, v := range voices {
    fmt.Printf("%s (%s) — %s\n", v.Name, v.Gender, v.ID)
}

audio, err := provider.Synthesize(ctx, &volcengine.SynthesizeOptions{
    Text:  "你好，这是火山引擎语音合成测试。",
    Voice: "BV700_streaming", // 灿灿
})
```

### 背景音乐混音

为语音添加背景音乐，需要系统安装 [FFmpeg](https://ffmpeg.org/)：

```go
loop := true
opts := &edgetts.SynthesizeOptions{
    Text:  "欢迎收听我们的播客！",
    Voice: "zh-CN-XiaoxiaoNeural",
    BackgroundMusic: &tts.BackgroundMusicOptions{
        MusicPath: "background.mp3",
        Volume:    0.3,           // 背景音乐音量 30%
        FadeIn:    2.0,           // 淡入 2 秒
        FadeOut:   3.0,           // 淡出 3 秒
        Loop:      &loop,         // 循环播放
    },
}

audio, err := provider.Synthesize(ctx, opts)
```

> 混音功能仅支持 `Synthesize()`，不支持 `SynthesizeStream()`。

### 语音缓存

减少重复 API 调用，支持 TTL 过期和后台自动刷新：

```go
provider := edgetts.New().WithVoiceCache(
    tts.WithTTL(24 * time.Hour),
    tts.WithBackgroundRefresh(12 * time.Hour),
)
defer provider.Close()

voices, err := provider.ListVoices(ctx, "zh-CN") // 首次拉取，后续读缓存
```

- TTL 过期后自动重新拉取；拉取失败时返回过期数据（stale-while-revalidate）
- 后台刷新避免请求时阻塞

### 健康检查

周期性检测提供商可用性：

```go
health := tts.NewProviderHealth(
    func(ctx context.Context) bool {
        return provider.IsAvailable(ctx)
    },
    tts.WithCheckInterval(5 * time.Minute),
    tts.WithMaxFails(3),
    tts.WithCooldownTime(60 * time.Second),
)
health.Start(ctx)
defer health.Stop()

if health.IsHealthy() {
    // 可以正常调用
}
```

### 故障转移

按顺序尝试多个提供商，首个成功即返回：

```go
// 注意：SynthesizeWithFallback 要求所有提供商使用相同的选项类型
audio, err := tts.SynthesizeWithFallback(ctx, opts, primaryProvider, backupProvider)
if err != nil {
    var fallbackErr *tts.FallbackError
    if errors.As(err, &fallbackErr) {
        for _, f := range fallbackErr.Failures {
            log.Printf("%s: %v", f.Provider, f.Err)
        }
    }
}
```

### 语音筛选

所有非零条件取交集（AND 逻辑）：

```go
filtered := tts.FilterVoices(voices, tts.VoiceFilter{
    Language: "zh-CN",
    Gender:   tts.GenderFemale,
})

// 自定义筛选函数
filtered = tts.FilterVoices(voices, tts.VoiceFilter{
    Language: "zh-CN",
    FilterFunc: func(v tts.Voice) bool {
        extra, ok := tts.GetExtra[*edgetts.VoiceExtra](&v)
        return ok && slices.Contains(extra.Categories, "Novel")
    },
})
```

### 格式探测

每个 Provider 维护一个 `FormatRegistry`，管理输出格式的声明和运行时探测：

```go
// 获取已被验证可用的格式列表
formats := provider.SupportedFormats()

// 获取注册表，可对未验证格式执行实际探测
registry := provider.FormatRegistry()
available, unavailable, err := registry.ProbeAll(ctx)
```

### 错误处理

```go
audio, err := provider.Synthesize(ctx, opts)
if err != nil {
    if ttsErr, ok := err.(*tts.Error); ok {
        switch ttsErr.Code {
        case tts.ErrCodeClockSkew:
            // 时钟不同步，已自动重试
        case tts.ErrCodeTimeout:
            // 超时，调整超时配置
        case tts.ErrCodeNoAudioReceived:
            // 未收到音频，检查参数
        }
    }
}
```

## CLI

```bash
# 编译
go build -o ttsbridge ./cmd/ttsbridge

# 列出语音
ttsbridge voices --provider edgetts --locale zh-CN
ttsbridge voices --provider volcengine --format json
ttsbridge voices --provider all

# 合成语音
ttsbridge synthesize --provider edgetts --voice zh-CN-XiaoxiaoNeural --text "你好" --out output.mp3
ttsbridge synthesize --provider edgetts --voice zh-CN-XiaoxiaoNeural --file input.txt --out output.mp3
cat input.txt | ttsbridge synthesize --provider edgetts --voice zh-CN-XiaoxiaoNeural --file - --out output.mp3

# 自定义参数（rate/volume/pitch 使用百分比格式，仅 edgetts 支持）
ttsbridge synthesize --provider edgetts \
    --voice zh-CN-XiaoxiaoNeural \
    --text "你好" \
    --rate "+20%" --volume "-10%" --pitch "+0%" \
    --proxy http://proxy:8080 \
    --out output.mp3
```

## Web UI

内置 Web 界面，支持语音合成和背景音乐混音：

```bash
go run ./cmd/webui
```

浏览器访问 `http://localhost:8080`。

**环境变量配置：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `TTSBRIDGE_WEBUI_HOST` | `127.0.0.1` | 监听地址 |
| `TTSBRIDGE_WEBUI_PORT` | `8080` | 监听端口 |
| `TTSBRIDGE_WEBUI_TOKEN` | — | 远程访问鉴权 Token（本地回环无需设置） |

## FFmpeg 安装

背景音乐混音功能依赖 FFmpeg。库会自动在以下位置查找：

1. 系统 `PATH`
2. 可执行文件同级 `ffmpeg/bin/`
3. 库内嵌的 `ffmpeg/bin/` 目录

```bash
# Ubuntu / Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Windows (scoop)
scoop install ffmpeg
```

或使用项目下载脚本：

```bash
./scripts/download-ffmpeg.sh --platform linux --dry-run  # 仅检查，不下载
./scripts/download-ffmpeg.sh --platform linux             # 下载并校验 SHA-256
./scripts/download-ffmpeg.sh --all                        # Linux + Windows
```

## 架构

```
┌─────────────────────────────────────────────┐
│            应用层 (Application)               │
│  Synthesize · SynthesizeStream · ListVoices  │
└──────────────────────┬──────────────────────┘
                       │
┌──────────────────────▼──────────────────────┐
│            抽象层 (tts package)               │
│  Provider[T] · Voice · AudioStream · Error   │
│  VoiceCache · ProviderHealth · VoiceFilter   │
│  FormatRegistry · SynthesizeWithFallback     │
└──────────────────────┬──────────────────────┘
                       │
┌──────────┬───────────▼───────────┬──────────┐
│ EdgeTTS  │     Volcengine        │  Your    │
│ Provider │     Provider          │ Provider │
│ (WS)     │     (HTTP)            │  (...)   │
└──────────┴───────────────────────┴──────────┘
```

## 扩展新的提供商

实现 `Provider[T]` 泛型接口：

```go
type Provider[T any] interface {
    Name() string
    Synthesize(ctx context.Context, opts T) ([]byte, error)
    SynthesizeStream(ctx context.Context, opts T) (AudioStream, error)
    ListVoices(ctx context.Context, locale string) ([]Voice, error)
    IsAvailable(ctx context.Context) bool
}
```

`T` 是提供商专用的合成选项类型（如 `*edgetts.SynthesizeOptions`、`*volcengine.SynthesizeOptions`），每个提供商可定义自己需要的参数。

## 示例

| 示例 | 说明 | 路径 |
|------|------|------|
| 基本用法 | 合成、字幕回调、自定义配置、语音列表、错误处理 | `examples/advanced/` |
| 流式合成 | 逐块写入文件 | `examples/stream/` |
| 长文本 | 自动分块、进度回调 | `examples/long_text/` |
| 音频格式 | 查询 OutputOptions、选择输出格式 | `examples/audio_quality/` |
| 背景音乐 | 混音、淡入淡出、循环 | `examples/background_music/` |
| 语音扩展 | 访问 EdgeTTS/Volcengine 的 VoiceExtra | `examples/voice_extra/` |
| 语音列表 | 列出 EdgeTTS 语音 | `examples/voices/` |
| 火山引擎 | 测试全部 21 款免费音色 | `examples/volcengine/` |

```bash
# 运行示例
cd examples/advanced && go run main.go
```

## License

MIT License
