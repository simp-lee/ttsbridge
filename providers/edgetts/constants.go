package edgetts

const (
	providerName       = "edgetts"
	defaultClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"

	// ==== 恢复之前更旧的接口 322 个语音 ====

	baseURL           = "speech.platform.bing.com/consumer/speech/synthesize/readaloud"
	wsURLTemplate     = "wss://%s/edge/v1?TrustedClientToken=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s"
	voicesURLTemplate = "https://%s/voices/list?trustedclienttoken=%s"

	// ==== 旧版接口（2025年12月09日起不能用了）560-610 个语音 ====

	// baseURL           = "api.msedgeservices.com/tts/cognitiveservices"
	// wsURLTemplate     = "wss://%s/websocket/v1?Ocp-Apim-Subscription-Key=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s"
	// voicesURLTemplate = "https://%s/voices/list?Ocp-Apim-Subscription-Key=%s"

	// chromiumFullVersion 是 Chromium 完整版本号，来自 edge-tts Python 库 v7.2.6。
	// 最后同步: 2026-02-20。
	// 检查上游: https://github.com/rany2/edge-tts/blob/master/src/edge_tts/constants.py
	// 对应上游常量: CHROMIUM_FULL_VERSION
	//
	// 此版本号直接影响 DRM GEC token 生成（secMsGecVersion 依赖它），
	// 版本不匹配将导致 WebSocket 连接失败（服务端校验 Sec-MS-GEC 签名）。
	// 更新步骤:
	//   1. 查看上游 constants.py 中 CHROMIUM_FULL_VERSION 是否变化
	//   2. 同步更新 chromiumFullVersion 和 chromiumMajorVersion
	//   3. 运行 edgetts 测试验证连接正常
	chromiumFullVersion = "143.0.3650.75"
	// chromiumMajorVersion 是 Chromium 主版本号，从 chromiumFullVersion 提取。
	// 用于构建 User-Agent 等 HTTP 头部。与 chromiumFullVersion 保持同步。
	chromiumMajorVersion = "143"
	// secMsGecVersion 是 Sec-MS-GEC-Version 请求头的值，格式为 "1-{chromiumFullVersion}"。
	// 该值与 GenerateSecMsGec() 生成的 token 配对使用，参见 drm.go。
	secMsGecVersion = "1-" + chromiumFullVersion

	defaultOutputFormat  = "audio-24khz-48kbitrate-mono-mp3"
	defaultMaxSSMLBytes  = 4096
	defaultVoice         = "zh-CN-XiaoxiaoNeural" // Default voice when not specified
)
