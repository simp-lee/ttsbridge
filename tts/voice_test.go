package tts

import (
	"encoding/json"
	"testing"
)

// testExtra 用于测试的扩展结构
type testExtra struct {
	Status string   `json:"status"`
	Styles []string `json:"styles,omitempty"`
}

func TestVoice(t *testing.T) {
	t.Run("基本字段", func(t *testing.T) {
		v := Voice{
			ID:       "test-id",
			Name:     "测试语音",
			Language: LanguageZhCN,
			Gender:   GenderFemale,
			Provider: "test",
		}

		if v.ID != "test-id" {
			t.Errorf("ID = %s, want test-id", v.ID)
		}
		if v.Language != LanguageZhCN {
			t.Errorf("Language = %s, want zh-CN", v.Language)
		}
		if v.Gender != GenderFemale {
			t.Errorf("Gender = %s, want Female", v.Gender)
		}
	})

	t.Run("多语言支持", func(t *testing.T) {
		v := Voice{
			ID:        "multi-lang-voice",
			Name:      "多语言测试",
			Language:  LanguageZhCN,
			Languages: []Language{LanguageZhCN, LanguageEnUS, LanguageJaJP},
			Gender:    GenderFemale,
			Provider:  "test",
		}

		if len(v.Languages) != 3 {
			t.Errorf("len(Languages) = %d, want 3", len(v.Languages))
		}
		if v.Languages[0] != LanguageZhCN || v.Languages[1] != LanguageEnUS || v.Languages[2] != LanguageJaJP {
			t.Errorf("Languages = %v, want [zh-CN en-US ja-JP]", v.Languages)
		}
	})

	t.Run("Extra字段", func(t *testing.T) {
		extra := &testExtra{
			Status: "GA",
			Styles: []string{"cheerful", "sad"},
		}

		v := Voice{
			ID:       "test-extra",
			Name:     "Extra测试",
			Language: LanguageZhCN,
			Gender:   GenderFemale,
			Provider: "test",
			Extra:    extra,
		}

		// 使用 GetExtra 获取类型安全的 Extra
		if retrieved, ok := GetExtra[*testExtra](&v); !ok {
			t.Error("GetExtra should return ok for correct type")
		} else {
			if retrieved.Status != "GA" {
				t.Errorf("Status = %s, want GA", retrieved.Status)
			}
			if len(retrieved.Styles) != 2 {
				t.Errorf("len(Styles) = %d, want 2", len(retrieved.Styles))
			}
		}

		// 测试错误类型
		if _, ok := GetExtra[string](&v); ok {
			t.Error("GetExtra should return !ok for wrong type")
		}
	})

	t.Run("空Extra", func(t *testing.T) {
		v := Voice{
			ID:       "no-extra",
			Name:     "无Extra",
			Language: LanguageZhCN,
			Gender:   GenderMale,
			Provider: "test",
		}

		if _, ok := GetExtra[*testExtra](&v); ok {
			t.Error("GetExtra should return !ok when Extra is nil")
		}
	})

	t.Run("SupportsLanguage方法", func(t *testing.T) {
		v := Voice{
			ID:        "test-voice",
			Name:      "测试语音",
			Language:  LanguageZhCN,
			Languages: []Language{LanguageZhCN, LanguageEnUS},
			Gender:    GenderFemale,
			Provider:  "test",
		}

		// 测试主语言完全匹配
		if !v.SupportsLanguage("zh-CN") {
			t.Error("should support zh-CN (main language)")
		}

		// 测试主语言前缀匹配
		if !v.SupportsLanguage("zh") {
			t.Error("should support zh (main language prefix)")
		}

		// 测试多语言列表中的语言
		if !v.SupportsLanguage("en-US") {
			t.Error("should support en-US (in languages list)")
		}

		// 测试不支持的语言
		if v.SupportsLanguage("ja-JP") {
			t.Error("should not support ja-JP")
		}
	})

	t.Run("JSON序列化", func(t *testing.T) {
		extra := &testExtra{
			Status: "GA",
			Styles: []string{"cheerful"},
		}

		v := Voice{
			ID:        "test-json",
			Name:      "JSON测试",
			Language:  LanguageZhCN,
			Languages: []Language{LanguageZhCN, LanguageEnUS},
			Gender:    GenderFemale,
			Provider:  "test",
			Extra:     extra,
		}

		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var decoded Voice
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if decoded.ID != v.ID || decoded.Name != v.Name || decoded.Language != v.Language {
			t.Errorf("Decoded voice mismatch: got %+v, want %+v", decoded, v)
		}

		if len(decoded.Languages) != 2 {
			t.Errorf("Decoded languages length = %d, want 2", len(decoded.Languages))
		}

		// Extra 字段在 JSON 反序列化后会变成 map[string]interface{}
		if decoded.Extra == nil {
			t.Error("Extra should not be nil after unmarshal")
		}
	})

	t.Run("JSON omitempty", func(t *testing.T) {
		v := Voice{
			ID:       "test-omit",
			Name:     "省略测试",
			Language: LanguageZhCN,
			Gender:   GenderMale,
			Provider: "test",
			// Extra 为 nil，应该被省略
		}

		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("Unmarshal to map failed: %v", err)
		}

		if _, hasExtra := m["extra"]; hasExtra {
			t.Error("JSON should not contain 'extra' field when nil (omitempty)")
		}
	})

	t.Run("合并不同Provider", func(t *testing.T) {
		voices := []Voice{
			{
				ID:       "edge-1",
				Name:     "Edge语音",
				Language: LanguageZhCN,
				Gender:   GenderFemale,
				Provider: "edge",
			},
			{
				ID:       "volc-1",
				Name:     "火山语音",
				Language: LanguageZhCN,
				Gender:   GenderMale,
				Provider: "volcengine",
			},
			{
				ID:       "azure-1",
				Name:     "Azure语音",
				Language: LanguageEnUS,
				Gender:   GenderFemale,
				Provider: "azure",
			},
		}

		// 验证可以统一处理
		for _, v := range voices {
			if v.ID == "" || v.Name == "" {
				t.Errorf("Voice %+v has empty required field", v)
			}
		}

		// 验证不同的语言
		if voices[0].Language != LanguageZhCN {
			t.Errorf("voices[0].Language = %s, want zh-CN", voices[0].Language)
		}
		if voices[2].Language != LanguageEnUS {
			t.Errorf("voices[2].Language = %s, want en-US", voices[2].Language)
		}
	})
}
