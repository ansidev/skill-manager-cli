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
				Flags:  directCommandFlags(true),
				Action: install,
			},
			{
				Name:   "uninstall",
				Usage:  "Remove selected managed skills",
				Flags:  directCommandFlags(false),
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
	Operation    string
	Skill        string
	Action       string
	State        string
	Repository   string
	Source       string
	Path         string
	Targets      []string
	DryRun       bool
	Err          error
	CreatedLinks []string
	DeletedLinks []string
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
	return &taskProgress{bars: mpb.New(mpb.WithOutput(writer), mpb.WithAutoRefresh())}
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
		mpb.BarRemoveOnComplete(),
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
			&cli.BoolFlag{Name: "overwrite", Usage: "overwrite existing skills and symlinks"},
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

	// Process repositories as soon as they are available. Local repositories
	// are handled in-line; remote repositories are cloned with bounded
	// parallelism and each clones triggers its own follow-up skill processing
	// instead of waiting for every other clone to finish.
	var outcomeMu sync.Mutex
	var skillMu sync.Mutex
	appendOutcome := func(outcome operationOutcome) {
		outcomeMu.Lock()
		defer outcomeMu.Unlock()
		outcomes = append(outcomes, outcome)
	}

	processPrepared := func(prepared *applyRepository, workspace *source.Workspace, obtainErr error) {
		prepared.workspace = workspace
		prepared.obtainErr = obtainErr
		repository := prepared.repository
		if len(prepared.entriesToResolve) == 0 {
			if workspace != nil {
				_ = workspace.Cleanup()
			}
			return
		}
		if obtainErr != nil {
			for _, entry := range prepared.entriesToResolve {
				appendOutcome(operationOutcome{
					Operation:  "apply",
					Skill:      entry.Name,
					State:      normalizeSkillState(entry.State),
					Action:     actionFailed,
					Repository: repository.Repo,
					Source:     prepared.sourceLocation,
					Path:       config.JoinBase(entry.BasePath, entry.Name),
					Targets:    targets,
					DryRun:     c.Bool("dry-run"),
					Err:        obtainErr,
				})
			}
			return
		}
		resolution := source.ResolveSkills(prepared.workspace.Path, prepared.entriesToResolve)
		for _, failure := range resolution.Failures {
			appendOutcome(operationOutcome{
				Operation:  "apply",
				Skill:      failure.Entry.Name,
				State:      normalizeSkillState(failure.Entry.State),
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
			skillMu.Lock()
			outcome, lastBar := applySkill(logger, c, storeRoot, targets, repository.Repo, prepared.sourceLocation, relativeRepositoryPath(prepared.workspace.Path, skill.Path), skill, progress, prepared.progressBar)
			skillMu.Unlock()
			prepared.progressBar = lastBar
			appendOutcome(outcome)
		}
		if err := prepared.workspace.Cleanup(); err != nil {
			appendOutcome(operationOutcome{
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

	for i := range repositories {
		if len(repositories[i].entriesToResolve) == 0 {
			continue
		}
		if repositories[i].provider.Type == "local" {
			workspace, err := resolver.Obtain(repositories[i].repository.Repo, repositories[i].provider)
			processPrepared(&repositories[i], workspace, err)
		}
	}

	remote := make([]int, 0)
	for i := range repositories {
		if len(repositories[i].entriesToResolve) == 0 {
			continue
		}
		if repositories[i].provider.Type != "local" {
			remote = append(remote, i)
		}
	}
	if len(remote) > 0 {
		const maxConcurrentClones = 4
		workerCount := min(len(remote), maxConcurrentClones)
		jobs := make(chan int, len(remote))
		for _, index := range remote {
			jobs <- index
		}
		close(jobs)

		var wg sync.WaitGroup
		sem := make(chan struct{}, workerCount)
		wg.Add(workerCount)
		for range workerCount {
			go func() {
				defer wg.Done()
				for index := range jobs {
					repo := repositories[index]
					bar := progress.addSpinner("Clone repo: "+repo.repository.Repo, nil)
					repositories[index].progressBar = bar
					sem <- struct{}{}
					workspace, err := resolver.Obtain(repo.repository.Repo, repo.provider)
					<-sem
					completeTask(bar)
					processPrepared(&repositories[index], workspace, err)
				}
			}()
		}
		wg.Wait()
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
		State:      normalizeSkillState(skill.Entry.State),
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
	if err := validateStoreTargetPaths(storeRoot, skill.Entry.Name, targets); err != nil {
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, after
	}
	logger.Debug("evaluating configured skill", "skill", skill.Entry.Name, "repository", repository, "path", inRepoPath)
	if outcome.DryRun {
		outcome.Action = actionForState(skill.Entry.State)
		logger.Info("dry-run: planned skill operation", "skill", skill.Entry.Name, "action", outcome.Action, "targets", strings.Join(targets, ","))
		return outcome, after
	}
	if skill.Entry.State == "uninstalled" {
		uninstallBar := progress.add("Uninstall skill: "+skill.Entry.Name, after)
		outcome.Action, outcome.Err = removeSkill(logger, storePath, skill.Entry.Name, targets, &outcome)
		completeTask(uninstallBar)
		return outcome, uninstallBar
	}
	installBar := progress.add("Install skill: "+skill.Entry.Name, after)
	symlinkBars := symlinkTaskBars(progress, targets, skill.Entry.Name, installBar)
	lastBar := lastTaskBar(symlinkBars, installBar)
	storeChanged, err := store.StoreSkill(skill.Path, storePath, c.Bool("overwrite"))
	completeTask(installBar)
	if err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, lastBar
	}
	if err := validateStoreTargetPaths(storeRoot, skill.Entry.Name, targets); err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, lastBar
	}
	linkChanged, err := linkTargets(logger, storeRoot, storePath, skill.Entry.Name, targets, symlinkBars, &outcome, c.Bool("overwrite"))
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

func validateStoreTargetPaths(storeRoot, skillName string, targets []string) error {
	canonicalTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		targetPath := filepath.Join(target, skillName)
		if err := validateStoreTargetPath(storeRoot, targetPath); err != nil {
			return err
		}
		canonicalTarget, err := canonicalPath(targetPath)
		if err != nil {
			return fmt.Errorf("resolve target path %q: %w", targetPath, err)
		}
		for i, existing := range canonicalTargets {
			if pathContains(existing, canonicalTarget) || pathContains(canonicalTarget, existing) {
				return fmt.Errorf("target paths %q and %q overlap", filepath.Join(targets[i], skillName), targetPath)
			}
		}
		canonicalTargets = append(canonicalTargets, canonicalTarget)
	}
	return nil
}

func validateStoreTargetPath(storeRoot, targetPath string) error {
	canonicalStore, err := canonicalStoreRoot(storeRoot)
	if err != nil {
		return fmt.Errorf("resolve store path %q: %w", storeRoot, err)
	}
	storeLocation, err := canonicalPath(storeRoot)
	if err != nil {
		return fmt.Errorf("resolve store path %q: %w", storeRoot, err)
	}
	canonicalTarget, err := canonicalPath(targetPath)
	if err != nil {
		return fmt.Errorf("resolve target path %q: %w", targetPath, err)
	}
	for _, storePath := range []string{canonicalStore, storeLocation} {
		if pathContains(storePath, canonicalTarget) || pathContains(canonicalTarget, storePath) {
			return fmt.Errorf("target path %q overlaps central store path %q", targetPath, storeRoot)
		}
	}
	return nil
}

func canonicalStoreRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	info, lstatErr := os.Lstat(absolute)
	if lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("resolve dangling store root symlink %q: %w", absolute, err)
	}
	if lstatErr != nil && !os.IsNotExist(lstatErr) {
		return "", lstatErr
	}
	return canonicalPath(absolute)
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	name := filepath.Base(absolute)
	current := filepath.Dir(absolute)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if os.IsNotExist(err) {
			info, lstatErr := os.Lstat(current)
			if lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("resolve dangling symlink %q: %w", current, err)
			}
			if lstatErr != nil && !os.IsNotExist(lstatErr) {
				return "", lstatErr
			}
		}
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(filepath.Join(resolved, name)), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(absolute), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathContains(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
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
		printHeading(writer, "Skills to install")
		for _, entry := range install {
			fmt.Fprintf(writer, "  - Repository: %s\n", entry.repo)
			fmt.Fprintln(writer, "    - Skills:")
			for _, skill := range entry.skills {
				fmt.Fprintf(writer, "      - %s\n", skill)
			}
		}
	}
	if len(uninstall) > 0 {
		printHeading(writer, "Skills to uninstall")
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
	overwrite       bool
}

func directCommandFlags(overwrite bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "repo", Usage: "Repository path or provider repository name"},
		&cli.StringFlag{Name: "provider-type", Usage: "Provider type: github, local, or custom"},
		&cli.StringFlag{Name: "provider-base-url", Usage: "Base URL for a custom provider"},
		&cli.StringFlag{Name: "skill", Usage: "Skill name or '*' for all; defaults to configured repository skills"},
		&cli.StringFlag{Name: "skill-base-path", Usage: "Path to skills in the repository"},
		&cli.StringFlag{Name: "skill-dirs", Usage: "Comma-separated target skill directories"},
		&cli.StringFlag{Name: "agents", Aliases: []string{"a"}, Usage: "Comma-separated configured agent names"},
		&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Skip confirmation"},
		&cli.BoolFlag{Name: "debug", Aliases: []string{"d"}, Usage: "Emit debug logs"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Report changes without modifying files"},
	}
	if overwrite {
		flags = append(flags, &cli.BoolFlag{Name: "overwrite", Usage: "Overwrite existing skills and symlinks"})
	}
	return flags
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
		overwrite:       c.Bool("overwrite"),
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
	repository, entries, err := resolveSelection(opts, cfg, true)
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

	var outcomes []operationOutcome
	valid := entries[:0]
	for _, entry := range entries {
		if err := validateDirectSelection(repository, entry); err != nil {
			outcomes = append(outcomes, operationOutcome{Operation: "install", Skill: entry.Name, Repository: repository.Repo, Action: actionFailed, Err: err})
		} else {
			valid = append(valid, entry)
		}
	}
	entries = valid
	if len(entries) == 0 {
		failed := logSummary(commandWriter(c), "install", outcomes)
		return fmt.Errorf("install completed with %d failed operation(s)", failed)
	}
	storeRoot, err := storeRoot()
	if err != nil {
		return err
	}
	provider := repository.EffectiveProvider()
	sourceLocation := repositorySource(repository.Repo, provider)
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
		for _, entry := range entries {
			outcomes = append(outcomes, operationOutcome{Operation: "install", Skill: entry.Name, State: "installed", Action: actionFailed, Repository: repository.Repo, Source: sourceLocation, Path: config.JoinBase(entry.BasePath, entry.Name), Targets: targets, DryRun: opts.dryRun, Err: err})
		}
		logSummary(commandWriter(c), "install", outcomes)
		return err
	}
	defer workspace.Cleanup()
	resolution := source.ResolveSkills(workspace.Path, entries)
	if len(resolution.Skills) == 0 && len(resolution.Failures) == 0 && len(outcomes) == 0 {
		return fmt.Errorf("no skills selected for %q", repository.Repo)
	}
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
	if len(resolution.Skills) == 0 && len(outcomes) > 0 {
		failed := logSummary(commandWriter(c), "install", outcomes)
		return fmt.Errorf("install completed with %d failed operation(s)", failed)
	}
	if err := confirm(c, opts, fmt.Sprintf("install %d skill(s)", len(resolution.Skills))); err != nil {
		if len(outcomes) > 0 {
			logSummary(commandWriter(c), "install", outcomes)
		}
		return err
	}
	pathsByName := make(map[string]string)
	conflicts := make(map[string]bool)
	for _, resolved := range resolution.Skills {
		name := resolved.Entry.Name
		if path, ok := pathsByName[name]; ok && path != resolved.Path {
			conflicts[name] = true
		} else {
			pathsByName[name] = resolved.Path
		}
	}
	seen := make(map[string]bool)
	for _, resolved := range resolution.Skills {
		name := resolved.Entry.Name
		if seen[name] {
			continue
		}
		seen[name] = true
		if conflicts[name] {
			outcomes = append(outcomes, operationOutcome{Operation: "install", Skill: name, Repository: repository.Repo, Action: actionFailed, Err: fmt.Errorf("conflicting source paths for skill %q", name)})
			continue
		}
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
		State:      "installed",
		Repository: repository,
		Source:     sourceLocation,
		Path:       inRepoPath,
		Targets:    targets,
		DryRun:     opts.dryRun,
	}
	storePath := filepath.Join(storeRoot, resolved.Entry.Name)
	if err := validateStoreTargetPaths(storeRoot, resolved.Entry.Name, targets); err != nil {
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, after
	}
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
	storeChanged, err := store.StoreSkill(resolved.Path, storePath, opts.overwrite)
	completeTask(installBar)
	if err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = fmt.Errorf("store skill %q: %w", resolved.Entry.Name, err)
		return outcome, lastBar
	}
	if err := validateStoreTargetPaths(storeRoot, resolved.Entry.Name, targets); err != nil {
		abortTasks(symlinkBars)
		outcome.Action = actionFailed
		outcome.Err = err
		return outcome, lastBar
	}
	logger.Debug("copy decision complete", "skill", resolved.Entry.Name, "changed", storeChanged)
	linkChanged, err := linkTargets(logger, storeRoot, storePath, resolved.Entry.Name, targets, symlinkBars, &outcome, opts.overwrite)
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
	repository, entries, err := resolveSelection(opts, cfg, false)
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
	var selected []config.SkillEntry
	var outcomes []operationOutcome
	var workspace *source.Workspace
	for _, entry := range entries {
		if err := validateDirectSelection(repository, entry); err != nil {
			outcomes = append(outcomes, operationOutcome{Operation: "uninstall", Skill: entry.Name, Repository: repository.Repo, Action: actionFailed, Err: err})
			continue
		}
		if entry.Name != "*" {
			selected = append(selected, entry)
			continue
		}
		if strings.TrimSpace(repository.Repo) == "" {
			return errors.New("repository is required for wildcard uninstall")
		}
		if workspace == nil {
			workspace, err = source.Obtain(repository.Repo, repository.EffectiveProvider())
			if err != nil {
				outcomes = append(outcomes, operationOutcome{Operation: "uninstall", Skill: entry.Name, Repository: repository.Repo, Action: actionFailed, Err: err})
				continue
			}
			defer workspace.Cleanup()
		}
		resolution := source.ResolveSkills(workspace.Path, []config.SkillEntry{entry})
		for _, failure := range resolution.Failures {
			outcomes = append(outcomes, operationOutcome{Operation: "uninstall", Skill: failure.Entry.Name, Repository: repository.Repo, Action: actionFailed, Err: failure.Err})
		}
		for _, skill := range resolution.Skills {
			selected = append(selected, skill.Entry)
		}
	}
	if len(selected) == 0 && len(outcomes) == 0 {
		return fmt.Errorf("no skills selected for %q", repository.Repo)
	}
	if len(selected) == 0 && len(outcomes) > 0 {
		failed := logSummary(commandWriter(c), "uninstall", outcomes)
		return fmt.Errorf("uninstall completed with %d failed operation(s)", failed)
	}
	if err := confirm(c, opts, fmt.Sprintf("uninstall %d skill(s)", len(selected))); err != nil {
		if len(outcomes) > 0 {
			logSummary(commandWriter(c), "uninstall", outcomes)
		}
		return err
	}
	sourceLocation := ""
	if strings.TrimSpace(repository.Repo) != "" {
		sourceLocation = repositorySource(repository.Repo, repository.EffectiveProvider())
	}
	for _, entry := range selected {
		storePath, err := storePathFor(entry.Name)
		if err != nil {
			return err
		}
		outcome := operationOutcome{
			Operation: "uninstall", Skill: entry.Name, State: "uninstalled",
			Repository: repository.Repo, Source: sourceLocation,
			Targets: targets, DryRun: opts.dryRun,
		}
		if sourceLocation != "" {
			outcome.Path = config.JoinBase(entry.BasePath, entry.Name)
		}
		if opts.dryRun {
			logger.Info("dry-run: would unlink skill", "skill", entry.Name, "targets", strings.Join(targets, ","))
			logger.Info("dry-run: would remove stored skill", "skill", entry.Name, "store", storePath)
			outcome.Action = actionUninstalled
		} else {
			outcome.Action, outcome.Err = removeSkill(logger, storePath, entry.Name, targets, &outcome)
		}
		outcomes = append(outcomes, outcome)
	}
	failed := logSummary(commandWriter(c), "uninstall", outcomes)
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

func resolveSelection(opts commandOptions, cfg *config.Config, requireRepository bool) (config.Repository, []config.SkillEntry, error) {
	var matched *config.Repository
	if !requireRepository && opts.repo == "" && opts.skill != "" && opts.skill != "*" && len(cfg.Skills) > 1 {
		cfg = &config.Config{}
	}
	for i := range cfg.Skills {
		if opts.repo == "" || cfg.Skills[i].Repo == opts.repo {
			if matched != nil {
				return config.Repository{}, nil, errors.New("repository is ambiguous; provide --repo")
			}
			matched = &cfg.Skills[i]
		}
	}

	repository := config.Repository{Repo: opts.repo}
	if matched != nil {
		repository = *matched
	}
	if requireRepository && strings.TrimSpace(repository.Repo) == "" {
		return config.Repository{}, nil, errors.New("repository is required (use --repo or configure exactly one repository)")
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
	if err := config.Validate(&config.Config{Skills: []config.Repository{repository}}); err != nil {
		return config.Repository{}, nil, err
	}

	var entries []config.SkillEntry
	if opts.skill != "" {
		entry := config.SkillEntry{Name: opts.skill}
		if matched != nil {
			found := false
			for _, configured := range matched.Skills {
				if configured.Name == opts.skill {
					if found && opts.skillBasePath == "" && config.JoinBase(entry.BasePath, entry.Name) != config.JoinBase(configured.BasePath, configured.Name) {
						return config.Repository{}, nil, fmt.Errorf("skill %q has ambiguous configured base paths; use --skill-base-path", opts.skill)
					}
					entry = configured
					found = true
				}
			}
		}
		entries = []config.SkillEntry{entry}
	} else if matched != nil {
		entries = append(entries, matched.Skills...)
	}
	if len(entries) == 0 {
		return config.Repository{}, nil, errors.New("skill is required (use --skill or configure repository skills)")
	}
	for i := range entries {
		if opts.skillBasePath != "" {
			entries[i].BasePath = opts.skillBasePath
		}
	}
	return repository, entries, nil
}

func validateDirectSelection(repository config.Repository, entry config.SkillEntry) error {
	if strings.TrimSpace(entry.Name) == "" || filepath.IsAbs(entry.Name) || filepath.Clean(entry.Name) != entry.Name || entry.Name == "." || entry.Name == ".." || strings.Contains(entry.Name, string(filepath.Separator)) {
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

func normalizeSkillState(state string) string {
	if strings.TrimSpace(state) == "" {
		return "installed"
	}
	return state
}

func linkTargets(logger *slog.Logger, storeRoot, storePath, skillName string, targets []string, bars []*mpb.Bar, outcome *operationOutcome, overwrite bool) (bool, error) {
	changedAny := false
	var firstErr error
	for i, target := range targets {
		linkPath := filepath.Join(target, skillName)
		logger.Debug("evaluating skill link", "skill", skillName, "link", linkPath)
		if err := validateStoreTargetPath(storeRoot, linkPath); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", target, err)
			}
			completeTask(bars[i])
			continue
		}
		changed, err := store.LinkSkill(storePath, linkPath, overwrite)
		completeTask(bars[i])
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", target, err)
			}
		} else {
			changedAny = changedAny || changed
			if changed {
				outcome.CreatedLinks = append(outcome.CreatedLinks, linkPath)
			}
			logger.Debug("link decision complete", "skill", skillName, "changed", changed)
		}
	}
	return changedAny, firstErr
}

func removeSkill(logger *slog.Logger, storePath, skillName string, targets []string, outcome *operationOutcome) (string, error) {
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
	removed, err := unlinkTargets(logger, storePath, skillName, targets, outcome)
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

func unlinkTargets(logger *slog.Logger, storePath, skillName string, targets []string, outcome *operationOutcome) (bool, error) {
	changedAny := false
	var firstErr error
	for _, target := range targets {
		linkPath := filepath.Join(target, skillName)
		logger.Debug("evaluating skill unlink", "skill", skillName, "link", linkPath)
		changed, err := store.UnlinkManagedSkill(storePath, linkPath)
		if err != nil {
			// A path that is not a managed symlink for this store is not a
			// failure: skip it and continue unlinking every managed symlink
			// before the store copy is removed.
			if errors.Is(err, store.ErrPathConflict) {
				logger.Debug("skipping unmanaged path", "skill", skillName, "link", linkPath)
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", target, err)
			}
			continue
		}
		changedAny = changedAny || changed
		if changed {
			outcome.DeletedLinks = append(outcome.DeletedLinks, linkPath)
		}
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
	addOutcome := func(groups *[]repositorySummary, outcome operationOutcome) {
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
		item := fmt.Sprintf("%s (%s)", name, detail)
		for _, path := range outcome.CreatedLinks {
			item += "\n        - symlink created: " + path
		}
		for _, path := range outcome.DeletedLinks {
			item += "\n        - symlink deleted: " + path
		}
		(*groups)[index].skills = append((*groups)[index].skills, item)
	}

	var installed, skippedInstalled, uninstalled, skippedUninstalled []repositorySummary
	for _, outcome := range outcomes {
		if outcome.Action == actionFailed {
			failed++
		}
		state := normalizeSkillState(outcome.State)
		uninstall := operation == "uninstall" || state == "uninstalled"
		if uninstall {
			if outcome.Action == actionSkipped {
				addOutcome(&skippedUninstalled, outcome)
			} else {
				addOutcome(&uninstalled, outcome)
			}
		} else {
			if outcome.Action == actionSkipped {
				addOutcome(&skippedInstalled, outcome)
			} else {
				addOutcome(&installed, outcome)
			}
		}
	}
	printGroup := func(title string, groups []repositorySummary) {
		if len(groups) == 0 {
			return
		}
		printHeading(writer, title)
		for _, repository := range groups {
			fmt.Fprintf(writer, "  - Repository: %s\n    - Skills:\n", repository.name)
			for _, skill := range repository.skills {
				fmt.Fprintf(writer, "      - %s\n", skill)
			}
		}
	}

	sections := []struct {
		title  string
		groups []repositorySummary
	}{
		{"Installed skills", installed},
		{"Skills to install but skipped (already installed)", skippedInstalled},
		{"Uninstalled skills", uninstalled},
		{"Skills to uninstall but skipped (already uninstalled)", skippedUninstalled},
	}
	for _, section := range sections {
		if len(section.groups) == 0 {
			continue
		}
		printGroup(section.title, section.groups)
	}
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

func printHeading(writer io.Writer, title string) {
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, colorizeHeading(title))
}

// colorizeHeading highlights a section heading for terminal output.
func colorizeHeading(title string) string {
	return "\033[1;36m" + title + "\033[0m"
}

func commandWriter(c *cli.Context) io.Writer {
	if c.App.Writer != nil {
		return c.App.Writer
	}
	return os.Stdout
}
