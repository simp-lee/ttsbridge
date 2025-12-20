# TTSBridge CLI（ttsbridge）

本目录包含 `ttsbridge` 命令行工具入口（见 `cmd/ttsbridge/main.go`）。

本文档以“当前实现”为准，命令只有 `voices` 和 `synthesize`，强调可脚本化（stdout/stderr 分离）与稳定的退出码。

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
- `--locale`：按语言过滤，例如 `zh-CN`（可选）
- `--format`：`text|json`（默认 `text`）

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
- `--out`：输出文件路径（可选；不传则自动生成 `tts_<provider>_<timestamp>.<ext>` 并在 stderr 打印）
- `--stdout`：把音频字节写到 stdout（与 `--out` 互斥）
- `--rate` / `--volume` / `--pitch`：形如 `+50%`、`-20%`（仅 `edgetts` 支持；不支持时会直接报用法错误并退出 2）
- `--proxy`：代理 URL（可选）
- `--http-timeout`：HTTP 超时（Go duration，例如 `30s`）
- `--max-attempts`：最大尝试次数（含首次），默认 `3`

### 示例：合成到文件

```bash
ttsbridge synthesize --provider edgetts --text "你好，欢迎使用 TTSBridge" --out out.mp3
```

### 示例：自动输出文件名

```bash
ttsbridge synthesize --text "auto out"
# stderr 会提示：Output: tts_edgetts_YYYYMMDD_HHMMSS.mp3
```

### 示例：从 stdin 读取

```bash
# PowerShell
"你好，stdin" | ttsbridge synthesize --provider edgetts --file - --out stdin.mp3
```

### 示例：输出到 stdout（二进制）

`--stdout` 会输出原始音频字节；请避免把它直接管到会做文本处理的命令。

- 推荐：优先用 `--out`。
- 如果确实需要重定向 stdout 到文件，Windows 上建议用 `cmd` 的 `>`：

```bat
cmd /c "ttsbridge synthesize --provider edgetts --text \"stdout test\" --stdout > out.mp3"
```

## 常见错误

- `either --text or --file must be specified`：必须提供输入来源。
- `--text and --file are mutually exclusive`：`--text/--file` 互斥。
- `--out and --stdout are mutually exclusive`：`--out/--stdout` 互斥。
- `provider "volcengine" does not support --rate ...`：当前 provider 不支持 rate/volume/pitch，移除相关参数或切换到 `edgetts`。
