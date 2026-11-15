package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/ansidev/skill-manager-cli/internal/config"
	"github.com/ansidev/skill-manager-cli/internal/source"
)

func TestApplyReportsStateConflictAcrossRepositories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n        state: uninstalled\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"})
	if err == nil {
		t.Fatalf("apply should fail on conflicting states:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "conflicting states", "repo-a", "repo-b", "skills/shared")
	// Fail-fast exits before the plan is printed and before any repository
	// is acquired.
	if bytes.Contains(output.Bytes(), []byte("Skills to install")) || bytes.Contains(output.Bytes(), []byte("Skills to uninstall")) {
		t.Fatalf("fail-fast must exit before printing the apply plan:\n%s", output.String())
	}
	// Neither repository's request may take effect.
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestApplyReportsContentConflictAcrossRepositories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"})
	if err == nil {
		t.Fatalf("apply should fail on differing skill content:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "content differs between repositories", "repo-a", "repo-b", "shared (failed:")
	// The later repository must not silently overwrite the skill.
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestApplyReportsContentConflictResolvedFromWildcard(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: '*'\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"})
	if err == nil {
		t.Fatalf("apply should fail when a wildcard resolves a conflicting name:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "content differs between repositories", "shared (failed:")
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
}

func TestApplyAllowsIdenticalSkillAcrossRepositories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# Same\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# Same\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err != nil {
		t.Fatalf("apply should succeed for identical skill content: %v\n%s", err, output.String())
	}
	if bytes.Contains(output.Bytes(), []byte("Skill conflicts")) {
		t.Fatalf("identical content must not be reported as a conflict:\n%s", output.String())
	}
	storePath := filepath.Join(root, ".agents", "skills", "shared")
	assertFileExists(t, filepath.Join(storePath, "SKILL.md"))
	assertLinkTarget(t, filepath.Join(target, "shared"), storePath)
}

func TestApplyReportsConflictWithoutMutatingInDryRun(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--dry-run", "--agents", "pi"}); err == nil {
		t.Fatalf("dry run should still fail on conflicts:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts")
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
}

// unreachableRepositoryConfig writes a config with one local repository and
// one repository that can only be obtained over the network (unreachable), so
// a passing apply proves no repository acquisition was attempted.
func unreachableRepositoryConfig(t *testing.T, root string) {
	repoA := filepath.Join(root, "repo-a")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: unreachable/repo\n    provider:\n      type: custom\n      base_url: http://127.0.0.1:9/git\n    skills:\n      - name: shared\n        state: uninstalled\n")
}

func TestApplyFailsFastOnStateConflictBeforeAcquisition(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)
	unreachableRepositoryConfig(t, root)
	target := filepath.Join(root, "agent", "skills")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"})
	if err == nil {
		t.Fatalf("apply should fail fast on a configuration-detectable state conflict:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "conflicting states", "repo-a", "unreachable/repo", "skills/shared")
	// The failure must happen before the plan and before any repository
	// acquisition: the plan headings and clone errors must be absent.
	if bytes.Contains(output.Bytes(), []byte("Skills to install")) || bytes.Contains(output.Bytes(), []byte("Skills to uninstall")) {
		t.Fatalf("fail-fast must exit before printing the apply plan:\n%s", output.String())
	}
	if bytes.Contains(output.Bytes(), []byte("Clone repo")) {
		t.Fatalf("fail-fast must exit before acquiring any repository:\n%s", output.String())
	}
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestApplyFailsFastOnStateConflictInDryRun(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)
	unreachableRepositoryConfig(t, root)
	target := filepath.Join(root, "agent", "skills")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	err := app.Run([]string{"skill", "apply", "--yes", "--dry-run", "--agents", "pi"})
	if err == nil {
		t.Fatalf("dry run should fail fast on a configuration-detectable state conflict:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "conflicting states")
	if bytes.Contains(output.Bytes(), []byte("Skills to install")) {
		t.Fatalf("fail-fast must exit before printing the apply plan even in dry run:\n%s", output.String())
	}
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestConfigStateConflicts(t *testing.T) {
	entry := func(name, state string) config.SkillEntry {
		return config.SkillEntry{Name: name, State: state}
	}
	t.Run("state conflict across repositories", func(t *testing.T) {
		cfg := &config.Config{Skills: []config.Repository{
			{Repo: "repo-a", Skills: []config.SkillEntry{entry("shared", "installed")}},
			{Repo: "repo-b", Skills: []config.SkillEntry{entry("shared", "uninstalled")}},
		}}
		conflicts := configStateConflicts(cfg)
		if len(conflicts) != 1 {
			t.Fatalf("conflicts = %v, want 1", conflicts)
		}
		if conflicts[0].Name != "shared" {
			t.Errorf("conflict name = %q, want %q", conflicts[0].Name, "shared")
		}
	})
	t.Run("empty states are normalized to installed", func(t *testing.T) {
		cfg := &config.Config{Skills: []config.Repository{
			{Repo: "repo-a", Skills: []config.SkillEntry{entry("shared", "")}},
			{Repo: "repo-b", Skills: []config.SkillEntry{entry("shared", "installed")}},
		}}
		if conflicts := configStateConflicts(cfg); len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
	t.Run("same name within one repository is not a conflict", func(t *testing.T) {
		cfg := &config.Config{Skills: []config.Repository{
			{Repo: "repo-a", Skills: []config.SkillEntry{entry("shared", "installed"), entry("shared", "uninstalled")}},
		}}
		if conflicts := configStateConflicts(cfg); len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
	t.Run("all installed across repositories is not a state conflict", func(t *testing.T) {
		cfg := &config.Config{Skills: []config.Repository{
			{Repo: "repo-a", Skills: []config.SkillEntry{entry("shared", "installed")}},
			{Repo: "repo-b", Skills: []config.SkillEntry{entry("shared", "installed")}},
		}}
		if conflicts := configStateConflicts(cfg); len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
	t.Run("wildcard anywhere disables fail-fast detection", func(t *testing.T) {
		cfg := &config.Config{Skills: []config.Repository{
			{Repo: "repo-a", Skills: []config.SkillEntry{entry("shared", "installed")}},
			{Repo: "repo-b", Skills: []config.SkillEntry{entry("*", "")}},
		}}
		if conflicts := configStateConflicts(cfg); len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
}

func TestDetectSkillConflicts(t *testing.T) {
	pending := func(repository, name, state, path string) pendingSkill {
		return pendingSkill{
			repository: repository,
			skill: source.ResolvedSkill{
				Entry: config.SkillEntry{Name: name, State: state},
				Path:  path,
			},
		}
	}
	t.Run("state conflict across repositories", func(t *testing.T) {
		conflicts := detectSkillConflicts([]pendingSkill{
			pending("repo-a", "shared", "installed", "/a/shared"),
			pending("repo-b", "shared", "uninstalled", "/b/shared"),
		})
		if len(conflicts) != 1 {
			t.Fatalf("conflicts = %v, want 1", conflicts)
		}
		if conflicts[0].Name != "shared" {
			t.Errorf("conflict name = %q, want %q", conflicts[0].Name, "shared")
		}
	})
	t.Run("content conflict across repositories", func(t *testing.T) {
		root := t.TempDir()
		pathA := filepath.Join(root, "a")
		pathB := filepath.Join(root, "b")
		writeFile(t, filepath.Join(pathA, "SKILL.md"), "# A\n")
		writeFile(t, filepath.Join(pathB, "SKILL.md"), "# B\n")
		conflicts := detectSkillConflicts([]pendingSkill{
			pending("repo-a", "shared", "installed", pathA),
			pending("repo-b", "shared", "installed", pathB),
		})
		if len(conflicts) != 1 {
			t.Fatalf("conflicts = %v, want 1", conflicts)
		}
	})
	t.Run("identical content is not a conflict", func(t *testing.T) {
		root := t.TempDir()
		pathA := filepath.Join(root, "a")
		pathB := filepath.Join(root, "b")
		writeFile(t, filepath.Join(pathA, "SKILL.md"), "# Same\n")
		writeFile(t, filepath.Join(pathB, "SKILL.md"), "# Same\n")
		conflicts := detectSkillConflicts([]pendingSkill{
			pending("repo-a", "shared", "installed", pathA),
			pending("repo-b", "shared", "installed", pathB),
		})
		if len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
	t.Run("same name within one repository is not a cross-repository conflict", func(t *testing.T) {
		conflicts := detectSkillConflicts([]pendingSkill{
			pending("repo-a", "shared", "installed", "/a/shared"),
			pending("repo-a", "shared", "uninstalled", "/a2/shared"),
		})
		if len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
	t.Run("single repository entries never conflict", func(t *testing.T) {
		conflicts := detectSkillConflicts([]pendingSkill{
			pending("repo-a", "alpha", "installed", "/a/alpha"),
			pending("repo-b", "beta", "uninstalled", "/b/beta"),
		})
		if len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, want none", conflicts)
		}
	})
}

func TestApplyConflictLeavesIndependentSkillsApplied(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoA, "skills", "unique-a", "SKILL.md"), "# Unique A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	writeFile(t, filepath.Join(repoB, "skills", "unique-b", "SKILL.md"), "# Unique B\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n      - name: unique-a\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n      - name: unique-b\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatalf("apply should fail while any conflict remains:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "unique-a (installed)", "unique-b (installed)", "shared (failed:")
	assertFileExists(t, filepath.Join(root, ".agents", "skills", "unique-a"))
	assertFileExists(t, filepath.Join(root, ".agents", "skills", "unique-b"))
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
}

func TestApplyContinuesNormalFlowWithoutConfigStateConflict(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	writeFile(t, filepath.Join(repoB, "skills", "beta", "SKILL.md"), "# Beta\n")
	target := filepath.Join(root, "agent", "skills")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: alpha\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: beta\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err != nil {
		t.Fatalf("apply should succeed when no configuration state conflict exists: %v\n%s", err, output.String())
	}
	assertOutputContains(t, output.String(), "Installed skills", "alpha (installed)", "beta (installed)", "Completed 2 operation(s); 0 failed.")
	if bytes.Contains(output.Bytes(), []byte("Skill conflicts")) {
		t.Fatalf("no conflict must be reported for disjoint skill names:\n%s", output.String())
	}
	storeAlpha := filepath.Join(root, ".agents", "skills", "alpha")
	assertFileExists(t, filepath.Join(storeAlpha, "SKILL.md"))
	assertLinkTarget(t, filepath.Join(target, "alpha"), storeAlpha)
	storeBeta := filepath.Join(root, ".agents", "skills", "beta")
	assertFileExists(t, filepath.Join(storeBeta, "SKILL.md"))
	assertLinkTarget(t, filepath.Join(target, "beta"), storeBeta)
}

func TestApplyWildcardEntrySkipsFailFastAndResolvesRepositories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	repoC := filepath.Join(root, "repo-c")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoC, "skills", "solo", "SKILL.md"), "# Solo\n")
	target := filepath.Join(root, "agent", "skills")
	// repo-a and repo-b declare a configuration-detectable state conflict,
	// but repo-c's wildcard entry disables fail-fast, so apply must resolve
	// every repository first and report through the normal flow.
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n        state: uninstalled\n  - repo: "+repoC+"\n    provider:\n      type: local\n    skills:\n      - name: '*'\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatalf("apply should fail after reporting the state conflict:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "conflicting states", "repo-a", "repo-b")
	// The wildcard-resolved skill proves repositories were acquired and
	// resolved: the fail-fast path never performs any action.
	assertOutputContains(t, output.String(), "solo (installed)")
	assertFileExists(t, filepath.Join(root, ".agents", "skills", "solo", "SKILL.md"))
	assertLinkTarget(t, filepath.Join(target, "solo"), filepath.Join(root, ".agents", "skills", "solo"))
	// The conflicting name fails in the post-resolution summary; the final
	// "Completed" line is printed only by the resolve-then-report flow.
	assertOutputContains(t, output.String(), "shared (failed:", "Completed 3 operation(s); 2 failed.")
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestApplySameNamedInstalledEntriesResolveBeforeConflictDecision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	target := filepath.Join(root, "agent", "skills")
	// Both repositories declare "shared" as installed. "ghost" does not exist
	// in repo-a, so its resolution failure in the summary proves that
	// resolution ran before the content conflict was decided.
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n      - name: ghost\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatalf("apply should fail after resolving differing content:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "content differs between repositories", "shared (failed:")
	// The resolution failure proves the resolve phase ran before any conflict
	// decision, and the final summary line proves the flow passed fail-fast.
	assertOutputContains(t, output.String(), "ghost (failed:", "Completed 3 operation(s); 3 failed.")
	assertPathMissing(t, filepath.Join(root, ".agents", "skills", "shared"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}

func TestApplyStateConflictDoesNotRemoveStoredSkill(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	changeDirectory(t, root)

	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	writeFile(t, filepath.Join(repoA, "skills", "shared", "SKILL.md"), "# From A\n")
	writeFile(t, filepath.Join(repoB, "skills", "shared", "SKILL.md"), "# From B\n")
	target := filepath.Join(root, "agent", "skills")
	storePath := filepath.Join(root, ".agents", "skills", "shared")
	// Simulate a previously installed skill; one repository now marks it
	// uninstalled while the other still marks it installed.
	writeFile(t, filepath.Join(storePath, "SKILL.md"), "# Stored\n")
	writeFile(t, filepath.Join(root, "config.yaml"), "dirs:\n  pi: "+target+"\nskills:\n  - repo: "+repoA+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n        state: uninstalled\n  - repo: "+repoB+"\n    provider:\n      type: local\n    skills:\n      - name: shared\n")

	app := New("dev", "test")
	var output bytes.Buffer
	app.Writer = &output
	if err := app.Run([]string{"skill", "apply", "--yes", "--agents", "pi"}); err == nil {
		t.Fatalf("apply should fail on conflicting states:\n%s", output.String())
	}
	assertOutputContains(t, output.String(), "Skill conflicts", "conflicting states")
	// The stored skill must survive: no repository's request may win silently.
	assertFileExists(t, filepath.Join(storePath, "SKILL.md"))
	assertPathMissing(t, filepath.Join(target, "shared"))
}
