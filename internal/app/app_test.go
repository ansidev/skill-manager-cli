package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ansidev/skill-manager-cli/internal/config"
	"github.com/ansidev/skill-manager-cli/internal/source"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{
			name:    "with version and commit",
			version: "1.0.0",
			commit:  "abc123",
			want:    "skill-manager-cli",
		},
		{
			name:    "with defaults",
			version: "dev",
			commit:  "unknown",
			want:    "skill-manager-cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.version, tt.commit)
			if got == nil {
				t.Fatal("New() returned nil")
			}
			if got.Name != tt.want {
				t.Errorf("New().Name = %v, want %v", got.Name, tt.want)
			}
			if got.Usage == "" {
				t.Error("New().Usage should not be empty")
			}
		})
	}
}

func TestNew_SetsVersion(t *testing.T) {
	app := New("1.0.0", "abc123")
	if got, want := app.Version, "1.0.0 (commit abc123)"; got != want {
		t.Errorf("New().Version = %q, want %q", got, want)
	}
}

func TestNew_HelpShowsVersionUsageAndCommands(t *testing.T) {
	app := New("1.0.0", "abc123")
	var output bytes.Buffer
	app.Writer = &output

	if err := app.Run([]string{"skill", "--help"}); err != nil {
		t.Fatalf("Run(--help) returned error: %v", err)
	}

	for _, want := range []string{
		"1.0.0 (commit abc123)",
		"Manage agent skills for coding assistants",
		"init",
		"apply",
		"install",
		"uninstall",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help output missing %q:\n%s", want, output.String())
		}
	}
}

func TestNew_VersionFlagsPrintVersionAndCommit(t *testing.T) {
	for _, argument := range []string{"-v", "--version"} {
		t.Run(argument, func(t *testing.T) {
			app := New("1.0.0", "abc123")
			var output bytes.Buffer
			app.Writer = &output

			if err := app.Run([]string{"skill", argument}); err != nil {
				t.Fatalf("Run(%s) returned error: %v", argument, err)
			}

			want := "skill-manager-cli version 1.0.0 (commit abc123)"
			if got := output.String(); got != want+"\n" {
				t.Errorf("Run(%s) output = %q, want %q", argument, got, want+"\n")
			}
		})
	}
}

func TestNew_RegistersCommands(t *testing.T) {
	app := New("1.0.0", "abc123")
	got := make(map[string]bool, len(app.Commands))
	for _, command := range app.Commands {
		got[command.Name] = true
	}

	for _, want := range []string{"init", "apply", "install", "uninstall"} {
		if !got[want] {
			t.Errorf("New() commands missing %q", want)
		}
	}
}

func TestApplyInstallsConfiguredLocalSkill(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repository := filepath.Join(root, "repository")
	writeFile(t, filepath.Join(repository, "skills", "example", "SKILL.md"), "# Example\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: example\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output

	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err != nil {
		t.Fatalf("apply returned error: %v\n%s", err, output.String())
	}

	linkPath := filepath.Join(target, "example")
	storePath := filepath.Join(root, ".agents", "skills", "example")
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%q): %v", linkPath, err)
	}
	if got != storePath {
		t.Errorf("link target = %q, want %q", got, storePath)
	}
	if _, err := os.Stat(filepath.Join(storePath, "SKILL.md")); err != nil {
		t.Fatalf("stored skill missing: %v", err)
	}
}

func TestApplyRejectionDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repository := filepath.Join(root, "repository")
	writeFile(t, filepath.Join(repository, "skills", "example", "SKILL.md"), "# Example\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: example\n")

	app := New("dev", "test")
	app.Reader = strings.NewReader("n\n")
	if err := app.Run([]string{"skill", "apply", "--agents", "pi"}); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(target, "example")); !os.IsNotExist(err) {
		t.Fatalf("target mutation error = %v, want no target skill", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".agents", "skills", "example")); !os.IsNotExist(err) {
		t.Fatalf("store mutation error = %v, want no stored skill", err)
	}
}

func TestApplyDryRunDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repository := filepath.Join(root, "repository")
	writeFile(t, filepath.Join(repository, "skills", "example", "SKILL.md"), "# Example\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: example\n")

	app := New("dev", "test")
	if err := app.Run([]string{"skill", "apply", "--yes", "--dry-run", "--agents", "pi"}); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(target, "example")); !os.IsNotExist(err) {
		t.Fatalf("target mutation error = %v, want no target skill", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".agents", "skills", "example")); !os.IsNotExist(err) {
		t.Fatalf("store mutation error = %v, want no stored skill", err)
	}
}

func TestApplyUninstallsMissingSourceSkill(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "agent", "skills")
	storePath := filepath.Join(root, ".agents", "skills", "example")
	writeFile(t, filepath.Join(storePath, "SKILL.md"), "# Example\n")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(storePath, filepath.Join(target, "example")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: example\n        state: uninstalled\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "example")); !os.IsNotExist(err) {
		t.Fatalf("managed link error = %v, want removed", err)
	}
	if _, err := os.Lstat(storePath); !os.IsNotExist(err) {
		t.Fatalf("stored skill error = %v, want removed", err)
	}
}

func TestApplyContinuesAfterSkillSelectionFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repository := filepath.Join(root, "repository")
	writeFile(t, filepath.Join(repository, "skills", "valid", "SKILL.md"), "# Valid\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: missing\n      - name: valid\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatal("apply returned nil error, want summary of the missing skill failure")
	}
	if _, err := os.Stat(filepath.Join(target, "valid", "SKILL.md")); err != nil {
		t.Fatalf("valid skill was not applied after independent failure: %v", err)
	}
	assertOutputContains(t, output.String(),
		"Skills installed",
		"missing (failed:",
		"valid (installed)",
		"Completed 2 operation(s); 1 failed.",
	)
}

func changeDirectory(t *testing.T, path string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstallThenUninstallLocalSkill(t *testing.T) {
	repository := t.TempDir()
	skillPath := filepath.Join(repository, "skills", "hello")
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	t.Setenv("HOME", home)

	app := New("1.0.0", "abc123")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "hello", "--skill-dirs", target, "-y"}); err != nil {
		t.Fatalf("install returned error: %v\n%s", err, output.String())
	}
	storePath := filepath.Join(home, ".agents", "skills", "hello")
	if _, err := os.Stat(filepath.Join(storePath, "SKILL.md")); err != nil {
		t.Fatalf("stored skill missing: %v", err)
	}
	linkPath := filepath.Join(target, "hello")
	if info, err := os.Lstat(linkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target should be a managed symlink, info=%v err=%v", info, err)
	}

	if err := app.Run([]string{"skill", "uninstall", "--skill", "hello", "--skill-dirs", target, "-y"}); err != nil {
		t.Fatalf("uninstall returned error: %v\n%s", err, output.String())
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("managed link remains after uninstall: %v", err)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("stored skill remains after uninstall: %v", err)
	}
}

func TestInstallDryRunDoesNotMutate(t *testing.T) {
	repository := t.TempDir()
	skillPath := filepath.Join(repository, "skills", "hello")
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	t.Setenv("HOME", home)

	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "hello", "--skill-dirs", target, "--dry-run"}); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "hello")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created store entry: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "hello")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target link: %v", err)
	}
}

func TestInstallRequiresRepositoryAndSkill(t *testing.T) {
	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "install", "--skill-dirs", t.TempDir(), "-y"}); err == nil {
		t.Fatal("install without repository and skill returned nil error")
	}
}

func TestInstallHonorsConfirmation(t *testing.T) {
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	target := filepath.Join(t.TempDir(), "target")

	app := New("1.0.0", "abc123")
	var output bytes.Buffer
	app.Writer = &output
	app.Reader = strings.NewReader("n\n")
	err := app.Run([]string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "hello", "--skill-dirs", target})
	if err == nil {
		t.Fatalf("declined confirmation returned nil error\n%s", output.String())
	}
	if _, statErr := os.Lstat(filepath.Join(target, "hello")); !os.IsNotExist(statErr) {
		t.Fatalf("declined install created a link: %v", statErr)
	}
}

func TestUninstallPreservesUserOwnedTargetAndStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storePath := filepath.Join(home, ".agents", "skills", "hello")
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	userPath := filepath.Join(target, "hello")
	if err := os.MkdirAll(userPath, 0o755); err != nil {
		t.Fatal(err)
	}

	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "uninstall", "--skill", "hello", "--skill-dirs", target, "-y"}); err == nil {
		t.Fatal("uninstall should report the user-owned path conflict")
	}
	if _, err := os.Stat(filepath.Join(userPath, "..")); err != nil {
		t.Fatalf("user-owned target parent removed: %v", err)
	}
	if info, err := os.Stat(userPath); err != nil || !info.IsDir() {
		t.Fatalf("user-owned target was not preserved: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("store entry removed despite no managed link: %v", err)
	}
}

func TestInstallRejectsTraversalSkillName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "install", "--repo", t.TempDir(), "--provider-type", "local", "--skill", "../escape", "--skill-dirs", t.TempDir(), "-y"}); err == nil {
		t.Fatal("traversal skill name returned nil error")
	}
}

func TestInstallFallsBackToConfig(t *testing.T) {
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	target := filepath.Join(t.TempDir(), "target")
	t.Chdir(t.TempDir())
	cfg := "dirs:\n  pi: " + target + "\nskills:\n  - repo: " + repository + "\n    provider:\n      type: local\n    skills:\n      - name: hello\n"
	if err := os.WriteFile("config.yaml", []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "install", "-y"}); err != nil {
		t.Fatalf("install using config defaults returned error: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(target, "hello")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config-driven install did not link: info=%v err=%v", info, err)
	}
}

func TestUninstallDryRunDoesNotMutate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	storePath := filepath.Join(home, ".agents", "skills", "hello")
	if err := os.MkdirAll(storePath, 0o755); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	linkPath := filepath.Join(target, "hello")
	if err := os.Symlink(storePath, linkPath); err != nil {
		t.Fatal(err)
	}

	app := New("1.0.0", "abc123")
	if err := app.Run([]string{"skill", "uninstall", "--skill", "hello", "--skill-dirs", target, "--dry-run"}); err != nil {
		t.Fatalf("uninstall dry-run returned error: %v", err)
	}
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("dry-run removed the link: %v", err)
	}
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("dry-run removed the store entry: %v", err)
	}
}

func TestObtainRepositoriesClonesConcurrently(t *testing.T) {
	runner := &blockingCloneRunner{entered: make(chan struct{}, 2), release: make(chan struct{})}
	repositories := []applyRepository{
		{repository: config.Repository{Repo: "first", Provider: &config.Provider{Type: "github"}}, entriesToResolve: []config.SkillEntry{{Name: "alpha"}}},
		{repository: config.Repository{Repo: "second", Provider: &config.Provider{Type: "github"}}, entriesToResolve: []config.SkillEntry{{Name: "beta"}}},
	}
	done := make(chan struct{})
	go func() {
		obtainRepositories(&source.Resolver{Runner: runner}, repositories, nil)
		close(done)
	}()

	for range 2 {
		select {
		case <-runner.entered:
		case <-time.After(time.Second):
			close(runner.release)
			t.Fatal("expected both repository clones to start before either completed")
		}
	}
	close(runner.release)
	<-done
	for _, repository := range repositories {
		if repository.workspace == nil {
			t.Fatalf("repository %q did not get a workspace", repository.repository.Repo)
		}
		if err := repository.workspace.Cleanup(); err != nil {
			t.Errorf("cleanup %q workspace: %v", repository.repository.Repo, err)
		}
	}
}

type blockingCloneRunner struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingCloneRunner) Run(string, ...string) ([]byte, []byte, error) {
	r.entered <- struct{}{}
	<-r.release
	return nil, nil, nil
}

func TestCommandsDoNotEmitStructuredSlogOutput(t *testing.T) {
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())

	app := New("1.0.0", "abc123")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "hello", "--skill-dirs", t.TempDir(), "--dry-run", "-d"}); err != nil {
		t.Fatalf("install returned error: %v", err)
	}
	if strings.Contains(output.String(), "time=") || strings.Contains(output.String(), "level=") || strings.Contains(output.String(), "msg=") {
		t.Fatalf("unexpected structured slog output:\n%s", output.String())
	}
}

func TestEndToEndLifecycleWithOperationSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	t.Chdir(work)

	application := New("dev", "test")
	var output bytes.Buffer
	application.Writer = &output
	application.Reader = strings.NewReader("")

	if err := application.Run([]string{"skill", "init"}); err != nil {
		t.Fatalf("init returned error: %v\n%s", err, output.String())
	}
	if _, err := os.Stat(filepath.Join(work, "config.yaml")); err != nil {
		t.Fatalf("init did not create config.yaml: %v", err)
	}
	if _, err := config.Load(filepath.Join(work, "config.yaml")); err != nil {
		t.Fatalf("generated config is not loadable: %v", err)
	}

	repository := filepath.Join(work, "repository")
	writeFile(t, filepath.Join(repository, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	writeFile(t, filepath.Join(repository, "skills", "beta", "SKILL.md"), "# Beta\n")
	namedTarget := filepath.Join(work, "named-skills")
	wildcardTarget := filepath.Join(work, "wildcard-skills")
	storeRoot := filepath.Join(home, ".agents", "skills")

	writeLifecycleConfig(t, repository, namedTarget, wildcardTarget, "      - name: alpha\n")

	output.Reset()
	application.Reader = strings.NewReader("y\n")
	if err := application.Run([]string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "alpha", "--skill-dirs", namedTarget}); err != nil {
		t.Fatalf("confirmed install returned error: %v\n%s", err, output.String())
	}
	assertLinkTarget(t, filepath.Join(namedTarget, "alpha"), filepath.Join(storeRoot, "alpha"))
	assertFileExists(t, filepath.Join(storeRoot, "alpha", "SKILL.md"))
	assertOutputContains(t, output.String(),
		"Skills installed",
		"alpha (installed)",
		"Completed 1 operation(s); 0 failed.",
	)

	output.Reset()
	if err := application.Run([]string{"skill", "uninstall", "--skill", "alpha", "--skill-dirs", namedTarget, "-y"}); err != nil {
		t.Fatalf("uninstall returned error: %v\n%s", err, output.String())
	}
	assertPathMissing(t, filepath.Join(namedTarget, "alpha"))
	assertPathMissing(t, filepath.Join(storeRoot, "alpha"))
	assertOutputContains(t, output.String(),
		"Skills uninstalled",
		"alpha (uninstalled)",
		"Completed 1 operation(s); 0 failed.",
	)

	writeLifecycleConfig(t, repository, namedTarget, wildcardTarget, "      - name: \"*\"\n")
	output.Reset()
	if err := application.Run([]string{"skill", "apply", "--yes", "--dry-run", "--agents", "wildcard"}); err != nil {
		t.Fatalf("dry-run apply returned error: %v\n%s", err, output.String())
	}
	for _, skill := range []string{"alpha", "beta"} {
		assertPathMissing(t, filepath.Join(storeRoot, skill))
		assertPathMissing(t, filepath.Join(wildcardTarget, skill))
	}
	assertOutputContains(t, output.String(),
		"Skills installed",
		"alpha (installed)",
		"beta (installed)",
		"Completed 2 operation(s); 0 failed.",
	)

	output.Reset()
	application.Reader = strings.NewReader("y\n")
	if err := application.Run([]string{"skill", "apply", "--agents", "wildcard"}); err != nil {
		t.Fatalf("confirmed apply returned error: %v\n%s", err, output.String())
	}
	for _, skill := range []string{"alpha", "beta"} {
		assertLinkTarget(t, filepath.Join(wildcardTarget, skill), filepath.Join(storeRoot, skill))
		assertFileExists(t, filepath.Join(storeRoot, skill, "SKILL.md"))
	}
	assertOutputContains(t, output.String(),
		"Skills installed",
		"alpha (installed)",
		"beta (installed)",
		"Completed 2 operation(s); 0 failed.",
	)

	writeLifecycleConfig(t, repository, namedTarget, wildcardTarget, "      - name: beta\n        state: uninstalled\n")
	output.Reset()
	if err := application.Run([]string{"skill", "apply", "--yes", "--agents", "wildcard"}); err != nil {
		t.Fatalf("apply uninstall returned error: %v\n%s", err, output.String())
	}
	assertPathMissing(t, filepath.Join(wildcardTarget, "beta"))
	assertPathMissing(t, filepath.Join(storeRoot, "beta"))
	assertLinkTarget(t, filepath.Join(wildcardTarget, "alpha"), filepath.Join(storeRoot, "alpha"))
	assertOutputContains(t, output.String(),
		"Skills uninstalled",
		"beta (uninstalled)",
		"Completed 1 operation(s); 0 failed.",
	)
}

func TestInstallSummaryReportsSkippedOnRepeat(t *testing.T) {
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "skills", "hello", "SKILL.md"), "# Hello\n")
	t.Setenv("HOME", t.TempDir())
	target := filepath.Join(t.TempDir(), "target")

	application := New("dev", "test")
	var output bytes.Buffer
	application.Writer = &output

	args := []string{"skill", "install", "--repo", repository, "--provider-type", "local", "--skill", "hello", "--skill-dirs", target, "-y"}
	if err := application.Run(args); err != nil {
		t.Fatalf("first install returned error: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := application.Run(args); err != nil {
		t.Fatalf("repeat install returned error: %v\n%s", err, output.String())
	}
	assertOutputContains(t, output.String(),
		"Skills installed",
		"hello (skipped)",
		"Completed 1 operation(s); 0 failed.",
	)
}

func TestUninstallSummaryReportsSkippedWhenNothingManaged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	target := t.TempDir()

	application := New("dev", "test")
	var output bytes.Buffer
	application.Writer = &output

	if err := application.Run([]string{"skill", "uninstall", "--skill", "absent", "--skill-dirs", target, "-y"}); err != nil {
		t.Fatalf("uninstall of absent skill returned error: %v\n%s", err, output.String())
	}
	assertOutputContains(t, output.String(),
		"Skills uninstalled",
		"absent (skipped)",
		"Completed 1 operation(s); 0 failed.",
	)
}

func writeLifecycleConfig(t *testing.T, repository, namedTarget, wildcardTarget, skillEntries string) {
	t.Helper()
	contents := "dirs:\n  named: " + namedTarget + "\n  wildcard: " + wildcardTarget +
		"\nskills:\n  - repo: " + repository +
		"\n    provider:\n      type: local\n    skills:\n" + skillEntries
	writeFile(t, filepath.Join(".", "config.yaml"), contents)
}

func assertLinkTarget(t *testing.T, linkPath, want string) {
	t.Helper()
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%q): %v", linkPath, err)
	}
	if got != want {
		t.Fatalf("link %q target = %q, want %q", linkPath, got, want)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("path %q error = %v, want absent", path, err)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %q: %v", path, err)
	}
}

func assertOutputContains(t *testing.T, output string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, output)
		}
	}
}
