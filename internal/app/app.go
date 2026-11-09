package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/config"
	"github.com/ansidev/skill-manager-cli/internal/skilldirs"
	"github.com/ansidev/skill-manager-cli/internal/source"
	"github.com/ansidev/skill-manager-cli/internal/store"
	"github.com/ansidev/skill-manager-cli/internal/targetdirs"

	"github.com/urfave/cli/v2"
)

const defaultConfigPath = "config.yaml"

// New creates a new CLI application with the given version and commit info.
func New(versionStr, commitStr string) *cli.App {
	return &cli.App{
		Name:    "skill-manager",
		Usage:   "Manage agent skills for coding assistants",
		Version: fmt.Sprintf("%s (commit %s)", versionStr, commitStr),
		Commands: []*cli.Command{
			{Name: "init", Action: func(c *cli.Context) error {
				if err := config.WriteExample(defaultConfigPath); err != nil {
					return err
				}
				fmt.Fprintln(commandWriter(c), "created config.yaml")
				return nil
			}},
			applyCommand(),
			{
				Name:   "install",
				Usage:  "Install selected skills from one repository",
				Flags:  directCommandFlags(),
				Action: install,
			},
			{
				Name:   "uninstall",
				Usage:  "Remove selected managed skills",
				Flags:  directCommandFlags(),
				Action: uninstall,
			},
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
	if !c.Bool("yes") && !confirmApply(c.App.Reader, c.App.Writer) {
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

func confirmApply(reader io.Reader, writer io.Writer) bool {
	fmt.Fprint(writer, "Apply configured skill changes? [y/N] ")
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

type commandOptions struct {
	repo            string
	providerType    string
	providerBaseURL string
	skill           string
	skillBasePath   string
	skillDirs       string
	agents          string
	assumeYes       bool
	debug           bool
	dryRun          bool
}

func directCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "repo", Usage: "Repository path or provider repository name"},
		&cli.StringFlag{Name: "provider-type", Usage: "Provider type: github, local, or custom"},
		&cli.StringFlag{Name: "provider-base-url", Usage: "Base URL for a custom provider"},
		&cli.StringFlag{Name: "skill", Usage: "Skill name to manage"},
		&cli.StringFlag{Name: "skill-base-path", Usage: "Path to skills in the repository"},
		&cli.StringFlag{Name: "skill-dirs", Usage: "Comma-separated target skill directories"},
		&cli.StringFlag{Name: "agents", Aliases: []string{"a"}, Usage: "Comma-separated configured agent names"},
		&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Skip confirmation"},
		&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}, Usage: "Emit debug logs"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Report changes without modifying files"},
	}
}

func optionsFromContext(c *cli.Context) commandOptions {
	return commandOptions{
		repo:            c.String("repo"),
		providerType:    c.String("provider-type"),
		providerBaseURL: c.String("provider-base-url"),
		skill:           c.String("skill"),
		skillBasePath:   c.String("skill-base-path"),
		skillDirs:       c.String("skill-dirs"),
		agents:          c.String("agents"),
		assumeYes:       c.Bool("yes"),
		debug:           c.Bool("debug"),
		dryRun:          c.Bool("dry-run"),
	}
}

func install(c *cli.Context) error {
	opts := optionsFromContext(c)
	logger := commandLogger(c, opts.debug)
	logger.Info("starting install")
	cfg, err := loadOptionalConfig()
	if err != nil {
		return err
	}
	repository, entry, err := resolveSelection(opts, cfg, true)
	if err != nil {
		return err
	}
	targets, err := skilldirs.Resolve(opts.skillDirs, opts.agents, cfg)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("at least one target skill directory is required (use --skill-dirs or configure dirs)")
	}

	workspace, err := source.Obtain(repository.Repo, repository.EffectiveProvider())
	if err != nil {
		return err
	}
	defer workspace.Cleanup()
	resolution := source.ResolveSkills(workspace.Path, []config.SkillEntry{entry})
	if len(resolution.Failures) > 0 {
		return fmt.Errorf("resolve skill %q: %w", resolution.Failures[0].Entry.Name, resolution.Failures[0].Err)
	}
	if len(resolution.Skills) == 0 {
		return fmt.Errorf("no skills selected for %q", repository.Repo)
	}
	if err := confirm(c, opts, fmt.Sprintf("install %d skill(s)", len(resolution.Skills))); err != nil {
		return err
	}

	for _, resolved := range resolution.Skills {
		storePath, err := storePathFor(resolved.Entry.Name)
		if err != nil {
			return err
		}
		logger.Debug("evaluating skill copy", "skill", resolved.Entry.Name, "source", resolved.Path, "store", storePath)
		if opts.dryRun {
			logger.Info("dry-run: would copy skill", "skill", resolved.Entry.Name, "store", storePath)
		} else if changed, err := store.StoreSkill(resolved.Path, storePath); err != nil {
			return fmt.Errorf("store skill %q: %w", resolved.Entry.Name, err)
		} else {
			logger.Debug("copy decision complete", "skill", resolved.Entry.Name, "changed", changed)
		}

		for _, target := range targets {
			linkPath := filepath.Join(target, resolved.Entry.Name)
			logger.Debug("evaluating skill link", "skill", resolved.Entry.Name, "link", linkPath)
			if opts.dryRun {
				logger.Info("dry-run: would link skill", "skill", resolved.Entry.Name, "target", linkPath)
				continue
			}
			changed, err := store.LinkSkill(storePath, linkPath)
			if err != nil {
				return fmt.Errorf("link skill %q into %q: %w", resolved.Entry.Name, target, err)
			}
			logger.Debug("link decision complete", "skill", resolved.Entry.Name, "changed", changed)
		}
	}
	logger.Info("finished install")
	return nil
}

func uninstall(c *cli.Context) error {
	opts := optionsFromContext(c)
	logger := commandLogger(c, opts.debug)
	logger.Info("starting uninstall")
	cfg, err := loadOptionalConfig()
	if err != nil {
		return err
	}
	_, entry, err := resolveSelection(opts, cfg, false)
	if err != nil {
		return err
	}
	targets, err := skilldirs.Resolve(opts.skillDirs, opts.agents, cfg)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("at least one target skill directory is required (use --skill-dirs or configure dirs)")
	}
	storePath, err := storePathFor(entry.Name)
	if err != nil {
		return err
	}
	if err := confirm(c, opts, fmt.Sprintf("uninstall skill %q", entry.Name)); err != nil {
		return err
	}

	managedLinksRemoved := false
	for _, target := range targets {
		linkPath := filepath.Join(target, entry.Name)
		logger.Debug("evaluating skill unlink", "skill", entry.Name, "link", linkPath)
		if opts.dryRun {
			logger.Info("dry-run: would unlink skill", "skill", entry.Name, "target", linkPath)
			continue
		}
		changed, err := store.UnlinkManagedSkill(storePath, linkPath)
		if err != nil {
			return fmt.Errorf("unlink skill %q from %q: %w", entry.Name, target, err)
		}
		if changed {
			managedLinksRemoved = true
		}
		logger.Debug("unlink decision complete", "skill", entry.Name, "changed", changed)
	}
	if opts.dryRun {
		logger.Info("dry-run: would remove stored skill", "skill", entry.Name, "store", storePath)
		return nil
	}
	if managedLinksRemoved {
		if err := store.RemoveSkill(storePath); err != nil {
			return fmt.Errorf("remove stored skill %q: %w", entry.Name, err)
		}
	}
	logger.Info("finished uninstall")
	return nil
}

func loadOptionalConfig() (*config.Config, error) {
	if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
		return &config.Config{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", defaultConfigPath, err)
	}
	return config.Load(defaultConfigPath)
}

func resolveSelection(opts commandOptions, cfg *config.Config, requireRepository bool) (config.Repository, config.SkillEntry, error) {
	var matched *config.Repository
	for i := range cfg.Skills {
		if opts.repo == "" || cfg.Skills[i].Repo == opts.repo {
			if matched != nil {
				return config.Repository{}, config.SkillEntry{}, errors.New("repository is ambiguous; provide --repo")
			}
			matched = &cfg.Skills[i]
		}
	}

	repository := config.Repository{Repo: opts.repo}
	if matched != nil {
		repository = *matched
	}
	if requireRepository && strings.TrimSpace(repository.Repo) == "" {
		return config.Repository{}, config.SkillEntry{}, errors.New("repository is required (use --repo or configure exactly one repository)")
	}
	provider := repository.EffectiveProvider()
	if opts.providerType != "" {
		provider.Type = opts.providerType
		if opts.providerBaseURL == "" {
			provider.BaseURL = ""
		}
	}
	if opts.providerBaseURL != "" {
		provider.BaseURL = opts.providerBaseURL
	}
	provider.Normalize()
	repository.Provider = &provider

	entry := config.SkillEntry{Name: opts.skill, BasePath: opts.skillBasePath}
	if entry.Name == "" && matched != nil {
		if len(matched.Skills) != 1 {
			return config.Repository{}, config.SkillEntry{}, errors.New("skill is required (use --skill or configure exactly one skill)")
		}
		entry = matched.Skills[0]
		if opts.skillBasePath != "" {
			entry.BasePath = opts.skillBasePath
		}
	}
	if strings.TrimSpace(entry.Name) == "" {
		return config.Repository{}, config.SkillEntry{}, errors.New("skill is required (use --skill or configure exactly one skill)")
	}
	if err := validateDirectSelection(repository, entry); err != nil {
		return config.Repository{}, config.SkillEntry{}, err
	}
	return repository, entry, nil
}

func validateDirectSelection(repository config.Repository, entry config.SkillEntry) error {
	if filepath.IsAbs(entry.Name) || filepath.Clean(entry.Name) != entry.Name || entry.Name == "." || entry.Name == ".." || strings.Contains(entry.Name, string(filepath.Separator)) {
		return fmt.Errorf("skill name %q must be a single directory name", entry.Name)
	}
	if entry.State == "" {
		entry.State = "installed"
	}
	return config.Validate(&config.Config{Skills: []config.Repository{{Repo: repository.Repo, Provider: repository.Provider, Skills: []config.SkillEntry{entry}}}})
}

func storePathFor(skill string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".agents", "skills", skill), nil
}

func confirm(c *cli.Context, opts commandOptions, action string) error {
	if opts.dryRun || opts.assumeYes {
		return nil
	}
	writer := commandWriter(c)
	fmt.Fprintf(writer, "Proceed to %s? [y/N]: ", action)
	reader := c.App.Reader
	if reader == nil {
		reader = os.Stdin
	}
	answer, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
		return nil
	}
	return errors.New("operation cancelled")
}

func commandLogger(c *cli.Context, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(commandWriter(c), &slog.HandlerOptions{Level: level}))
}

func commandWriter(c *cli.Context) io.Writer {
	if c.App.Writer != nil {
		return c.App.Writer
	}
	return os.Stdout
}
