package main

import (
	"fmt"
	"os"

	"github.com/ansidev/skill-manager-cli/internal/app"
)

// These variables are set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	application := app.New(version, commit)
	if err := application.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
