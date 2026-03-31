# TTSBridge CLI（ttsbridge）

本目录包含 `ttsbridge` 命令行工具入口（见 `cmd/ttsbridge/main.go`）。

当前实现只有 `voices` 和 `synthesize` 两个子命令。stdout 只输出结果，stderr 用于帮助、警告和错误，便于脚本处理。

## 安装与运行

在仓库根目录：

```bash
# 直接运行
go run ./cmd/ttsbridge --help

# 或编译（Windows 会生成 ttsbridge.exe）
go build -o ttsbridge ./cmd/ttsbridge
```

## 命令概览

- `ttsbridge --help` / `-h`：全局帮助
- `ttsbridge --version` / `-V`：版本信息
- `ttsbridge voices`：列出可用音色
- `ttsbridge synthesize`：合成音频

## 退出码（Exit Code）

- `0`：成功
- `1`：运行失败（网络 / Provider / IO 等运行时错误）
- `2`：用法错误（参数缺失、冲突、非法值等）

说明：使用 `go run` 时，Go 工具链可能会把子进程退出码包装成 `exit status N`，但编译后的二进制会按上述退出码退出。

## voices

列出可用音色，支持文本或 JSON 输出。

```bash
ttsbridge voices [flags]
```

Flags：

- `--provider`：`edgetts|volcengine|all`（默认 `all`）
- `--locale`：按语言过滤，例如 `zh-CN`
- `--format`：`text|json`（默认 `text`）
- `--proxy`：代理 URL（可选）
- `--http-timeout`：HTTP 超时，Go duration 格式，默认 `30s`
- `--max-attempts`：最大尝试次数（含首次），默认 `3`

说明：

- `--format` 只控制 `voices` 命令自身输出为文本或 JSON，不是合成音频格式。
- `--provider all` 时，如果部分 provider 失败，会在 stderr 打印 warning，并继续输出成功 provider 的结果；只有全部失败才返回退出码 `1`。

### 文本输出（--format text）

每行一个 voice，tab 分隔：

```
<provider>\t<language>\t<gender>\t<id>\t<name>
```

示例：

```bash
ttsbridge voices --provider edgetts --locale zh-CN
```

### JSON 输出（--format json）

输出 `[]tts.Voice` 的 JSON 数组，适合脚本处理：

```bash
# 建议写入文件，避免终端/管道截断
ttsbridge voices --provider volcengine --format json > voices.json
```

## synthesize

把文本合成为音频文件（默认）或输出到 stdout。

```bash
ttsbridge synthesize [flags]
```

Flags（实现支持的最小集合）：

- `--provider`：`edgetts|volcengine`（默认 `edgetts`）
- `--voice`：voice ID（可选；不传则使用 provider 默认值）
- `--text`：输入文本（与 `--file` 二选一）
- `--file`：从文件读取；传 `-` 表示从 stdin 读取（与 `--text` 二选一）
- `--max-input-bytes`：从 `--file` 或 stdin 最多读取的字节数，默认 `1048576`；`0` 表示不限制
- `--out`：输出文件路径（可选；不传则自动生成 `tts_<provider>_<timestamp>.<ext>` 并在 stderr 打印）
- `--stdout`：把音频字节写到 stdout（与 `--out` 互斥）
- `--rate` / `--volume` / `--pitch`：形如 `+50%`、`-20%`，默认 `+0%`（仅 `edgetts` 支持；传入非默认值而 provider 不支持时会直接报用法错误并退出 `2`）
- `--proxy`：代理 URL（可选）
- `--http-timeout`：HTTP 超时，Go duration 格式，默认 `30s`
- `--max-attempts`：最大尝试次数（含首次），默认 `3`

Provider 说明：

- CLI 当前只接收 plain text 输入；内置 `edgetts` 与 `volcengine` 都不支持 RawSSML。
- `--voice` 省略时会使用 provider 默认音色：`edgetts=zh-CN-XiaoxiaoNeural`，`volcengine=BV700_streaming`。
- `edgetts` 支持 `--rate` / `--volume` / `--pitch`，默认输出 `mp3`。
- `volcengine` 不支持上述 prosody 参数，默认输出 `wav`；自动生成的输出文件扩展名会跟随实际结果格式。

### 示例：合成到文件

```bash
ttsbridge synthesize --provider edgetts --text "你好，欢迎使用 TTSBridge" --out out.mp3
```

### 示例：自动生成输出文件

```bash
ttsbridge synthesize --text "auto out"
# stderr 会提示生成的实际路径，例如：Output: tts_edgetts_<timestamp>.mp3
```

### 示例：从 stdin 读取

```bash
printf '你好，stdin' | ttsbridge synthesize --provider edgetts --file - --out stdin.mp3
```

PowerShell 也可以直接把管道内容传给 `--file -`。

### 示例：输出到 stdout（二进制）

`--stdout` 会输出原始音频字节；请直接重定向到文件或二进制友好的下游命令。

```bash
ttsbridge synthesize --provider edgetts --text "stdout test" --stdout > out.mp3
```

Windows 下如果当前 shell 会做文本处理，优先用 `--out`；必须重定向时可改用 `cmd /c`。

## 常见错误

- `either --text or --file must be specified`：必须提供输入来源。
- `--text and --file are mutually exclusive`：`--text/--file` 互斥。
- `--out and --stdout are mutually exclusive`：`--out/--stdout` 互斥。
- `input exceeds --max-input-bytes limit (...)`：输入文件或 stdin 超过限制；调大该值或设为 `0`。
- `provider "volcengine" does not support --rate ...`：当前 provider 不支持 rate/volume/pitch，移除相关参数或切换到 `edgetts`。
