package app

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

// New creates a new CLI application with the given version and commit info.
func New(versionStr, commitStr string) *cli.App {
	return &cli.App{
		Name:    "skill-manager",
		Usage:   "Manage agent skills for coding assistants",
		Version: fmt.Sprintf("%s (commit %s)", versionStr, commitStr),
		Commands: []*cli.Command{
			{Name: "init"},
			{Name: "apply"},
			{Name: "install"},
			{Name: "uninstall"},
		},
		Action: cli.ShowAppHelp,
	}
}
