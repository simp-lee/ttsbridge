# TTSBridge

TTSBridge 是一个通用的文字转语音(Text-to-Speech)工具,为多个 TTS 服务提供商提供统一的抽象层。

## 特性

- 🔌 **统一接口**: 对外提供统一的 API,屏蔽底层 TTS 服务商的差异
- 🌍 **多提供商支持**: 支持 Edge TTS、阿里云、腾讯云等多个 TTS 服务
- 🔄 **灵活切换**: 轻松在不同的 TTS 提供商之间切换
- ⚡ **高性能**: 支持流式输出,降低延迟
- 🛠️ **易于扩展**: 插件化架构,方便添加新的 TTS 提供商
- 🔐 **生产就绪**: Edge TTS 已优化，具有 DRM 保护、自动重试、长文本支持等生产级特性
- 🎵 **背景音乐**: 支持为语音添加背景音乐，带音量控制、淡入淡出、循环播放等功能

## 支持的 TTS 提供商

- [x] **Edge TTS** (免费) - 🎉 **已优化！** 基于 [edge-tts](https://github.com/rany2/edge-tts) (9.3k⭐) 最佳实践
  - ✅ DRM 保护和 403 自动恢复
  - ✅ 长文本自动分割（无限长度）
  - ✅ UTF-8 安全和字符清理
  - ✅ 生产级稳定性
- [ ] Azure Cognitive Services TTS
- [ ] 阿里云 TTS
- [ ] 腾讯云 TTS
- [ ] Google Cloud TTS
- [ ] AWS Polly
- [ ] 讯飞 TTS
- [ ] 百度 TTS

## 快速开始

### 安装

```bash
go get github.com/simp-lee/ttsbridge
```

### 基本使用

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/simp-lee/ttsbridge/tts"
    "github.com/simp-lee/ttsbridge/providers/edgetts"
)

func main() {
    // 创建 Edge TTS 提供商
    provider := edgetts.New()
    
    // 配置语音参数（音频格式已简化为官方默认：24kHz 48kbps MP3）
    opts := &tts.SynthesizeOptions{
        Text:     "你好,欢迎使用 TTSBridge!",
        Voice:    "zh-CN-XiaoxiaoNeural",  // 语音名称
        Rate:     1.0,                      // 语速 (0.5-2.0)
        Volume:   1.0,                      // 音量 (0.0-1.0)
        Pitch:    1.0,                      // 音调 (0.5-2.0)
    }
    
    // 合成语音
    ctx := context.Background()
    audio, err := provider.Synthesize(ctx, opts)
    if err != nil {
        log.Fatal(err)
    }
    
    // 保存到文件
    err = os.WriteFile("output.mp3", audio, 0644)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Println("语音合成完成!")
}
```

### 使用语音选择器 CLI

```bash
# 运行语音选择器工具
go run cmd/voice-selector/main.go

# 或者编译后使用
go build -o voice-selector cmd/voice-selector/main.go
./voice-selector
```

### 使用 Web UI

```bash
# 运行 Web UI
go run cmd/webui/main.go

# 浏览器访问
# http://localhost:8080
```

### 获取语音列表

```go
// 获取所有中文语音
voices, err := provider.ListVoices(ctx, "zh-CN")
if err != nil {
    log.Fatal(err)
}

for _, voice := range voices {
    fmt.Printf("%s (%s): %s\n", voice.DisplayName, voice.Gender, voice.ShortName)
    if len(voice.Styles) > 0 {
        fmt.Printf("  风格: %v\n", voice.Styles)
    }
}
```

### 高级配置

```go
// 使用代理和自定义超时
provider := edgetts.NewWithOptions(&edgetts.ProviderOptions{
    ClientToken:      "custom-token",
    ProxyURL:         "http://proxy.example.com:8080",
    HTTPTimeout:      60 * time.Second,
    ConnectTimeout:   15 * time.Second,
    MaxRetryAttempts: 5,
    ReceiveTimeout:   90 * time.Second,
})

// 启用元数据获取字幕
opts := &tts.SynthesizeOptions{
    Text:                    "这是一段测试文本。",
    Voice:                   "zh-CN-XiaoxiaoNeural",
    WordBoundaryEnabled:     true,
    SentenceBoundaryEnabled: false,
    MetadataCallback: func(metadataType string, offset int64, duration int64, text string) {
        // 实时接收词/句边界信息
        fmt.Printf("[%s] %d-%d: %s\n", metadataType, offset, offset+duration, text)
    },
}

audio, err := provider.Synthesize(ctx, opts)
```

### 流式输出

```go
stream, err := provider.SynthesizeStream(ctx, opts)
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

// 逐块读取音频数据
for {
    chunk, err := stream.Read()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    // 处理音频块
    processAudioChunk(chunk)
}
```

### 错误处理

```go
audio, err := provider.Synthesize(ctx, opts)
if err != nil {
    if ttsErr, ok := err.(*tts.Error); ok {
        log.Printf("TTS Error [%s]: %s", ttsErr.Code, ttsErr.Message)
        
        switch ttsErr.Code {
        case tts.ErrCodeClockSkew:
            // 时钟不同步，已自动重试
        case tts.ErrCodeTimeout:
            // 超时，可能需要调整超时配置
        case tts.ErrCodeNoAudioReceived:
            // 未收到音频，检查参数
        }
    }
}
```

### 音频格式

音频格式已简化为官方默认：**audio-24khz-48kbitrate-mono-mp3**

这是 [edge-tts](https://github.com/rany2/edge-tts) 项目采用的标准格式，具有：
- ✅ 最佳兼容性和稳定性
- ✅ 平衡的质量和文件大小
- ✅ 广泛验证的生产环境使用
- ✅ 适合大多数应用场景

### 背景音乐混音 🎵

为语音添加背景音乐，打造专业音频效果：

```go
opts := &tts.SynthesizeOptions{
    Text:   "你好，欢迎收听我们的播客！",
    Voice:  "zh-CN-XiaoxiaoNeural",
    Rate:   1.0,
    Volume: 1.0,
    Pitch:  1.0,
    // 添加背景音乐配置
    BackgroundMusic: &tts.BackgroundMusicOptions{
        MusicPath:       "background.mp3", // 背景音乐文件路径
        Volume:          0.3,               // 背景音乐音量 30%
        FadeIn:          2.0,               // 淡入 2 秒
        FadeOut:         3.0,               // 淡出 3 秒
        StartTime:       0.0,               // 从头开始
        Loop:            true,              // 循环播放
        MainAudioVolume: 1.0,               // 主音频音量 100%
    },
}

audio, err := provider.Synthesize(ctx, opts)
```

**支持的音频格式**: MP3, WAV, OGG, FLAC, M4A, AAC, WMA

**前置要求**: 需要安装 [ffmpeg](https://ffmpeg.org/)

**注意**: 背景音乐混音功能仅支持 `Synthesize()` 方法，不支持 `SynthesizeStream()` 流式输出

详细文档请参考：[背景音乐混音功能](docs/背景音乐混音功能.md)

**示例程序**:
```bash
# 运行背景音乐混音示例
cd examples/background_music
go run main.go your_music.mp3
```

## 架构设计

```
┌─────────────────────────────────────┐
│         应用层 (Application)         │
│    使用统一的 TTS API 进行调用       │
└─────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────┐
│        抽象层 (Abstraction)          │
│   统一的 Provider 接口和数据结构     │
└─────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────┐
│      Provider 实现层 (Providers)     │
│  ┌─────────┐ ┌─────────┐ ┌────────┐ │
│  │Edge TTS │ │ 阿里云  │ │腾讯云  │ │
│  └─────────┘ └─────────┘ └────────┘ │
└─────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────┐
│     第三方 TTS 服务 (External APIs)  │
└─────────────────────────────────────┘
```

## 扩展新的提供商

实现 `Provider` 接口即可添加新的 TTS 提供商:

```go
type Provider interface {
    Name() string
    Synthesize(ctx context.Context, opts *SynthesizeOptions) ([]byte, error)
    SynthesizeStream(ctx context.Context, opts *SynthesizeOptions) (AudioStream, error)
    ListVoices(ctx context.Context, locale string) ([]Voice, error)
    IsAvailable(ctx context.Context) bool
}
```

## 配置文件

支持使用 YAML 或 JSON 配置文件:

```yaml
providers:
  edge:
    enabled: true
    
  azure:
    enabled: false
    api_key: "your-api-key"
    region: "eastus"
    
  aliyun:
    enabled: false
    access_key_id: "your-access-key-id"
    access_key_secret: "your-access-key-secret"

default_provider: edge

cache:
  enabled: true
  directory: "./cache"
  max_size_mb: 100
```

## License

MIT License
