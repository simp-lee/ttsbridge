package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	fmt.Fprintln(c.fs.Output(), `Usage:
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
	c.resetFlagSet()
	c.fs.SetOutput(stderr)
	if err := c.fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil // Help was printed, exit successfully
		}
		return &UsageError{Err: err}
	}
	if c.maxInputBytes < 0 {
		return &UsageError{Message: "--max-input-bytes must be >= 0"}
	}

	cfg := &ProviderConfig{
		Proxy:       c.proxy,
		HTTPTimeout: c.httpTimeout,
		MaxAttempts: c.maxAttempts,
	}

	adapter := GetProvider(c.provider, cfg)
	if adapter == nil {
		return &UsageError{Message: fmt.Sprintf("unknown provider: %q (available: %s)", c.provider, strings.Join(ListProviders(), ", "))}
	}

	// Validate text/file mutual exclusivity
	if c.text == "" && c.file == "" {
		return &UsageError{Message: "either --text or --file must be specified"}
	}
	if c.text != "" && c.file != "" {
		return &UsageError{Message: "--text and --file are mutually exclusive"}
	}

	// Validate out/stdout mutual exclusivity
	if c.out != "" && c.stdout {
		return &UsageError{Message: "--out and --stdout are mutually exclusive"}
	}

	// Check rate/volume/pitch support
	hasRateVolumePitch := c.rate != "+0%" || c.volume != "+0%" || c.pitch != "+0%"
	if hasRateVolumePitch && !adapter.SupportsRateVolumePitch() {
		var unsupported []string
		if c.rate != "+0%" {
			unsupported = append(unsupported, "--rate")
		}
		if c.volume != "+0%" {
			unsupported = append(unsupported, "--volume")
		}
		if c.pitch != "+0%" {
			unsupported = append(unsupported, "--pitch")
		}
		return &UsageError{
			Message: fmt.Sprintf("provider %q does not support %s parameter(s)\nHint: remove %s or use --provider edgetts",
				c.provider, strings.Join(unsupported, ", "), strings.Join(unsupported, ", ")),
		}
	}

	// Parse rate/volume/pitch
	rate, err := ParseRatePercent(c.rate)
	if err != nil {
		return &UsageError{Message: fmt.Sprintf("invalid --rate: %v", err)}
	}
	volume, err := ParseVolumePercent(c.volume)
	if err != nil {
		return &UsageError{Message: fmt.Sprintf("invalid --volume: %v", err)}
	}
	pitch, err := ParsePitchPercent(c.pitch)
	if err != nil {
		return &UsageError{Message: fmt.Sprintf("invalid --pitch: %v", err)}
	}

	// Read input text
	inputText, err := c.readInputText()
	if err != nil {
		var usageErr *UsageError
		if errors.As(err, &usageErr) {
			return usageErr
		}
		return &RuntimeError{Message: fmt.Sprintf("failed to read input text: %v", err)}
	}
	if strings.TrimSpace(inputText) == "" {
		return &UsageError{Message: "input text cannot be empty"}
	}

	// Set default voice if not specified
	voice := c.voice
	if voice == "" {
		voice = adapter.DefaultVoice()
	}

	// Build synthesis request
	req := &SynthesizeRequest{
		Text:   inputText,
		Voice:  voice,
		Rate:   rate,
		Volume: volume,
		Pitch:  pitch,
	}

	// Synthesize
	ctx := context.Background()
	audioData, err := adapter.Synthesize(ctx, req)
	if err != nil {
		return &RuntimeError{Message: fmt.Sprintf("synthesis failed: %v", err)}
	}

	// Write output
	if c.stdout {
		_, err = stdoutWriter.Write(audioData)
		return err
	}

	outputPath := c.out
	if outputPath == "" {
		outputPath = c.generateOutputPath(adapter.DefaultFormat())
		fmt.Fprintf(stderr, "Output: %s\n", outputPath)
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &RuntimeError{Message: fmt.Sprintf("failed to create output directory: %v", err)}
		}
	}

	if err := os.WriteFile(outputPath, audioData, 0644); err != nil {
		return &RuntimeError{Message: fmt.Sprintf("failed to write output file: %v", err)}
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
		f, err := os.Open(c.file)
		if err != nil {
			if os.IsNotExist(err) {
				return "", &UsageError{Message: fmt.Sprintf("input file not found: %q", c.file)}
			}
			return "", &RuntimeError{Message: fmt.Sprintf("failed to open input file %q: %v", c.file, err)}
		}
		defer f.Close()
		reader = f
	}

	if c.maxInputBytes == 0 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", err
		}
		return string(data), nil
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

func (c *SynthesizeCmd) generateOutputPath(format string) string {
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("tts_%s_%s.%s", c.provider, timestamp, format)
}
