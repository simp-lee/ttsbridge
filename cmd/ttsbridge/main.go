// Package main implements the ttsbridge CLI.
package main

import (
	"os"

	"github.com/simp-lee/ttsbridge/internal/cli"
)

// Version information, set by linker flags during build
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	app := cli.NewApp(version, commit, date)
	if err := app.Run(os.Args[1:]); err != nil {
		os.Exit(app.ExitCode())
	}
}
