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
	"sync"

	"github.com/ansidev/skill-manager-cli/internal/config"
	"github.com/ansidev/skill-manager-cli/internal/source"
	"github.com/ansidev/skill-manager-cli/internal/store"
	"github.com/ansidev/skill-manager-cli/internal/targetdirs"
	"github.com/urfave/cli/v2"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

const defaultConfigPath = "config.yaml"

// New creates a new CLI application with the given version and commit info.
func New(versionStr, commitStr string) *cli.App {
	return &cli.App{
		Name:    "skill-manager-cli",
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

const (
	actionInstalled   = "installed"
	actionUninstalled = "uninstalled"
	actionSkipped     = "skipped"
	actionFailed      = "failed"
)

type operationOutcome struct {
	Operation  string
	Skill      string
	Action     string
	Repository string
	Source     string
	Path       string
	Targets    []string
	DryRun     bool
	Err        error
}

type applyRepository struct {
	repository       config.Repository
	provider         config.Provider
	sourceLocation   string
	namedUninstalls  []config.SkillEntry
	entriesToResolve []config.SkillEntry
	workspace        *source.Workspace
	obtainErr        error
	progressBar      *mpb.Bar
}

type taskProgress struct {
	bars     *mpb.Progress
	waitOnce sync.Once
}

func newTaskProgress(writer io.Writer) *taskProgress {
	return &taskProgress{bars: mpb.New(mpb.WithOutput(writer))}
}

func (p *taskProgress) add(label string, after *mpb.Bar) *mpb.Bar {
	return p.addTask(label, after, false)
}

func (p *taskProgress) addSpinner(label string, after *mpb.Bar) *mpb.Bar {
	return p.addTask(label, after, true)
}

func (p *taskProgress) addTask(label string, after *mpb.Bar, spinner bool) *mpb.Bar {
	if p == nil || p.bars == nil {
		return nil
	}
	options := []mpb.BarOption{
		mpb.PrependDecorators(decor.Name(label)),
		mpb.AppendDecorators(decor.OnComplete(decor.Percentage(), "done")),
	}
	if after != nil {
		options = append(options, mpb.BarQueueAfter(after), mpb.BarFillerClearOnComplete())
	}
	if spinner {
		return p.bars.AddSpinner(1, options...)
	}
	return p.bars.AddBar(1, options...)
}

func (p *taskProgress) wait() {
	if p != nil && p.bars != nil {
		p.waitOnce.Do(p.bars.Wait)
	}
}

func completeTask(bar *mpb.Bar) {
	if bar != nil {
		bar.Increment()
	}
}

func abortTasks(bars []*mpb.Bar) {
	for _, bar := range bars {
		if bar != nil {
			bar.Abort(true)
		}
	}
}

func symlinkTaskBars(progress *taskProgress, targets []string, skillName string, after *mpb.Bar) []*mpb.Bar {
	bars := make([]*mpb.Bar, 0, len(targets))
	for _, target := range targets {
		after = progress.add("Create symlink: "+filepath.Join(target, skillName), after)
		bars = append(bars, after)
	}
	return bars
}

func lastTaskBar(bars []*mpb.Bar, fallback *mpb.Bar) *mpb.Bar {
	if len(bars) == 0 {
		return fallback
	}
	return bars[len(bars)-1]
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
	logger := commandLogger(c, c.Bool("debug"))
	logger.Info("starting apply")
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("load config.yaml: %w", err)
	}
	targets, err := targetdirs.Resolve(c.String("skill-dirs"), c.String("agents"), cfg)
	if err != nil {
		return err
	}
	printApplyPlan(c.App.Writer, cfg)
	if !c.Bool("yes") && !confirmApply(c.App.Reader, c.App.Writer) {
		fmt.Fprintln(c.App.Writer, "apply cancelled")
		return nil
	}

	storeRoot, err := storeRoot()
	if err != nil {
		return err
	}
	outcomes := make([]operationOutcome, 0)
	progress := newTaskProgress(commandWriter(c))
	defer progress.wait()
	resolver := source.NewResolver()
	repositories := make([]applyRepository, len(cfg.Skills))
	for i, repository := range cfg.Skills {
		provider := repository.EffectiveProvider()
		prepared := applyRepository{
			repository:     repository,
			provider:       provider,
			sourceLocation: repositorySource(repository.Repo, provider),
		}
		for _, entry := range repository.Skills {
			// A named uninstall does not need a repository workspace.
			if entry.State == "uninstalled" && entry.Name != "*" {
				prepared.namedUninstalls = append(prepared.namedUninstalls, entry)
			} else {
				prepared.entriesToResolve = append(prepared.entriesToResolve, entry)
			}
		}
		repositories[i] = prepared
	}
	for i := range repositories {
		for _, entry := range repositories[i].namedUninstalls {
			outcome, _ := applySkill(logger, c, storeRoot, targets, repositories[i].repository.Repo, repositories[i].sourceLocation, config.JoinBase(entry.BasePath, entry.Name), source.ResolvedSkill{Entry: entry}, progress, nil)
			outcomes = append(outcomes, outcome)
		}
	}

	obtainRepositories(resolver, repositories, progress)

	for _, prepared := range repositories {
		repository := prepared.repository
		if len(prepared.entriesToResolve) == 0 {
			continue
		}
		if prepared.obtainErr != nil {
			outcomes = append(outcomes, operationOutcome{
				Operation:  "apply",
				Action:     actionFailed,
				Repository: repository.Repo,
				Source:     prepared.sourceLocation,
				Targets:    targets,
				DryRun:     c.Bool("dry-run"),
				Err:        prepared.obtainErr,
			})
			continue
		}
		resolution := source.ResolveSkills(prepared.workspace.Path, prepared.entriesToResolve)
		for _, failure := range resolution.Failures {
			outcomes = append(outcomes, operationOutcome{
				Operation:  "apply",
				Skill:      failure.Entry.Name,
				Action:     actionFailed,
				Repository: repository.Repo,
				Source:     prepared.sourceLocation,
				Path:       config.JoinBase(failure.Entry.BasePath, failure.Entry.Name),
				Targets:    targets,
				DryRun:     c.Bool("dry-run"),
				Err:        failure.Err,
			})
		}
		for _, skill := range resolution.Skills {
			outcome, lastBar := applySkill(logger, c, storeRoot, targets, repository.Repo, prepared.sourceLocation, relativeRepositoryPath(prepared.workspace.Path, skill.Path), skill, progress, prepared.progressBar)
			prepared.progressBar = lastBar
			outcomes = append(outcomes, outcome)
		}
		if err := prepared.workspace.Cleanup(); err != nil {
			outcomes = append(outcomes, operationOutcome{
				Operation:  "apply",
				Action:     actionFailed,
				Repository: repository.Repo,
				Source:     prepared.sourceLocation,
				Targets:    targets,
				DryRun:     c.Bool("dry-run"),
				Err:        fmt.Errorf("clean workspace: %w", err),
			})
		}
	}

	progress.wait()
	failed := logSummary(commandWriter(c), "apply", outcomes)
	logger.Info("finished apply", "operations", len(outcomes), "failed", failed)
	if failed > 0 {
		return fmt.Errorf("apply completed with %d failed operation(s)", failed)
	}
	return nil
}

func obtainRepositories(resolver *source.Resolver, repositories []applyRepository, progress *taskProgress) {
	remote := make([]int, 0)
	for i := range repositories {
		if len(repositories[i].entriesToResolve) == 0 {
			continue
		}
		if repositories[i].provider.Type == "local" {
			repositories[i].workspace, repositories[i].obtainErr = resolver.Obtain(repositories[i].repository.Repo, repositories[i].provider)
		} else {
			remote = append(remote, i)
		}
	}
	if len(remote) == 0 {
		return
	}

	const maxConcurrentClones = 4
	workerCount := min(len(remote), maxConcurrentClones)
	jobs := make(chan int, len(remote))
	results := make(chan struct {
		index     int
		workspace *source.Workspace
		err       error
	}, len(remote))
	for _, index := range remote {
		repositories[index].progressBar = progress.addSpinner("Clone repo: "+repositories[index].repository.Repo, nil)
		jobs <- index
	}
	close(jobs)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				repository := repositories[index]
				workspace, err := resolver.Obtain(repository.repository.Repo, repository.provider)
				completeTask(repository.progressBar)
				results <- struct {
					index     int
					workspace *source.Workspace
					err       error
				}{index: index, workspace: workspace, err: err}
			}
		}()
	}
	for range len(remote) {
		result := <-results
		repositories[result.index].workspace = result.workspace
		repositories[result.index].obtainErr = result.err
	}
	workers.Wait()
}

func applySkill(logger *slog.Logger, c *cli.Context, storeRoot string, targets []string, repository, sourceLocation, inRepoPath string, skill source.ResolvedSkill, progress *taskProgress, after *mpb.Bar) (operationOutcome, *mpb.Bar) {
	outcome := operationOutcome{
		Operation:  "apply",
		Skill:      skill.Entry.Name,
		Repository: repository,
		Source:     sourceLocation,
		Path:       inRepoPath,
		Targets:    targets,
		DryRun:     c.Bool("dry-run"),
	}
	if !validSkillName(skill.Entry.Name) {
		outcome.Action = actionFailed
		outcome.Err = fmt.Errorf("invalid skill name %q", skill.Entry.Name)
		return outcome, after
	}
	storePath := filepath.Join(storeRoot, skill.Entry.Name)
	for _, target := range targets {
		targetPath := filepath.Join(target, skill.Entry.Name)
		if err := validateScopedPath(storeRoot, storePath, target, targetPath); err != nil {
			outcome.Action = actionFailed
			outcome.Err = err
			return outcome, after
		}
	}
	logger.Debug("evaluating configured skill", "skill", skill.Entry.Name, "repository", repository, "path", inRepoPath)
	if outcome.DryRun {
		outcome.Action = actionForState(skill.Entry.State)
		logger.Info("dry-run: planned skill operation", "skill", skill.Entry.Name, "action", outcome.Action, "targets", strings.Join(targets, ","))
		return outcome, after
	}
	if skill.Entry.State == "uninstalled" {
		outcome.Action, outcome.Err = removeSkill(logger, storePath, skill.Entry.Name, targets)
		return outcome, after
	}
	installBar := progress.add("Install skill: "+skill.Entry.Name, after)
	symlinkBars := symlinkTaskBars(progress, targets, skill.Entry.Name, installBar)
	lastBar := lastTaskBar(symlinkBars, installBar)
	storeChanged, err := store.StoreSkill(skill.Path, storePath)
	completeTask(installBar)
	if err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, lastBar
	}
	linkChanged, err := linkTargets(logger, storePath, skill.Entry.Name, targets, symlinkBars)
	if err != nil {
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, lastBar
	}
	if storeChanged || linkChanged {
		outcome.Action = actionInstalled
	} else {
		outcome.Action = actionSkipped
	}
	return outcome, lastBar
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

func printApplyPlan(writer io.Writer, cfg *config.Config) {
	type planEntry struct {
		repo   string
		skills []string
	}
	install := make([]planEntry, 0)
	uninstall := make([]planEntry, 0)
	for _, repository := range cfg.Skills {
		var installSkills, uninstallSkills []string
		for _, entry := range repository.Skills {
			if entry.State == "uninstalled" {
				uninstallSkills = append(uninstallSkills, entry.Name)
			} else {
				installSkills = append(installSkills, entry.Name)
			}
		}
		if len(installSkills) > 0 {
			install = append(install, planEntry{repo: repository.Repo, skills: installSkills})
		}
		if len(uninstallSkills) > 0 {
			uninstall = append(uninstall, planEntry{repo: repository.Repo, skills: uninstallSkills})
		}
	}
	if len(install) == 0 && len(uninstall) == 0 {
		return
	}
	if len(install) > 0 {
		fmt.Fprintln(writer, "Skills to install")
		for _, entry := range install {
			fmt.Fprintf(writer, "  - Repository: %s\n", entry.repo)
			fmt.Fprintln(writer, "    - Skills:")
			for _, skill := range entry.skills {
				fmt.Fprintf(writer, "      - %s\n", skill)
			}
		}
	}
	if len(uninstall) > 0 {
		if len(install) > 0 {
			fmt.Fprintln(writer)
		}
		fmt.Fprintln(writer, "Skills to uninstall")
		for _, entry := range uninstall {
			fmt.Fprintf(writer, "  - Repository: %s\n", entry.repo)
			fmt.Fprintln(writer, "    - Skills:")
			for _, skill := range entry.skills {
				fmt.Fprintf(writer, "      - %s\n", skill)
			}
		}
	}
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
	targets, err := targetdirs.Resolve(opts.skillDirs, opts.agents, cfg)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("at least one target skill directory is required (use --skill-dirs or configure dirs)")
	}

	storeRoot, err := storeRoot()
	if err != nil {
		return err
	}
	provider := repository.EffectiveProvider()
	sourceLocation := repositorySource(repository.Repo, provider)
	basePath := config.JoinBase(entry.BasePath, entry.Name)
	progress := newTaskProgress(commandWriter(c))
	defer progress.wait()
	var lastBar *mpb.Bar
	if provider.Type != "local" {
		lastBar = progress.addSpinner("Clone repo: "+repository.Repo, nil)
	}
	workspace, err := source.Obtain(repository.Repo, provider)
	completeTask(lastBar)
	if err != nil {
		progress.wait()
		logSummary(commandWriter(c), "install", []operationOutcome{{
			Operation:  "install",
			Action:     actionFailed,
			Repository: repository.Repo,
			Source:     sourceLocation,
			Path:       basePath,
			Targets:    targets,
			DryRun:     opts.dryRun,
			Err:        err,
		}})
		return err
	}
	defer workspace.Cleanup()
	resolution := source.ResolveSkills(workspace.Path, []config.SkillEntry{entry})
	if len(resolution.Skills) == 0 && len(resolution.Failures) == 0 {
		return fmt.Errorf("no skills selected for %q", repository.Repo)
	}
	if err := confirm(c, opts, fmt.Sprintf("install %d skill(s)", len(resolution.Skills))); err != nil {
		return err
	}

	outcomes := make([]operationOutcome, 0, len(resolution.Skills)+len(resolution.Failures))
	for _, failure := range resolution.Failures {
		outcomes = append(outcomes, operationOutcome{
			Operation:  "install",
			Skill:      failure.Entry.Name,
			Action:     actionFailed,
			Repository: repository.Repo,
			Source:     sourceLocation,
			Path:       config.JoinBase(failure.Entry.BasePath, failure.Entry.Name),
			Targets:    targets,
			DryRun:     opts.dryRun,
			Err:        failure.Err,
		})
	}
	for _, resolved := range resolution.Skills {
		outcome, nextBar := installSkill(logger, opts, storeRoot, targets, repository.Repo, sourceLocation, relativeRepositoryPath(workspace.Path, resolved.Path), resolved, progress, lastBar)
		lastBar = nextBar
		outcomes = append(outcomes, outcome)
	}
	progress.wait()
	failed := logSummary(commandWriter(c), "install", outcomes)
	if failed > 0 {
		return fmt.Errorf("install completed with %d failed operation(s)", failed)
	}
	logger.Info("finished install")
	return nil
}

func installSkill(logger *slog.Logger, opts commandOptions, storeRoot string, targets []string, repository, sourceLocation, inRepoPath string, resolved source.ResolvedSkill, progress *taskProgress, after *mpb.Bar) (operationOutcome, *mpb.Bar) {
	outcome := operationOutcome{
		Operation:  "install",
		Skill:      resolved.Entry.Name,
		Repository: repository,
		Source:     sourceLocation,
		Path:       inRepoPath,
		Targets:    targets,
		DryRun:     opts.dryRun,
	}
	storePath := filepath.Join(storeRoot, resolved.Entry.Name)
	logger.Debug("evaluating skill copy", "skill", resolved.Entry.Name, "source", resolved.Path, "store", storePath)
	if opts.dryRun {
		outcome.Action = actionInstalled
		logger.Info("dry-run: would copy skill", "skill", resolved.Entry.Name, "store", storePath)
		for _, target := range targets {
			logger.Info("dry-run: would link skill", "skill", resolved.Entry.Name, "target", filepath.Join(target, resolved.Entry.Name))
		}
		return outcome, after
	}
	installBar := progress.add("Install skill: "+resolved.Entry.Name, after)
	symlinkBars := symlinkTaskBars(progress, targets, resolved.Entry.Name, installBar)
	lastBar := lastTaskBar(symlinkBars, installBar)
	storeChanged, err := store.StoreSkill(resolved.Path, storePath)
	completeTask(installBar)
	if err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = fmt.Errorf("store skill %q: %w", resolved.Entry.Name, err)
		return outcome, lastBar
	}
	logger.Debug("copy decision complete", "skill", resolved.Entry.Name, "changed", storeChanged)
	linkChanged, err := linkTargets(logger, storePath, resolved.Entry.Name, targets, symlinkBars)
	if err != nil {
		outcome.Action = actionFailed
		outcome.Err = fmt.Errorf("link skill %q: %w", resolved.Entry.Name, err)
		return outcome, lastBar
	}
	if storeChanged || linkChanged {
		outcome.Action = actionInstalled
	} else {
		outcome.Action = actionSkipped
	}
	return outcome, lastBar
}

func uninstall(c *cli.Context) error {
	opts := optionsFromContext(c)
	logger := commandLogger(c, opts.debug)
	logger.Info("starting uninstall")
	cfg, err := loadOptionalConfig()
	if err != nil {
		return err
	}
	repository, entry, err := resolveSelection(opts, cfg, false)
	if err != nil {
		return err
	}
	targets, err := targetdirs.Resolve(opts.skillDirs, opts.agents, cfg)
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

	sourceLocation := ""
	inRepoPath := ""
	if strings.TrimSpace(repository.Repo) != "" {
		sourceLocation = repositorySource(repository.Repo, repository.EffectiveProvider())
		inRepoPath = config.JoinBase(entry.BasePath, entry.Name)
	}
	outcome := operationOutcome{
		Operation:  "uninstall",
		Skill:      entry.Name,
		Repository: repository.Repo,
		Source:     sourceLocation,
		Path:       inRepoPath,
		Targets:    targets,
		DryRun:     opts.dryRun,
	}
	if opts.dryRun {
		logger.Info("dry-run: would unlink skill", "skill", entry.Name, "targets", strings.Join(targets, ","))
		logger.Info("dry-run: would remove stored skill", "skill", entry.Name, "store", storePath)
		outcome.Action = actionUninstalled
		logSummary(commandWriter(c), "uninstall", []operationOutcome{outcome})
		return nil
	}

	outcome.Action, outcome.Err = removeSkill(logger, storePath, entry.Name, targets)
	failed := logSummary(commandWriter(c), "uninstall", []operationOutcome{outcome})
	if failed > 0 {
		return fmt.Errorf("uninstall completed with %d failed operation(s)", failed)
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

func storeRoot() (string, error) {
	if root := os.Getenv("SKILL_MANAGER_STORE_DIR"); root != "" {
		return filepath.Abs(root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

func storePathFor(skill string) (string, error) {
	root, err := storeRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, skill), nil
}

func repositorySource(repo string, provider config.Provider) string {
	if provider.Type == "local" {
		return repo
	}
	return source.ResolveURL(repo, provider)
}

func relativeRepositoryPath(root, path string) string {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		canonicalRoot = root
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		canonicalPath = path
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return ""
	}
	return rel
}

func actionForState(state string) string {
	if state == "uninstalled" {
		return actionUninstalled
	}
	return actionInstalled
}

func linkTargets(logger *slog.Logger, storePath, skillName string, targets []string, bars []*mpb.Bar) (bool, error) {
	changedAny := false
	var firstErr error
	for i, target := range targets {
		linkPath := filepath.Join(target, skillName)
		logger.Debug("evaluating skill link", "skill", skillName, "link", linkPath)
		changed, err := store.LinkSkill(storePath, linkPath)
		completeTask(bars[i])
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", target, err)
			}
		} else {
			changedAny = changedAny || changed
			logger.Debug("link decision complete", "skill", skillName, "changed", changed)
		}
	}
	return changedAny, firstErr
}

func removeSkill(logger *slog.Logger, storePath, skillName string, targets []string) (string, error) {
	cfg, err := loadOptionalConfig()
	if err != nil {
		return actionFailed, err
	}
	configured, err := targetdirs.Resolve("", "", cfg)
	if err != nil {
		return actionFailed, err
	}
	selected := make(map[string]bool, len(targets))
	for _, target := range targets {
		abs, err := filepath.Abs(target)
		if err != nil {
			return actionFailed, err
		}
		selected[abs] = true
	}
	storeAbs, err := filepath.Abs(storePath)
	if err != nil {
		return actionFailed, err
	}
	for _, target := range configured {
		abs, err := filepath.Abs(target)
		if err != nil {
			return actionFailed, err
		}
		if selected[abs] {
			continue
		}
		link := filepath.Join(abs, skillName)
		info, err := os.Lstat(link)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return actionFailed, fmt.Errorf("inspect configured path %q: %w", link, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		linked, err := os.Readlink(link)
		if err != nil {
			return actionFailed, fmt.Errorf("read configured link %q: %w", link, err)
		}
		if !filepath.IsAbs(linked) {
			linked = filepath.Join(abs, linked)
		}
		if filepath.Clean(linked) == storeAbs {
			return actionFailed, fmt.Errorf("configured link %q still references stored skill; include its agent", link)
		}
	}
	removed, err := unlinkTargets(logger, storePath, skillName, targets)
	if err != nil {
		return actionFailed, err
	}
	_, err = os.Lstat(storePath)
	if err != nil && !os.IsNotExist(err) {
		return actionFailed, fmt.Errorf("inspect stored skill %q: %w", storePath, err)
	}
	exists := err == nil
	if err := store.RemoveSkill(storePath); err != nil {
		return actionFailed, fmt.Errorf("remove stored skill %q: %w", storePath, err)
	}
	if removed || exists {
		return actionUninstalled, nil
	}
	return actionSkipped, nil
}

func unlinkTargets(logger *slog.Logger, storePath, skillName string, targets []string) (bool, error) {
	changedAny := false
	var firstErr error
	for _, target := range targets {
		linkPath := filepath.Join(target, skillName)
		logger.Debug("evaluating skill unlink", "skill", skillName, "link", linkPath)
		changed, err := store.UnlinkManagedSkill(storePath, linkPath)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", target, err)
			}
			continue
		}
		changedAny = changedAny || changed
		logger.Debug("unlink decision complete", "skill", skillName, "changed", changed)
	}
	return changedAny, firstErr
}

func logSummary(writer io.Writer, operation string, outcomes []operationOutcome) int {
	failed := 0
	type repositorySummary struct {
		name   string
		skills []string
	}
	var toInstall, toUninstall []repositorySummary
	for _, outcome := range outcomes {
		if outcome.Action == actionFailed {
			failed++
		}
		uninstall := operation == "uninstall" || outcome.Action == actionUninstalled
		groups := &toInstall
		if uninstall {
			groups = &toUninstall
		}
		repository := outcome.Repository
		if repository == "" {
			repository = "local store"
		}
		index := -1
		for i := range *groups {
			if (*groups)[i].name == repository {
				index = i
				break
			}
		}
		if index == -1 {
			*groups = append(*groups, repositorySummary{name: repository})
			index = len(*groups) - 1
		}
		name := outcome.Skill
		if name == "" {
			name = "repository operation"
		}
		detail := outcome.Action
		if outcome.Err != nil {
			detail += ": " + outcome.Err.Error()
		}
		(*groups)[index].skills = append((*groups)[index].skills, fmt.Sprintf("%s (%s)", name, detail))
	}
	printGroup := func(title string, groups []repositorySummary) {
		if len(groups) == 0 {
			return
		}
		fmt.Fprintln(writer, title)
		for _, repository := range groups {
			fmt.Fprintf(writer, "  - Repository: %s\n    - Skills:\n", repository.name)
			for _, skill := range repository.skills {
				fmt.Fprintf(writer, "      - %s\n", skill)
			}
		}
	}
	printGroup("Skills installed", toInstall)
	if len(toInstall) > 0 && len(toUninstall) > 0 {
		fmt.Fprintln(writer)
	}
	printGroup("Skills uninstalled", toUninstall)
	fmt.Fprintf(writer, "Completed %d operation(s); %d failed.\n", len(outcomes), failed)
	return failed
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

func commandLogger(_ *cli.Context, _ bool) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func commandWriter(c *cli.Context) io.Writer {
	if c.App.Writer != nil {
		return c.App.Writer
	}
	return os.Stdout
}
