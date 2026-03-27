package tts

import "strings"

// VoiceFilter 语音筛选条件
type VoiceFilter struct {
	Language   string           // 语言筛选（前缀匹配，如 "en" 匹配 "en-US"），空字符串表示不筛选
	Gender     Gender           // 性别筛选，空字符串表示不筛选
	Provider   string           // Provider 名称筛选，空字符串表示不筛选
	FilterFunc func(Voice) bool // 自定义筛选函数，返回 true 表示保留，nil 表示不使用自定义筛选
}

// FilterVoices 根据条件筛选语音列表
// 所有非零值的筛选条件取交集（AND 逻辑）
func FilterVoices(voices []Voice, filter VoiceFilter) []Voice {
	language := strings.TrimSpace(filter.Language)

	var result []Voice
	for i := range voices {
		v := &voices[i]
		if language != "" && !v.SupportsLanguage(language) {
			continue
		}
		if filter.Gender != "" && v.Gender != filter.Gender {
			continue
		}
		if filter.Provider != "" && v.Provider != filter.Provider {
			continue
		}
		if filter.FilterFunc != nil && !filter.FilterFunc(*v) {
			continue
		}
		result = append(result, cloneVoice(*v))
	}
	return result
}
