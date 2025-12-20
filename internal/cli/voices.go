package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/simp-lee/ttsbridge/tts"
)

// VoicesCmd represents the voices subcommand
type VoicesCmd struct {
	fs       *flag.FlagSet
	provider string
	locale   string
	format   string
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
	c.fs.Usage = c.usage
}

func (c *VoicesCmd) usage() {
	fmt.Fprintln(c.fs.Output(), `Usage:
  ttsbridge voices [flags]

Flags:
  --provider string   Provider: edgetts|volcengine|all (default "all")
  --locale string     Filter by locale, e.g. "zh-CN" (optional)
  --format string     Output format: text|json (default "text")
  -h, --help          Help for voices`)
}

// Name returns the command name
func (c *VoicesCmd) Name() string {
	return "voices"
}

// Run executes the voices command
func (c *VoicesCmd) Run(args []string, stdout, stderr io.Writer) error {
	c.resetFlagSet()
	c.fs.SetOutput(stderr)
	if err := c.fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil // Help was printed, exit successfully
		}
		return &UsageError{Err: err}
	}

	// Validate format
	if c.format != "text" && c.format != "json" {
		return &UsageError{Message: fmt.Sprintf("invalid format: %q (expected text or json)", c.format)}
	}

	// Validate provider
	validProviders := append(ListProviders(), "all")
	if !contains(validProviders, c.provider) {
		return &UsageError{Message: fmt.Sprintf("invalid provider: %q (expected %s)", c.provider, strings.Join(validProviders, "|"))}
	}

	ctx := context.Background()
	cfg := &ProviderConfig{}

	var allVoices []tts.Voice

	// Collect voices from providers
	providers := ListProviders()
	if c.provider != "all" {
		providers = []string{c.provider}
	}

	for _, providerName := range providers {
		adapter := GetProvider(providerName, cfg)
		if adapter == nil {
			continue
		}

		voices, err := adapter.ListVoices(ctx, c.locale)
		if err != nil {
			fmt.Fprintf(stderr, "Warning: failed to list voices from %s: %v\n", providerName, err)
			continue
		}
		allVoices = append(allVoices, voices...)
	}

	// Sort by provider, then by language, then by name
	sort.Slice(allVoices, func(i, j int) bool {
		if allVoices[i].Provider != allVoices[j].Provider {
			return allVoices[i].Provider < allVoices[j].Provider
		}
		if allVoices[i].Language != allVoices[j].Language {
			return allVoices[i].Language < allVoices[j].Language
		}
		return allVoices[i].Name < allVoices[j].Name
	})

	// Output
	if c.format == "json" {
		return c.outputJSON(stdout, allVoices)
	}
	return c.outputText(stdout, allVoices)
}

func (c *VoicesCmd) outputJSON(w io.Writer, voices []tts.Voice) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(voices); err != nil {
		return &RuntimeError{Message: fmt.Sprintf("failed to encode JSON: %v", err)}
	}
	return nil
}

func (c *VoicesCmd) outputText(w io.Writer, voices []tts.Voice) error {
	for _, v := range voices {
		// Format: provider\tlanguage\tgender\tid\tname
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Provider, v.Language, v.Gender, v.ID, v.Name); err != nil {
			return &RuntimeError{Message: fmt.Sprintf("failed to write output: %v", err)}
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
