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

	chromiumFullVersion  = "143.0.3650.75"
	chromiumMajorVersion = "143"
	secMsGecVersion      = "1-" + chromiumFullVersion

	defaultOutputFormat  = "audio-24khz-48kbitrate-mono-mp3"
	defaultMaxSSMLBytes  = 4096
	defaultOffsetPadding = 8750000                // Average offset compensation (ticks)
	defaultVoice         = "zh-CN-XiaoxiaoNeural" // Default voice when not specified
)
