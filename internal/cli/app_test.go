package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type testCommand struct {
	name string
	run  func(args []string, stdout, stderr io.Writer) error
}

func (c testCommand) Name() string { return c.name }

func (c testCommand) Run(args []string, stdout, stderr io.Writer) error {
	if c.run != nil {
		return c.run(args, stdout, stderr)
	}
	return nil
}

func newTestApp(stdout, stderr io.Writer) *App {
	return &App{
		version:  "1.2.3",
		commit:   "abc123",
		date:     "2026-03-25",
		stdout:   stdout,
		stderr:   stderr,
		commands: make(map[string]Command),
	}
}

func TestAppRegisterCommandPanicsOnDuplicateName(t *testing.T) {
	app := &App{commands: make(map[string]Command)}
	app.registerCommand(testCommand{name: "voices"})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("registerCommand() expected panic on duplicate command name")
		}
		if !strings.Contains(fmt.Sprint(recovered), "already registered") {
			t.Fatalf("panic = %v, want duplicate registration message", recovered)
		}
	}()

	app.registerCommand(testCommand{name: "voices"})
}

func TestAppRun_DispatchAndExitCodes(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		register        func(app *App)
		wantErr         bool
		wantExitCode    int
		wantStdoutParts []string
		wantStderrParts []string
	}{
		{
			name:         "help",
			args:         []string{"--help"},
			wantExitCode: ExitSuccess,
			wantStderrParts: []string{
				"Usage:",
				"Available Commands:",
			},
		},
		{
			name:         "version",
			args:         []string{"--version"},
			wantExitCode: ExitSuccess,
			wantStdoutParts: []string{
				"ttsbridge 1.2.3",
				"commit: abc123",
				"built:  2026-03-25",
			},
		},
		{
			name:         "unknown command",
			args:         []string{"missing"},
			wantErr:      true,
			wantExitCode: ExitUsageError,
			wantStderrParts: []string{
				"unknown command \"missing\"",
				"Run 'ttsbridge --help' for usage.",
			},
		},
		{
			name: "dispatch success",
			args: []string{"ok", "--flag", "value"},
			register: func(app *App) {
				app.registerCommand(testCommand{
					name: "ok",
					run: func(args []string, stdout, stderr io.Writer) error {
						if got, want := strings.Join(args, " "), "--flag value"; got != want {
							return fmt.Errorf("args = %q, want %q", got, want)
						}
						_, _ = io.WriteString(stdout, "command ran")
						return nil
					},
				})
			},
			wantExitCode: ExitSuccess,
			wantStdoutParts: []string{
				"command ran",
			},
		},
		{
			name: "usage error mapping",
			args: []string{"bad"},
			register: func(app *App) {
				sentinel := errors.New("bad flag value")
				app.registerCommand(testCommand{
					name: "bad",
					run: func(args []string, stdout, stderr io.Writer) error {
						return &UsageError{Message: "invalid input", Err: sentinel}
					},
				})
			},
			wantErr:      true,
			wantExitCode: ExitUsageError,
			wantStderrParts: []string{
				"Error: invalid input: bad flag value",
			},
		},
		{
			name: "runtime error mapping",
			args: []string{"explode"},
			register: func(app *App) {
				sentinel := errors.New("network down")
				app.registerCommand(testCommand{
					name: "explode",
					run: func(args []string, stdout, stderr io.Writer) error {
						return &RuntimeError{Message: "synthesis failed", Err: sentinel}
					},
				})
			},
			wantErr:      true,
			wantExitCode: ExitRuntimeError,
			wantStderrParts: []string{
				"Error: synthesis failed: network down",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			app := newTestApp(&stdout, &stderr)
			if tt.register != nil {
				tt.register(app)
			}

			err := app.Run(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if app.ExitCode() != tt.wantExitCode {
				t.Fatalf("ExitCode() = %d, want %d", app.ExitCode(), tt.wantExitCode)
			}
			for _, part := range tt.wantStdoutParts {
				if !strings.Contains(stdout.String(), part) {
					t.Fatalf("stdout = %q, want substring %q", stdout.String(), part)
				}
			}
			for _, part := range tt.wantStderrParts {
				if !strings.Contains(stderr.String(), part) {
					t.Fatalf("stderr = %q, want substring %q", stderr.String(), part)
				}
			}
		})
	}
}
