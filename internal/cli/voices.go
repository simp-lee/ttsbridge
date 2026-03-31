package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

type voiceFetchResult struct {
	providerName string
	voices       []tts.Voice
	listErr      error
	factoryErr   error
	unavailable  bool
}

// VoicesCmd represents the voices subcommand
type VoicesCmd struct {
	fs          *flag.FlagSet
	provider    string
	locale      string
	format      string
	proxy       string
	httpTimeout time.Duration
	maxAttempts int
}

// NewVoicesCmd creates a new voices command
func NewVoicesCmd() *VoicesCmd {
	cmd := &VoicesCmd{}
	cmd.resetFlagSet()
	return cmd
}

func (c *VoicesCmd) resetFlagSet() {
	c.fs = flag.NewFlagSet("voices", flag.ContinueOnError)
	c.fs.StringVar(&c.provider, "provider", "all", "Provider: edgetts|volcengine|all")
	c.fs.StringVar(&c.locale, "locale", "", "Filter by locale, e.g. \"zh-CN\" (optional)")
	c.fs.StringVar(&c.format, "format", "text", "Output format: text|json")
	c.fs.StringVar(&c.proxy, "proxy", "", "Proxy URL (optional)")
	c.fs.DurationVar(&c.httpTimeout, "http-timeout", 30*time.Second, "HTTP timeout, e.g. 30s")
	c.fs.IntVar(&c.maxAttempts, "max-attempts", 3, "Max attempts including first")
	c.fs.Usage = c.usage
}

func (c *VoicesCmd) usage() {
	writeLineIgnoreError(c.fs.Output(), `Usage:
  ttsbridge voices [flags]

Flags:
  --provider string   Provider: edgetts|volcengine|all (default "all")
  --locale string     Filter by locale, e.g. "zh-CN" (optional)
  --format string     Output format: text|json (default "text")
	--proxy string      Proxy URL (optional)
	--http-timeout duration HTTP timeout, e.g. 30s (default 30s)
	--max-attempts int  Max attempts including first (default 3)
	-h, --help          Help for voices`)
}

// Name returns the command name
func (c *VoicesCmd) Name() string {
	return "voices"
}

// Run executes the voices command
func (c *VoicesCmd) Run(args []string, stdout, stderr io.Writer) error {
	helpShown, err := c.parseArgs(args, stderr)
	if err != nil {
		return err
	}
	if helpShown {
		return nil
	}
	if validationErr := c.validateFlags(); validationErr != nil {
		return validationErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeoutBudget(c.httpTimeout, c.maxAttempts))
	defer cancel()

	results := c.collectVoiceResults(ctx, c.providerConfig(), c.selectedProviders())
	allVoices, err := c.aggregateVoiceResults(results, stderr)
	if err != nil {
		return err
	}
	sortVoices(allVoices)
	return c.renderOutput(stdout, allVoices)
}

func (c *VoicesCmd) parseArgs(args []string, stderr io.Writer) (bool, error) {
	c.resetFlagSet()
	c.fs.SetOutput(stderr)
	if err := c.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, &UsageError{Err: err}
	}
	return false, nil
}

func (c *VoicesCmd) validateFlags() error {
	if c.format != "text" && c.format != "json" {
		return &UsageError{Message: fmt.Sprintf("invalid format: %q (expected text or json)", c.format)}
	}
	if c.maxAttempts < 1 {
		return &UsageError{Message: "--max-attempts must be >= 1"}
	}
	if c.httpTimeout <= 0 {
		return &UsageError{Message: "--http-timeout must be > 0"}
	}

	validProviders := append(ListProviders(), "all")
	if !contains(validProviders, c.provider) {
		return &UsageError{Message: fmt.Sprintf("invalid provider: %q (expected %s)", c.provider, strings.Join(validProviders, "|"))}
	}
	return nil
}

func (c *VoicesCmd) providerConfig() *ProviderConfig {
	return &ProviderConfig{
		Proxy:       c.proxy,
		HTTPTimeout: c.httpTimeout,
		MaxAttempts: c.maxAttempts,
	}
}

func (c *VoicesCmd) selectedProviders() []string {
	if c.provider == "all" {
		return ListProviders()
	}
	return []string{c.provider}
}

func (c *VoicesCmd) collectVoiceResults(ctx context.Context, cfg *ProviderConfig, providers []string) []voiceFetchResult {
	results := make([]voiceFetchResult, 0, len(providers))
	resultsCh := make(chan voiceFetchResult, len(providers))
	var wg sync.WaitGroup
	pendingProviders := make(map[string]struct{}, len(providers))

	for _, providerName := range providers {
		providerName := providerName
		pendingProviders[providerName] = struct{}{}
		wg.Add(1)
		go c.fetchVoices(ctx, cloneProviderConfig(cfg), providerName, resultsCh, &wg)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for len(pendingProviders) > 0 {
		select {
		case result, ok := <-resultsCh:
			if !ok {
				pendingProviders = nil
				break
			}
			results = append(results, result)
			delete(pendingProviders, result.providerName)
		case <-ctx.Done():
			for providerName := range pendingProviders {
				results = append(results, voiceFetchResult{providerName: providerName, listErr: ctx.Err()})
			}
			pendingProviders = nil
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].providerName < results[j].providerName
	})
	return results
}

func (c *VoicesCmd) fetchVoices(ctx context.Context, cfg *ProviderConfig, providerName string, resultsCh chan<- voiceFetchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	adapter, err := GetProvider(providerName, cfg)
	if err != nil {
		resultsCh <- voiceFetchResult{providerName: providerName, factoryErr: err}
		return
	}
	if adapter == nil {
		resultsCh <- voiceFetchResult{providerName: providerName, unavailable: true}
		return
	}

	voices, err := adapter.ListVoices(ctx, tts.VoiceFilter{Language: c.locale})
	resultsCh <- voiceFetchResult{providerName: providerName, voices: voices, listErr: err}
}

func (c *VoicesCmd) aggregateVoiceResults(results []voiceFetchResult, stderr io.Writer) ([]tts.Voice, error) {
	var allVoices []tts.Voice
	var failureMessages []string
	var failureErrors []error
	successfulProviders := 0

	for _, result := range results {
		if result.factoryErr != nil {
			return nil, usageErrorForProviderConfig(result.factoryErr)
		}
		if result.unavailable {
			adapterErr := fmt.Errorf("provider adapter unavailable")
			failureMessages = append(failureMessages, fmt.Sprintf("%s: %v", result.providerName, adapterErr))
			failureErrors = append(failureErrors, fmt.Errorf("%s: %w", result.providerName, adapterErr))
			writeFormatIgnoreError(stderr, "Warning: provider %s is unavailable\n", result.providerName)
			continue
		}
		if result.listErr != nil {
			failureMessages = append(failureMessages, fmt.Sprintf("%s: %v", result.providerName, result.listErr))
			failureErrors = append(failureErrors, fmt.Errorf("%s: %w", result.providerName, result.listErr))
			writeFormatIgnoreError(stderr, "Warning: failed to list voices from %s: %v\n", result.providerName, result.listErr)
			continue
		}
		successfulProviders++
		allVoices = append(allVoices, result.voices...)
	}

	if successfulProviders == 0 && len(failureMessages) > 0 {
		return nil, &RuntimeError{
			Message: fmt.Sprintf("failed to list voices from selected providers: %s", strings.Join(failureMessages, "; ")),
			Err:     errors.Join(failureErrors...),
		}
	}
	return allVoices, nil
}

func sortVoices(voices []tts.Voice) {
	sort.Slice(voices, func(i, j int) bool {
		if voices[i].Provider != voices[j].Provider {
			return voices[i].Provider < voices[j].Provider
		}
		if voices[i].Language != voices[j].Language {
			return voices[i].Language < voices[j].Language
		}
		return voices[i].Name < voices[j].Name
	})
}

func (c *VoicesCmd) renderOutput(stdout io.Writer, voices []tts.Voice) error {
	if c.format == "json" {
		return c.outputJSON(stdout, voices)
	}
	return c.outputText(stdout, voices)
}

func (c *VoicesCmd) outputJSON(w io.Writer, voices []tts.Voice) error {
	if voices == nil {
		voices = []tts.Voice{}
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(voices); err != nil {
		return &RuntimeError{Message: "failed to encode JSON", Err: err}
	}
	return nil
}

func (c *VoicesCmd) outputText(w io.Writer, voices []tts.Voice) error {
	for _, v := range voices {
		// Format: provider\tlanguage\tgender\tid\tname
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Provider, v.Language, v.Gender, v.ID, v.Name); err != nil {
			return &RuntimeError{Message: "failed to write output", Err: err}
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func cloneProviderConfig(cfg *ProviderConfig) *ProviderConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	return &clone
}

func commandTimeoutBudget(httpTimeout time.Duration, maxAttempts int) time.Duration {
	if httpTimeout <= 0 {
		return 0
	}
	if maxAttempts <= 1 {
		return httpTimeout
	}
	return httpTimeout * time.Duration(maxAttempts)
}
