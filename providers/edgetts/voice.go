package edgetts

// VoiceExtra EdgeTTS voice extended information (reference edge-tts Voice structure)
type VoiceExtra struct {
	ShortName        string   `json:"short_name"`
	FriendlyName     string   `json:"friendly_name,omitempty"`
	Locale           string   `json:"locale"`
	SecondaryLocales []string `json:"secondary_locales,omitempty"`
	Status           string   `json:"status,omitempty"` // "GA", "Preview", "Deprecated"
	Styles           []string `json:"styles,omitempty"`
	Roles            []string `json:"roles,omitempty"`
	Categories       []string `json:"categories,omitempty"`
	Personalities    []string `json:"personalities,omitempty"`
	SuggestedCodec   string   `json:"suggested_codec,omitempty"`
}
