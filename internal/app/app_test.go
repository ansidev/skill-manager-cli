package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			want:    "skill-manager",
		},
		{
			name:    "with defaults",
			version: "dev",
			commit:  "unknown",
			want:    "skill-manager",
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

			want := "skill-manager version 1.0.0 (commit abc123)"
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
	storePath := filepath.Join(root, ".skill-manager", "skills", "example")
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
	if _, err := os.Lstat(filepath.Join(root, ".skill-manager", "skills", "example")); !os.IsNotExist(err) {
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
	if _, err := os.Lstat(filepath.Join(root, ".skill-manager", "skills", "example")); !os.IsNotExist(err) {
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
	storePath := filepath.Join(root, ".skill-manager", "skills", "example")
	writeFile(t, filepath.Join(storePath, "SKILL.md"), "# Example\n")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(storePath, filepath.Join(target, "example")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repository+"\n    provider:\n      type: local\n    skills:\n      - name: example\n        state: uninstalled\n")

	app := New("dev", "test")
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
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatal("apply returned nil error, want summary of the missing skill failure")
	}
	if _, err := os.Stat(filepath.Join(target, "valid", "SKILL.md")); err != nil {
		t.Fatalf("valid skill was not applied after independent failure: %v", err)
	}
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
