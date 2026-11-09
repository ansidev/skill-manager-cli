package app

import (
	"fmt"

	"github.com/ansidev/skill-manager-cli/internal/version"
	"github.com/urfave/cli/v2"
)

// New creates a new CLI application with the given version and commit info.
func New(versionStr, commitStr string) *cli.App {
	info := version.Info{
		Version: versionStr,
		Commit:  commitStr,
	}

	return &cli.App{
		Name:  "skill-manager",
		Usage: "Manage agent skills for coding assistants",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "Show version and commit information",
			},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("version") {
				fmt.Println(info.String())
				return nil
			}
			return cli.ShowAppHelp(c)
		},
	}
}
