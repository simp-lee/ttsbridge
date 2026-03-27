package edgetts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/simp-lee/retry"
	"github.com/simp-lee/ttsbridge/tts"
)

// ListVoices lists available voices for the specified locale.
// When voice caching is enabled (via WithVoiceCache), the cached list is used
// and filtered by locale. Otherwise the list is fetched from the remote API.
func (p *Provider) ListVoices(ctx context.Context, locale string) ([]tts.Voice, error) {
	ctx = normalizeEdgeTTSContext(ctx)
	if err := p.runtimeConfigError(); err != nil {
		return nil, err
	}
	if p.voiceCache != nil {
		return p.voiceCache.Get(ctx, locale)
	}

	entries, err := retry.DoWithResult(func() ([]voiceListEntry, error) {
		return fetchVoiceList(ctx, p.client, p.clientToken)
	}, tts.RetryOptions(ctx, p.maxAttempts)...)

	if err != nil {
		if retry.IsRetryError(err) {
			return nil, &tts.Error{
				Code:     tts.ErrCodeNetworkError,
				Message:  fmt.Sprintf("voice list retrieval failed after %d attempts", p.maxAttempts),
				Provider: providerName,
				Err:      err,
			}
		}
		return nil, err
	}

	return filterAndConvertVoices(entries, locale), nil
}

type voiceListEntry struct {
	Name                string   `json:"Name"`
	ShortName           string   `json:"ShortName"`
	LocalName           string   `json:"LocalName"`
	FriendlyName        string   `json:"FriendlyName"`
	Gender              string   `json:"Gender"`
	Locale              string   `json:"Locale"`
	SecondaryLocaleList []string `json:"SecondaryLocaleList"`
	StyleList           []string `json:"StyleList"`
	RolePlayList        []string `json:"RolePlayList"`
	SuggestedCodec      string   `json:"SuggestedCodec"`
	Status              string   `json:"Status"`
	VoiceTag            struct {
		ContentCategories  []string `json:"ContentCategories"`
		VoicePersonalities []string `json:"VoicePersonalities"`
	} `json:"VoiceTag"`
}

func fetchVoiceList(ctx context.Context, client *http.Client, token string) ([]voiceListEntry, error) {
	voiceURL := fmt.Sprintf(
		"%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		fmt.Sprintf(voicesURLTemplate, baseURL, token),
		GenerateSecMsGec(token),
		secMsGecVersion,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, voiceURL, nil)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &tts.Error{
			Code:     tts.ErrCodeNetworkError,
			Message:  "failed to build voice request",
			Provider: providerName,
			Err:      err,
		}
	}

	applyVoiceHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &tts.Error{
			Code:     tts.ErrCodeNetworkError,
			Message:  "failed to fetch voices",
			Provider: providerName,
			Err:      err,
		}
	}
	defer closeIgnoreError(resp.Body)

	// Clock skew detection and adjustment on 403 Forbidden
	if resp.StatusCode == http.StatusForbidden {
		detail := readResponseBody(io.LimitReader(resp.Body, 4096))
		return nil, classifyForbiddenResponse(resp.Header.Get("Date"), detail)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &tts.Error{
			Code:     tts.ErrCodeProviderUnavail,
			Message:  fmt.Sprintf("unexpected status: %d", resp.StatusCode),
			Provider: providerName,
		}
	}

	var entries []voiceListEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, &tts.Error{
			Code:     tts.ErrCodeInternalError,
			Message:  "failed to parse voice list",
			Provider: providerName,
			Err:      err,
		}
	}

	return entries, nil
}

func applyVoiceHeaders(req *http.Request) {
	ua := fmt.Sprintf(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0",
		chromiumMajorVersion, chromiumMajorVersion,
	)

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-CH-UA", fmt.Sprintf("\" Not;A Brand\";v=\"99\", \"Microsoft Edge\";v=\"%s\", \"Chromium\";v=\"%s\"", chromiumMajorVersion, chromiumMajorVersion))
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Authority", "speech.platform.bing.com")
	req.Header.Set("Cookie", makeMUIDCookie())
}

func filterAndConvertVoices(entries []voiceListEntry, locale string) []tts.Voice {
	trimmedLocale := strings.ToLower(strings.TrimSpace(locale))
	voices := make([]tts.Voice, 0)

	for _, raw := range entries {
		if strings.EqualFold(raw.Status, "Deprecated") {
			continue
		}

		languages := collectLanguages(raw.Locale, raw.SecondaryLocaleList)
		if len(languages) == 0 {
			continue
		}

		if trimmedLocale != "" && !matchesLocale(trimmedLocale, languages) {
			continue
		}

		voices = append(voices, tts.Voice{
			ID:        raw.ShortName,
			Name:      firstNonEmpty(raw.FriendlyName, raw.LocalName, raw.Name, raw.ShortName),
			Language:  languages[0],
			Languages: languages,
			Gender:    tts.Gender(raw.Gender),
			Provider:  providerName,
			Extra: &VoiceExtra{
				ShortName:        raw.ShortName,
				FriendlyName:     raw.FriendlyName,
				Locale:           raw.Locale,
				SecondaryLocales: copySlice(raw.SecondaryLocaleList),
				Status:           raw.Status,
				Styles:           copySlice(raw.StyleList),
				Roles:            copySlice(raw.RolePlayList),
				Categories:       copySlice(raw.VoiceTag.ContentCategories),
				Personalities:    copySlice(raw.VoiceTag.VoicePersonalities),
				SuggestedCodec:   raw.SuggestedCodec,
			},
		})
	}

	sort.Slice(voices, func(i, j int) bool {
		return voices[i].ID < voices[j].ID
	})

	return voices
}

func collectLanguages(primary string, secondary []string) []tts.Language {
	seen := make(map[string]struct{})
	languages := make([]tts.Language, 0)

	addLang := func(value string) {
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			languages = append(languages, tts.Language(value))
		}
	}

	addLang(primary)
	for _, value := range secondary {
		addLang(value)
	}

	return languages
}

func matchesLocale(query string, languages []tts.Language) bool {
	if query == "" {
		return true
	}
	if len(languages) == 0 {
		return false
	}

	voice := tts.Voice{
		Language:  languages[0],
		Languages: languages,
	}
	return voice.SupportsLanguage(query)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func copySlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}
