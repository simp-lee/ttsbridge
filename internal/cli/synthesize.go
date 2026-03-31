package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simp-lee/ttsbridge/tts"
)

var now = time.Now

// SynthesizeCmd represents the synthesize subcommand
type SynthesizeCmd struct {
	fs            *flag.FlagSet
	provider      string
	voice         string
	text          string
	file          string
	maxInputBytes int64
	out           string
	stdout        bool
	rate          string
	volume        string
	pitch         string
	proxy         string
	httpTimeout   time.Duration
	maxAttempts   int
}

// NewSynthesizeCmd creates a new synthesize command
func NewSynthesizeCmd() *SynthesizeCmd {
	cmd := &SynthesizeCmd{}
	cmd.resetFlagSet()
	return cmd
}

func (c *SynthesizeCmd) resetFlagSet() {
	c.fs = flag.NewFlagSet("synthesize", flag.ContinueOnError)
	c.fs.StringVar(&c.provider, "provider", "edgetts", "Provider: edgetts|volcengine")
	c.fs.StringVar(&c.voice, "voice", "", "Voice ID (default per provider)")
	c.fs.StringVar(&c.text, "text", "", "Input text (conflicts with --file)")
	c.fs.StringVar(&c.file, "file", "", "Read input text from file; use \"-\" for stdin (conflicts with --text)")
	c.fs.Int64Var(&c.maxInputBytes, "max-input-bytes", 1<<20, "Max bytes to read from --file/- (stdin); 0 disables the limit")
	c.fs.StringVar(&c.out, "out", "", "Output file path (optional, auto-generated if omitted)")
	c.fs.BoolVar(&c.stdout, "stdout", false, "Write audio bytes to stdout")
	c.fs.StringVar(&c.rate, "rate", "+0%", "Rate adjustment, e.g. \"+50%\" (edgetts only)")
	c.fs.StringVar(&c.volume, "volume", "+0%", "Volume adjustment, e.g. \"+50%\" (edgetts only)")
	c.fs.StringVar(&c.pitch, "pitch", "+0%", "Pitch adjustment, e.g. \"+50%\" (edgetts only)")
	c.fs.StringVar(&c.proxy, "proxy", "", "Proxy URL (optional)")
	c.fs.DurationVar(&c.httpTimeout, "http-timeout", 30*time.Second, "HTTP timeout, e.g. 30s")
	c.fs.IntVar(&c.maxAttempts, "max-attempts", 3, "Max attempts including first")
	c.fs.Usage = c.usage
}

func (c *SynthesizeCmd) usage() {
	writeLineIgnoreError(c.fs.Output(), `Usage:
  ttsbridge synthesize [flags]

Flags:
  --provider string       Provider: edgetts|volcengine (default "edgetts")
  --voice string          Voice ID (default per provider: edgetts="zh-CN-XiaoxiaoNeural")
  --text string           Input text (conflicts with --file)
  --file string           Read input text from file; use "-" for stdin (conflicts with --text)
	--max-input-bytes int   Max bytes to read from --file/- (stdin); 0 disables limit (default 1048576)
  --out string            Output file path (optional, auto-generated if omitted)
  --stdout                Write audio bytes to stdout
  --rate string           Rate adjustment, e.g. "+50%" (edgetts only, default "+0%")
  --volume string         Volume adjustment, e.g. "+50%" (edgetts only, default "+0%")
  --pitch string          Pitch adjustment, e.g. "+50%" (edgetts only, default "+0%")
  --proxy string          Proxy URL (optional)
  --http-timeout duration HTTP timeout, e.g. 30s (default 30s)
  --max-attempts int      Max attempts including first (default 3)
	-h, --help              Help for synthesize`)
}

// Name returns the command name
func (c *SynthesizeCmd) Name() string {
	return "synthesize"
}

// Run executes the synthesize command
func (c *SynthesizeCmd) Run(args []string, stdoutWriter, stderr io.Writer) error {
	helpShown, err := c.parseArgs(args, stderr)
	if err != nil {
		return err
	}
	if helpShown {
		return nil
	}
	if numericErr := c.validateNumericFlags(); numericErr != nil {
		return numericErr
	}

	adapter, capabilities, err := c.loadProvider()
	if err != nil {
		return err
	}
	if inputErr := c.validateInputFlags(); inputErr != nil {
		return inputErr
	}

	prosodyFlags, err := c.parseProsodyFlags()
	if err != nil {
		return err
	}
	err = c.validateProsodySupport(capabilities, prosodyFlags)
	if err != nil {
		return err
	}
	req, err := c.buildRequest(prosodyFlags)
	if err != nil {
		return err
	}

	result, err := adapter.Synthesize(context.Background(), req)
	if err != nil {
		return &RuntimeError{Message: "synthesis failed", Err: err}
	}

	return c.writeResult(result, capabilities, stdoutWriter, stderr)
}

func (c *SynthesizeCmd) parseArgs(args []string, stderr io.Writer) (bool, error) {
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

func (c *SynthesizeCmd) validateNumericFlags() error {
	if c.maxInputBytes < 0 {
		return &UsageError{Message: "--max-input-bytes must be >= 0"}
	}
	if c.maxAttempts < 1 {
		return &UsageError{Message: "--max-attempts must be >= 1"}
	}
	if c.httpTimeout <= 0 {
		return &UsageError{Message: "--http-timeout must be > 0"}
	}
	return nil
}

func (c *SynthesizeCmd) loadProvider() (tts.Provider, tts.ProviderCapabilities, error) {
	adapter, err := GetProvider(c.provider, c.providerConfig())
	if err != nil {
		return nil, tts.ProviderCapabilities{}, usageErrorForProviderConfig(err)
	}
	if adapter == nil {
		return nil, tts.ProviderCapabilities{}, &UsageError{Message: fmt.Sprintf("unknown provider: %q (available: %s)", c.provider, strings.Join(ListProviders(), ", "))}
	}
	return adapter, adapter.Capabilities(), nil
}

func (c *SynthesizeCmd) providerConfig() *ProviderConfig {
	return &ProviderConfig{
		Proxy:       c.proxy,
		HTTPTimeout: c.httpTimeout,
		MaxAttempts: c.maxAttempts,
	}
}

func (c *SynthesizeCmd) validateInputFlags() error {
	if c.text == "" && c.file == "" {
		return &UsageError{Message: "either --text or --file must be specified"}
	}
	if c.text != "" && c.file != "" {
		return &UsageError{Message: "--text and --file are mutually exclusive"}
	}
	if c.out != "" && c.stdout {
		return &UsageError{Message: "--out and --stdout are mutually exclusive"}
	}
	return nil
}

type parsedProsodyFlags struct {
	rate      float64
	volume    float64
	pitch     float64
	hasRate   bool
	hasVolume bool
	hasPitch  bool
}

func (p parsedProsodyFlags) enabled() bool {
	return p.hasRate || p.hasVolume || p.hasPitch
}

func (p parsedProsodyFlags) unsupportedFlags() []string {
	unsupported := make([]string, 0, 3)
	if p.hasRate {
		unsupported = append(unsupported, "--rate")
	}
	if p.hasVolume {
		unsupported = append(unsupported, "--volume")
	}
	if p.hasPitch {
		unsupported = append(unsupported, "--pitch")
	}
	return unsupported
}

func (p parsedProsodyFlags) toProsodyParams() tts.ProsodyParams {
	prosody := tts.ProsodyParams{}

	if p.hasRate {
		prosody.Rate = p.rate
	}
	if p.hasVolume {
		if p.volume == 0 {
			prosody = prosody.WithVolume(0)
		} else {
			prosody.Volume = p.volume
		}
	}
	if p.hasPitch {
		prosody.Pitch = p.pitch
	}

	return prosody
}

func (c *SynthesizeCmd) parseProsodyFlags() (parsedProsodyFlags, error) {
	rate, err := ParseRatePercent(c.rate)
	if err != nil {
		return parsedProsodyFlags{}, &UsageError{Message: "invalid --rate", Err: err}
	}

	volume, err := ParseVolumePercent(c.volume)
	if err != nil {
		return parsedProsodyFlags{}, &UsageError{Message: "invalid --volume", Err: err}
	}

	pitch, err := ParsePitchPercent(c.pitch)
	if err != nil {
		return parsedProsodyFlags{}, &UsageError{Message: "invalid --pitch", Err: err}
	}

	return parsedProsodyFlags{
		rate:      rate,
		volume:    volume,
		pitch:     pitch,
		hasRate:   isActiveProsodyMultiplier(rate),
		hasVolume: isActiveProsodyMultiplier(volume),
		hasPitch:  isActiveProsodyMultiplier(pitch),
	}, nil
}

func isActiveProsodyMultiplier(value float64) bool {
	return math.Abs(value-1.0) > 1e-9
}

func (c *SynthesizeCmd) validateProsodySupport(capabilities tts.ProviderCapabilities, prosodyFlags parsedProsodyFlags) error {
	if !prosodyFlags.enabled() || capabilities.ProsodyParams {
		return nil
	}

	unsupported := prosodyFlags.unsupportedFlags()
	return &UsageError{
		Message: fmt.Sprintf("provider %q does not support %s parameter(s)\nHint: remove %s or use --provider edgetts",
			c.provider, strings.Join(unsupported, ", "), strings.Join(unsupported, ", ")),
	}
}

func (c *SynthesizeCmd) buildRequest(prosodyFlags parsedProsodyFlags) (tts.SynthesisRequest, error) {
	inputText, err := c.inputText()
	if err != nil {
		return tts.SynthesisRequest{}, err
	}

	req := tts.SynthesisRequest{
		InputMode: tts.InputModePlainText,
		Text:      inputText,
		VoiceID:   c.resolvedVoice(),
	}
	if !prosodyFlags.enabled() {
		return req, nil
	}

	req.InputMode = tts.InputModePlainTextWithProsody
	req.Prosody = prosodyFlags.toProsodyParams()
	return req, nil
}

func (c *SynthesizeCmd) inputText() (string, error) {
	inputText, err := c.readInputText()
	if err != nil {
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			return "", usageErr
		}
		return "", &RuntimeError{Message: "failed to read input text", Err: err}
	}
	if strings.TrimSpace(inputText) == "" {
		return "", &UsageError{Message: "input text cannot be empty"}
	}
	return inputText, nil
}

func (c *SynthesizeCmd) resolvedVoice() string {
	if c.voice != "" {
		return c.voice
	}
	return GetProviderDefaultVoice(c.provider)
}

func (c *SynthesizeCmd) writeResult(result *tts.SynthesisResult, capabilities tts.ProviderCapabilities, stdoutWriter, stderr io.Writer) error {
	if c.stdout {
		if _, err := stdoutWriter.Write(result.Audio); err != nil {
			return &RuntimeError{Message: "failed to write output", Err: err}
		}
		return nil
	}
	if c.out == "" {
		return c.writeAutoOutput(result, capabilities, stderr)
	}
	return c.writeNamedOutput(c.out, result.Audio)
}

func (c *SynthesizeCmd) writeAutoOutput(result *tts.SynthesisResult, capabilities tts.ProviderCapabilities, stderr io.Writer) error {
	format := result.Format
	if format == "" {
		format = capabilities.ResolvedOutputFormat("")
	}

	outputFile, generatedPath, err := c.createAutoOutputFile(format)
	if err != nil {
		return &RuntimeError{Message: "failed to create output file", Err: err}
	}
	if err := writeOutputFile(outputFile, generatedPath, result.Audio); err != nil {
		return &RuntimeError{Message: "failed to write output file", Err: err}
	}
	writeFormatIgnoreError(stderr, "Output: %s\n", generatedPath)
	return nil
}

func (c *SynthesizeCmd) writeNamedOutput(outputPath string, data []byte) error {
	cleanOutputPath := filepath.Clean(outputPath)
	dir := filepath.Dir(cleanOutputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return &RuntimeError{Message: "failed to create output directory", Err: err}
		}
	}
	if err := os.WriteFile(cleanOutputPath, data, 0o600); err != nil {
		return &RuntimeError{Message: "failed to write output file", Err: err}
	}
	return nil
}

func (c *SynthesizeCmd) readInputText() (string, error) {
	if c.text != "" {
		return c.text, nil
	}

	var reader io.Reader
	if c.file == "-" {
		reader = os.Stdin
	} else {
		cleanInputPath := filepath.Clean(c.file)
		f, err := os.Open(cleanInputPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", &UsageError{Message: fmt.Sprintf("input file not found: %q", c.file)}
			}
			return "", &RuntimeError{Message: fmt.Sprintf("failed to open input file %q", c.file), Err: err}
		}
		defer func() {
			_ = f.Close()
		}()
		reader = f
	}

	if c.maxInputBytes == 0 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	if c.maxInputBytes == math.MaxInt64 {
		return "", &UsageError{Message: "--max-input-bytes must be less than 9223372036854775807"}
	}

	limitedReader := &io.LimitedReader{R: reader, N: c.maxInputBytes + 1}
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > c.maxInputBytes {
		return "", &UsageError{Message: fmt.Sprintf("input exceeds --max-input-bytes limit (%d bytes)", c.maxInputBytes)}
	}

	return string(data), nil
}

func (c *SynthesizeCmd) generateOutputPath(format string, suffix int) string {
	format = sanitizeOutputFormatSuffix(format)
	timestamp := now().Format("20060102_150405.000000000")
	base := fmt.Sprintf("tts_%s_%s", c.provider, timestamp)
	if suffix == 0 {
		return fmt.Sprintf("%s.%s", base, format)
	}
	return fmt.Sprintf("%s_%d.%s", base, suffix, format)
}

func sanitizeOutputFormatSuffix(format string) string {
	trimmed := strings.ToLower(strings.TrimSpace(format))
	if trimmed == "" {
		return "bin"
	}

	var builder strings.Builder
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "bin"
	}
	return builder.String()
}

func (c *SynthesizeCmd) createAutoOutputFile(format string) (*os.File, string, error) {
	for suffix := 0; ; suffix++ {
		path := filepath.Clean(c.generateOutputPath(format, suffix))
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return file, path, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return nil, "", err
	}
}

func writeOutputFile(file *os.File, path string, data []byte) error {
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return nil
}
