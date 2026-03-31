# TTSBridge

Go 语言通用文字转语音 (TTS) 库，提供可在应用内直接复用的 provider-neutral 基础设施层。

它聚焦统一请求/结果契约、语音过滤、格式与元数据、缓存、健康检查、fallback 与 CLI 接入；角色映射、文本规范化、字幕规划等产品层语义不在本库范围内。

## 特性

- 统一公共契约：`tts.Provider`、`tts.SynthesisRequest`、`tts.SynthesisResult`、`tts.ProviderCapabilities`
- 统一语音模型：`tts.Voice`、`tts.VoiceFilter`、`tts.FilterVoices`
- 统一结果元数据：音频字节、格式、采样率、时长、provider、voice ID、可选 boundary events、可选 limitations
- 共享运行时能力：`tts.VoiceCache`、`tts.ProviderHealth`、`tts.SynthesizeWithFallback`、`tts.FormatRegistry`
- 通用音频辅助：`tts.PCMToWAV`、`tts.InferDuration`
- 内置 provider：EdgeTTS 与 Volcengine
- CLI：命令行辅助入口，适合脚本和批处理；详细用法见 `cmd/ttsbridge/README.md`

## 当前内置 Provider 能力

| Provider | RawSSML | ProsodyParams | PlainTextOnly | BoundaryEvents | Streaming | SupportedFormats | PreferredAudioFormat |
|----------|---------|---------------|---------------|----------------|-----------|------------------|----------------------|
| EdgeTTS | ❌ | ✅ | ❌ | ✅ | ✅ | `mp3` | `mp3` |
| Volcengine | ❌ | ❌ | ✅ | ❌ | ❌ | `wav` 原生 | `wav` |

说明：

- 当前内置 provider 均不支持 `tts.InputModeRawSSML` 透传。
- `PlainTextOnly=true` 表示 provider 只接受纯文本输入；当前 Volcengine 属于这一类。
- `BoundaryEvents` 仅在同步 `Synthesize` 的 `result.BoundaryEvents` 中返回；`SynthesizeStream` 只返回音频块，调用前可用 `request.ValidateStreamAgainst(provider.Name(), provider.Capabilities())` 预检。
- 未显式指定 `OutputFormat` 时会回落到 `PreferredAudioFormat`；当前 EdgeTTS 默认 `mp3`，Volcengine 默认 `wav`。

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
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	provider := edgetts.New()

	result, err := provider.Synthesize(context.Background(), tts.SynthesisRequest{
		InputMode:    tts.InputModePlainTextWithProsody,
		Text:         "你好，欢迎使用 TTSBridge。",
		VoiceID:      "zh-CN-XiaoxiaoNeural",
		OutputFormat: tts.AudioFormatMP3,
		Prosody:      (tts.ProsodyParams{}).WithRate(1.05),
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile("output."+result.Format, result.Audio, 0o644); err != nil {
		log.Fatal(err)
	}
}
```

未设置的 `ProsodyParams` 字段会使用 provider 默认值；如需显式传递 `0`，例如静音，请使用 `(tts.ProsodyParams{}).WithVolume(0)`。CLI 对应 `--volume -100%`。

## 统一请求与结果

```go
request := tts.SynthesisRequest{
	InputMode:          tts.InputModePlainText,
	Text:               "Hello from TTSBridge",
	VoiceID:            "en-US-JennyNeural",
	OutputFormat:       tts.AudioFormatMP3,
	NeedBoundaryEvents: true,
}

result, err := provider.Synthesize(ctx, request)
if err != nil {
	log.Fatal(err)
}

fmt.Println(result.Provider)
fmt.Println(result.VoiceID)
fmt.Println(result.Format)
fmt.Println(result.SampleRate)
fmt.Println(result.Duration)
fmt.Println(len(result.BoundaryEvents))
```

`tts.SynthesisResult` 至少返回：

- `Audio`
- `Format`
- `SampleRate`
- `Duration`
- `Provider`
- `VoiceID`
- 可选 `BoundaryEvents`
- 可选 `Limitations`

## 语音列表与过滤

```go
voices, err := provider.ListVoices(ctx, tts.VoiceFilter{
	Language: "zh",
	Gender:   tts.GenderFemale,
})
if err != nil {
	log.Fatal(err)
}

for _, voice := range voices {
	fmt.Printf("%s\t%s\t%s\t%s\n", voice.Provider, voice.Language, voice.ID, voice.Name)
}
```

## 能力探测与 Provider 选择

```go
caps := provider.Capabilities()

if caps.ProsodyParams {
	// 可以安全设置 request.Prosody
}

if !caps.RawSSML {
	// 当前 provider 不支持 InputModeRawSSML
}

format := caps.ResolvedOutputFormat("")
fmt.Println("preferred format:", format)
```

## Volcengine 示例

```go
package main

import (
	"context"
	"log"

	"github.com/simp-lee/ttsbridge/providers/volcengine"
	"github.com/simp-lee/ttsbridge/tts"
)

func main() {
	provider := volcengine.New()

	result, err := provider.Synthesize(context.Background(), tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      "你好，这是火山引擎示例。",
		VoiceID:   "BV700_streaming",
	})
	if err != nil {
		log.Fatal(err)
	}

	_ = result
}
```

Volcengine 仅支持 `InputModePlainText`，不支持 prosody、boundary events 和流式传输。

## 共享运行时能力

### Voice Cache

```go
provider := edgetts.New().WithVoiceCache(
	tts.WithTTL(24*time.Hour),
	tts.WithBackgroundRefresh(12*time.Hour),
)
defer provider.Close()

voices, err := provider.ListVoices(ctx, tts.VoiceFilter{Language: "zh-CN"})
```

### Fallback Chain

```go
primary := edgetts.New()
backup := volcengine.New()

result, err := tts.SynthesizeWithFallback(ctx, tts.SynthesisRequest{
	InputMode: tts.InputModePlainText,
	Text:      "fallback demo",
	VoiceID:   "zh-CN-XiaoxiaoNeural",
}, primary, backup)
if err != nil {
	log.Fatal(err)
}

_ = result
```

### PCMToWAV

```go
wavBytes, err := tts.PCMToWAV(pcmBytes, 24000, 1, 16)
if err != nil {
	log.Fatal(err)
}

_ = wavBytes
```

## CLI

CLI 适合命令行和脚本自动化：

```bash
go run ./cmd/ttsbridge --help
go build -o ttsbridge ./cmd/ttsbridge
```

详细命令、退出码和脚本化约定见 `cmd/ttsbridge/README.md`。

## 架构

```text
Application / Product Layer
        |
        |  provider-neutral request/result/filter/capabilities
        v
      tts
        |
        +-- providers/edgetts
        +-- providers/volcengine
        +-- future providers
```

## 扩展新的 Provider

实现 `tts.Provider` 即可：

```go
type Provider interface {
	Name() string
	Capabilities() tts.ProviderCapabilities
	Synthesize(ctx context.Context, request tts.SynthesisRequest) (*tts.SynthesisResult, error)
	SynthesizeStream(ctx context.Context, request tts.SynthesisRequest) (tts.AudioStream, error)
	ListVoices(ctx context.Context, filter tts.VoiceFilter) ([]tts.Voice, error)
}
```

推荐实现方式：

1. 在 provider 内部声明自己的 capability。
2. 把 `tts.SynthesisRequest` 翻译为 provider-native 请求。
3. 把 provider-native 音频与元数据回填到 `tts.SynthesisResult`。
4. 用 `tts.VoiceFilter` 和 `tts.FilterVoices` 暴露统一的语音发现语义。

## 示例

| 示例 | 说明 | 路径 |
|------|------|------|
| advanced | 统一请求、语音列表、错误处理、基础集成 | `examples/advanced/` |
| stream | 流式合成读取 | `examples/stream/` |
| long_text | 长文本与边界事件结果 | `examples/long_text/` |
| audio_quality | 输出格式与元数据 | `examples/audio_quality/` |
| voice_extra | 访问 provider-specific `VoiceExtra` | `examples/voice_extra/` |
| voices | 语音列举与过滤 | `examples/voices/` |
| volcengine | Volcengine plain-text only 用法 | `examples/volcengine/` |
| volcengine_debug | Volcengine 调试样例 | `examples/volcengine_debug/` |

## License

MIT License