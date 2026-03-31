package volcengine

import "github.com/simp-lee/ttsbridge/tts"

// voiceWithExtra 辅助函数，创建带 Extra 的 Voice
func voiceWithExtra(id, name string, language tts.Language, gender tts.Gender, category string, extraLanguages ...tts.Language) tts.Voice {
	voice := tts.Voice{
		ID:       id,
		Name:     name,
		Language: language,
		Gender:   gender,
		Provider: providerName,
		Extra: &VoiceExtra{
			Category:          category,
			Format:            "wav", // WAV容器, PCM_S16LE编码
			SampleRate:        24000,
			SupportsStreaming: false,
		},
	}
	if len(extraLanguages) > 0 {
		voice.Languages = make([]tts.Language, 0, len(extraLanguages)+1)
		voice.Languages = append(voice.Languages, language)
		voice.Languages = append(voice.Languages, extraLanguages...)
	}
	return voice
}

// GetAllVoices 返回火山翻译 TTS 支持的所有可用语音（21款免费音色）
// 文档: https://www.volcengine.com/docs/6561/97465
func GetAllVoices() []tts.Voice {
	return []tts.Voice{
		// 通用场景 (3款)
		voiceWithExtra("BV700_streaming", "灿灿", tts.LanguageZhCN, tts.GenderFemale, "通用场景", tts.LanguageEnUS, tts.LanguageJaJP, tts.LanguagePtBR, tts.LanguageEsMX, tts.LanguageIDID),
		voiceWithExtra("BV001_streaming", "通用女声", tts.LanguageZhCN, tts.GenderFemale, "通用场景"),
		voiceWithExtra("BV002_streaming", "通用男声", tts.LanguageZhCN, tts.GenderMale, "通用场景"),

		// 有声阅读 (5款)
		voiceWithExtra("BV701_streaming", "擎苍", tts.LanguageZhCN, tts.GenderMale, "有声阅读"),
		voiceWithExtra("BV119_streaming", "通用赞婚", tts.LanguageZhCN, tts.GenderFemale, "有声阅读"),
		voiceWithExtra("BV102_streaming", "儒雅青年", tts.LanguageZhCN, tts.GenderMale, "有声阅读"),
		voiceWithExtra("BV113_streaming", "甜宠少御", tts.LanguageZhCN, tts.GenderFemale, "有声阅读"),
		voiceWithExtra("BV115_streaming", "古风少御", tts.LanguageZhCN, tts.GenderFemale, "有声阅读"),

		// 智能助手/视频配音/特色/教育 (6款)
		voiceWithExtra("BV007_streaming", "亲切女声", tts.LanguageZhCN, tts.GenderFemale, "智能助手/视频配音/特色/教育"),
		voiceWithExtra("BV056_streaming", "阳光男声", tts.LanguageZhCN, tts.GenderMale, "智能助手/视频配音/特色/教育"),
		voiceWithExtra("BV005_streaming", "活泼女声", tts.LanguageZhCN, tts.GenderFemale, "智能助手/视频配音/特色/教育"),
		voiceWithExtra("BV051_streaming", "奶气萌娃", tts.LanguageZhCN, tts.GenderFemale, "智能助手/视频配音/特色/教育"),
		voiceWithExtra("BV034_streaming", "知性姐姐-双语", tts.LanguageZhCN, tts.GenderFemale, "智能助手/视频配音/特色/教育", tts.LanguageEnUS),
		voiceWithExtra("BV033_streaming", "温柔小哥", tts.LanguageZhCN, tts.GenderMale, "智能助手/视频配音/特色/教育"),

		// 方言 (3款)
		voiceWithExtra("BV021_streaming", "东北老铁", tts.LanguageZhCN, tts.GenderMale, "方言"),
		voiceWithExtra("BV019_streaming", "重庆小伙", tts.LanguageZhCN, tts.GenderMale, "方言"),
		voiceWithExtra("BV213_streaming", "广西表哥", tts.LanguageZhCN, tts.GenderMale, "方言"),

		// 英语 (2款)
		voiceWithExtra("BV503_streaming", "Ariana", tts.LanguageEnUS, tts.GenderFemale, "英语"),
		voiceWithExtra("BV504_streaming", "Jackson", tts.LanguageEnUS, tts.GenderMale, "英语"),

		// 日语 (2款)
		voiceWithExtra("BV522_streaming", "气质女生", tts.LanguageJaJP, tts.GenderFemale, "日语"),
		voiceWithExtra("BV524_streaming", "日语男声", tts.LanguageJaJP, tts.GenderMale, "日语"),
	}
}
