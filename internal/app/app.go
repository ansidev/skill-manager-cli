package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/config"
	"github.com/ansidev/skill-manager-cli/internal/source"
	"github.com/ansidev/skill-manager-cli/internal/store"
	"github.com/ansidev/skill-manager-cli/internal/targetdirs"

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
			applyCommand(),
			{Name: "install"},
			{Name: "uninstall"},
		},

		Action: cli.ShowAppHelp,
	}
}

type applyOutcome struct {
	repository string
	skill      string
	err        error
}

func applyCommand() *cli.Command {
	return &cli.Command{
		Name:  "apply",
		Usage: "apply the configured skill state",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "skip confirmation"},
			&cli.BoolFlag{Name: "dry-run", Usage: "show planned operations without changing files"},
			&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}, Usage: "show operation diagnostics"},
			&cli.StringFlag{Name: "skill-dirs", Usage: "comma-separated skill directories"},
			&cli.StringFlag{Name: "agents", Aliases: []string{"a"}, Usage: "comma-separated configured agent names"},
		},
		Action: apply,
	}
}

func apply(c *cli.Context) error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("load config.yaml: %w", err)
	}
	targets, err := targetdirs.Resolve(c.String("skill-dirs"), c.String("agents"), cfg)
	if err != nil {
		return err
	}
	if !c.Bool("yes") && !confirm(c.App.Reader, c.App.Writer) {
		fmt.Fprintln(c.App.Writer, "apply cancelled")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	storeRoot := filepath.Join(home, ".skill-manager", "skills")
	outcomes := make([]applyOutcome, 0)
	resolver := source.NewResolver()
	for _, repository := range cfg.Skills {
		namedUninstalls := make([]config.SkillEntry, 0)
		entriesToResolve := make([]config.SkillEntry, 0, len(repository.Skills))
		for _, entry := range repository.Skills {
			// Uninstalling a named skill does not require discovering it in a
			// source workspace, so keep it outside both acquisition and
			// path-resolution validation handled below.
			if entry.State == "uninstalled" && entry.Name != "*" {
				namedUninstalls = append(namedUninstalls, entry)
				continue
			}
			entriesToResolve = append(entriesToResolve, entry)
		}
		for _, entry := range namedUninstalls {
			outcomes = append(outcomes, applySkill(c, storeRoot, targets, repository.Repo, source.ResolvedSkill{Entry: entry}))
		}
		if len(entriesToResolve) == 0 {
			continue
		}

		workspace, err := resolver.Obtain(repository.Repo, repository.EffectiveProvider())
		if err != nil {
			outcomes = append(outcomes, applyOutcome{repository: repository.Repo, err: err})
			continue
		}
		resolution := source.ResolveSkills(workspace.Path, entriesToResolve)
		for _, failure := range resolution.Failures {
			outcomes = append(outcomes, applyOutcome{repository: repository.Repo, skill: failure.Entry.Name, err: failure.Err})
		}
		for _, skill := range resolution.Skills {
			outcomes = append(outcomes, applySkill(c, storeRoot, targets, repository.Repo, skill))
		}
		if err := workspace.Cleanup(); err != nil {
			outcomes = append(outcomes, applyOutcome{repository: repository.Repo, err: fmt.Errorf("clean workspace: %w", err)})
		}
	}

	failed := 0
	for _, outcome := range outcomes {
		if outcome.err == nil {
			continue
		}
		failed++
		fmt.Fprintf(c.App.Writer, "error: repository %s skill %s: %v\n", outcome.repository, outcome.skill, outcome.err)
	}
	fmt.Fprintf(c.App.Writer, "apply finished: %d operation(s), %d failed\n", len(outcomes), failed)
	if failed > 0 {
		return fmt.Errorf("apply completed with %d failed operation(s)", failed)
	}
	return nil
}

func applySkill(c *cli.Context, storeRoot string, targets []string, repository string, skill source.ResolvedSkill) applyOutcome {
	outcome := applyOutcome{repository: repository, skill: skill.Entry.Name}
	if !validSkillName(skill.Entry.Name) {
		outcome.err = fmt.Errorf("invalid skill name %q", skill.Entry.Name)
		return outcome
	}
	storePath := filepath.Join(storeRoot, skill.Entry.Name)
	for _, target := range targets {
		targetPath := filepath.Join(target, skill.Entry.Name)
		if err := validateScopedPath(storeRoot, storePath, target, targetPath); err != nil {
			outcome.err = err
			return outcome
		}
	}
	if c.Bool("debug") {
		fmt.Fprintf(c.App.Writer, "debug: apply %s from %s\n", skill.Entry.Name, repository)
	}
	if c.Bool("dry-run") {
		fmt.Fprintf(c.App.Writer, "dry-run: %s %s\n", skill.Entry.State, skill.Entry.Name)
		return outcome
	}
	if skill.Entry.State == "uninstalled" {
		var targetErr error
		for _, target := range targets {
			if _, err := store.UnlinkManagedSkill(storePath, filepath.Join(target, skill.Entry.Name)); err != nil && targetErr == nil {
				targetErr = fmt.Errorf("unlink %s: %w", target, err)
			}
		}
		if targetErr == nil {
			outcome.err = store.RemoveSkill(storePath)
		} else {
			outcome.err = targetErr
		}
		return outcome
	}
	if _, err := store.StoreSkill(skill.Path, storePath); err != nil {
		outcome.err = err
		return outcome
	}
	var targetErr error
	for _, target := range targets {
		if _, err := store.LinkSkill(storePath, filepath.Join(target, skill.Entry.Name)); err != nil && targetErr == nil {
			targetErr = fmt.Errorf("link %s: %w", target, err)
		}
	}
	outcome.err = targetErr
	return outcome
}

func validSkillName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "*" {
		return false
	}
	return name == filepath.Clean(name) && !strings.ContainsAny(name, "/\\")
}

func validateScopedPath(storeRoot, storePath, targetRoot, targetPath string) error {
	if rel, err := filepath.Rel(storeRoot, storePath); err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("store path %q escapes store root", storePath)
	}
	if rel, err := filepath.Rel(targetRoot, targetPath); err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("target path %q escapes target root", targetPath)
	}
	return nil
}

func confirm(reader io.Reader, writer io.Writer) bool {
	fmt.Fprint(writer, "Apply configured skill changes? [y/N] ")
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
