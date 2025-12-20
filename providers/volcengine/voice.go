package volcengine

import "slices"

// VoiceExtra Volcengine 语音的扩展信息
// 存储在 tts.Voice.Extra 字段中
type VoiceExtra struct {
	// Volcengine 特有字段
	// Category 适配场景: "通用场景", "有声阅读", "智能助手/视频配音/特色/教育", "方言", "英语", "日语"
	Category    string   `json:"category"`
	SceneTags   []string `json:"scene_tags,omitempty"`   // 场景标签
	EmotionTags []string `json:"emotion_tags,omitempty"` // 情感标签

	// 音频格式
	Format     string `json:"format"`      // "wav" - WAV容器, PCM_S16LE编码
	SampleRate int    `json:"sample_rate"` // 24000 Hz

	// 流式支持
	SupportsStreaming bool `json:"supports_streaming"` // true
}

// HasSceneTag 检查是否支持指定场景标签
func (e *VoiceExtra) HasSceneTag(tag string) bool {
	return slices.Contains(e.SceneTags, tag)
}

// HasEmotionTag 检查是否支持指定情感标签
func (e *VoiceExtra) HasEmotionTag(tag string) bool {
	return slices.Contains(e.EmotionTags, tag)
}
