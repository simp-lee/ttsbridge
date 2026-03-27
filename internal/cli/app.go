package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// App is the main CLI application
type App struct {
	version  string
	commit   string
	date     string
	exitCode int
	stdout   io.Writer
	stderr   io.Writer
	commands map[string]Command
	globalFS *flag.FlagSet
	showHelp bool
	showVer  bool
}

// Command is a subcommand interface
type Command interface {
	Name() string
	Run(args []string, stdout, stderr io.Writer) error
}

// NewApp creates a new CLI application
func NewApp(version, commit, date string) *App {
	app := &App{
		version:  version,
		commit:   commit,
		date:     date,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		commands: make(map[string]Command),
	}

	// Register commands
	app.registerCommand(NewVoicesCmd())
	app.registerCommand(NewSynthesizeCmd())

	return app
}

func (a *App) newGlobalFlagSet() *flag.FlagSet {
	a.showHelp = false
	a.showVer = false

	fs := flag.NewFlagSet("ttsbridge", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.BoolVar(&a.showHelp, "help", false, "Show help")
	fs.BoolVar(&a.showHelp, "h", false, "Show help")
	fs.BoolVar(&a.showVer, "version", false, "Show version")
	fs.BoolVar(&a.showVer, "V", false, "Show version")
	fs.Usage = a.usage
	return fs
}

func (a *App) registerCommand(cmd Command) {
	if _, exists := a.commands[cmd.Name()]; exists {
		panic(fmt.Sprintf("command %q already registered", cmd.Name()))
	}
	a.commands[cmd.Name()] = cmd
}

func (a *App) usage() {
	fmt.Fprintln(a.stderr, `TTSBridge - Text-to-Speech CLI Tool

Usage:
  ttsbridge [flags]
  ttsbridge <command> [flags]

Available Commands:
  voices      List available voices
  synthesize  Synthesize text to speech

Flags:
  -h, --help      Show help
  -V, --version   Show version

Use "ttsbridge <command> --help" for more information about a command.`)
}

// Run executes the CLI application
func (a *App) Run(args []string) error {
	a.exitCode = ExitSuccess
	a.globalFS = a.newGlobalFlagSet()

	if err := a.globalFS.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		a.exitCode = ExitUsageError
		return err
	}

	if a.showVer {
		a.printVersion()
		return nil
	}

	remaining := a.globalFS.Args()
	if a.showHelp || len(remaining) == 0 {
		a.usage()
		return nil
	}

	cmdName := remaining[0]
	cmdArgs := remaining[1:]
	if strings.HasPrefix(cmdName, "-") {
		a.usage()
		a.exitCode = ExitUsageError
		return fmt.Errorf("unknown flag: %s", cmdName)
	}

	cmd, ok := a.commands[cmdName]
	if !ok {
		fmt.Fprintf(a.stderr, "Error: unknown command %q\n", cmdName)
		fmt.Fprintln(a.stderr, "Run 'ttsbridge --help' for usage.")
		a.exitCode = ExitUsageError
		return fmt.Errorf("unknown command: %s", cmdName)
	}

	// Run the subcommand with remaining args
	err := cmd.Run(cmdArgs, a.stdout, a.stderr)
	if err != nil {
		a.handleError(err)
	}

	return err
}

func (a *App) handleError(err error) {
	var usageErr *UsageError
	var runtimeErr *RuntimeError

	if errors.As(err, &usageErr) {
		if usageErr.Message != "" && usageErr.Err != nil {
			if usageErr.Message == usageErr.Err.Error() {
				fmt.Fprintf(a.stderr, "Error: %s\n", usageErr.Message)
				a.exitCode = ExitUsageError
				return
			}
			fmt.Fprintf(a.stderr, "Error: %s: %v\n", usageErr.Message, usageErr.Err)
		} else if usageErr.Message != "" {
			fmt.Fprintf(a.stderr, "Error: %s\n", usageErr.Message)
		} else if usageErr.Err != nil {
			fmt.Fprintf(a.stderr, "Error: %v\n", usageErr.Err)
		}
		a.exitCode = ExitUsageError
		return
	}

	if errors.As(err, &runtimeErr) {
		if runtimeErr.Message != "" && runtimeErr.Err != nil {
			fmt.Fprintf(a.stderr, "Error: %s: %v\n", runtimeErr.Message, runtimeErr.Err)
		} else if runtimeErr.Message != "" {
			fmt.Fprintf(a.stderr, "Error: %s\n", runtimeErr.Message)
		} else if runtimeErr.Err != nil {
			fmt.Fprintf(a.stderr, "Error: %v\n", runtimeErr.Err)
		}
		a.exitCode = ExitRuntimeError
		return
	}

	// Generic error
	fmt.Fprintf(a.stderr, "Error: %v\n", err)
	a.exitCode = ExitRuntimeError
}

func (a *App) printVersion() {
	fmt.Fprintf(a.stdout, "ttsbridge %s\n", a.version)
	if a.commit != "none" && a.commit != "" {
		fmt.Fprintf(a.stdout, "  commit: %s\n", a.commit)
	}
	if a.date != "unknown" && a.date != "" {
		fmt.Fprintf(a.stdout, "  built:  %s\n", a.date)
	}
}

// ExitCode returns the exit code after Run completes
func (a *App) ExitCode() int {
	return a.exitCode
}
