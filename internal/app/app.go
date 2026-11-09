package app

import (
	"fmt"

	"github.com/ansidev/skill-manager-cli/internal/config"

	"github.com/urfave/cli/v2"
)

// New creates a new CLI application with the given version and commit info.
func New(versionStr, commitStr string) *cli.App {
	return &cli.App{
		Name:    "skill-manager",
		Usage:   "Manage agent skills for coding assistants",
		Version: fmt.Sprintf("%s (commit %s)", versionStr, commitStr),
		Commands: []*cli.Command{
			{Name: "init", Action: func(c *cli.Context) error {
				if err := config.WriteExample("config.yaml"); err != nil {
					return err
				}
				fmt.Println("created config.yaml")
				return nil
			}},
			{Name: "apply"},
			{Name: "install"},
			{Name: "uninstall"},
		},
		Action: cli.ShowAppHelp,
	}
}
