package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableDirectRepositorySelection(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	repo, store := filepath.Join(root, "repo"), filepath.Join(root, "store")
	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	writeFile(t, filepath.Join(repo, "custom", "alpha", "SKILL.md"), "alpha")
	writeFile(t, filepath.Join(repo, "skills", "beta", "SKILL.md"), "beta")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\n  two: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: alpha\n        base_path: custom\n      - name: beta\n", one, two, repo))
	run := func(success bool, args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+store)
		out, err := cmd.CombinedOutput()
		if (err == nil) != success {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	output := run(true, "install", "-y", "--repo", repo)
	for _, skill := range []string{"alpha", "beta"} {
		assertOutputContains(t, output, skill+" (installed)")
		assertFileExists(t, filepath.Join(store, skill, "SKILL.md"))
		assertLinkTarget(t, filepath.Join(one, skill), filepath.Join(store, skill))
		assertLinkTarget(t, filepath.Join(two, skill), filepath.Join(store, skill))
	}
	output = run(true, "uninstall", "-y", "--repo", repo)
	for _, skill := range []string{"alpha", "beta"} {
		assertOutputContains(t, output, skill+" (uninstalled)")
		assertPathMissing(t, filepath.Join(store, skill))
		assertPathMissing(t, filepath.Join(one, skill))
		assertPathMissing(t, filepath.Join(two, skill))
	}
	output = run(true, "install", "-y", "--repo", repo, "--skill", "alpha")
	assertOutputContains(t, output, "alpha (installed)")
	assertFileExists(t, filepath.Join(store, "alpha", "SKILL.md"))
	assertPathMissing(t, filepath.Join(store, "beta"))
	output = run(true, "uninstall", "-y", "--repo", repo, "--skill", "*", "--skill-base-path", "custom")
	assertOutputContains(t, output, "alpha (uninstalled)")
	assertPathMissing(t, filepath.Join(store, "alpha"))
	writeFile(t, filepath.Join(two, "beta", "user-file"), "keep")
	output = run(false, "install", "-y", "--repo", repo)
	assertOutputContains(t, output, "alpha (installed)", "beta (failed:", filepath.Join(two, "beta"))
	if strings.Contains(output, "beta (installed)") {
		t.Fatalf("reported partial failure as success: %s", output)
	}
	assertFileExists(t, filepath.Join(two, "beta", "user-file"))
	assertFileExists(t, filepath.Join(store, "alpha", "SKILL.md"))
}

func TestExecutableReportsInvalidSelectionWithoutConfirmation(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	repo := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(repo, "skills", "placeholder", "SKILL.md"), "placeholder")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: missing\n", filepath.Join(root, "target"), repo))
	cmd := exec.Command(binary, "install")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+filepath.Join(root, "store"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("missing skill should fail: %s", out)
	}
	assertOutputContains(t, string(out), "missing (failed:")
}

func TestExecutableRejectsAmbiguousNamedSkill(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	repo := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(repo, "one", "alpha", "SKILL.md"), "one")
	writeFile(t, filepath.Join(repo, "two", "alpha", "SKILL.md"), "two")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: alpha\n        base_path: one\n      - name: alpha\n        base_path: two\n", filepath.Join(root, "target"), repo))
	cmd := exec.Command(binary, "install", "-y", "--skill", "alpha")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+filepath.Join(root, "store"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ambiguous name should fail: %s", out)
	}
	assertOutputContains(t, string(out), "alpha", "ambiguous")
	assertPathMissing(t, filepath.Join(root, "store", "alpha"))
}

func TestExecutableReportsAllSkillsWhenSourceUnavailable(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: missing\n    provider:\n      type: custom\n      base_url: file://%s\n    skills:\n      - name: alpha\n      - name: beta\n", filepath.Join(root, "target"), root))
	cmd := exec.Command(binary, "install", "-y")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+filepath.Join(root, "store"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("missing source should fail: %s", out)
	}
	assertOutputContains(t, string(out), "alpha (failed:", "beta (failed:")
}

func TestExecutableInstallContinuesAfterInvalidSelection(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	repo, store, target := filepath.Join(root, "repo"), filepath.Join(root, "store"), filepath.Join(root, "target")
	writeFile(t, filepath.Join(repo, "skills", "alpha", "SKILL.md"), "alpha")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: ../bad\n      - name: alpha\n", target, repo))
	cmd := exec.Command(binary, "install", "-y")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+store)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid selection should fail: %s", out)
	}
	assertOutputContains(t, string(out), "../bad (failed:", "alpha (installed)")
	assertFileExists(t, filepath.Join(store, "alpha", "SKILL.md"))
}

func TestExecutableNamedUninstallWithMultipleRepositories(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	store, target := filepath.Join(root, "store"), filepath.Join(root, "target")
	writeFile(t, filepath.Join(store, "alpha", "SKILL.md"), "alpha")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: a\n  - repo: b\n", target))
	cmd := exec.Command(binary, "uninstall", "-y", "--skill", "alpha")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+store)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("named uninstall: %v: %s", err, out)
	}
	assertOutputContains(t, string(out), "alpha (uninstalled)")
	assertPathMissing(t, filepath.Join(store, "alpha"))
}

func TestExecutableReportsSelectionErrors(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	cmd := exec.Command(binary, "install", "-y", "--skill-dirs", filepath.Join(root, "target"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("missing repository should fail: %s", out)
	}
	assertOutputContains(t, string(out), "repository is required")
}

func TestExecutableRejectsConflictingSkillSources(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	repo, store, target := filepath.Join(root, "repo"), filepath.Join(root, "store"), filepath.Join(root, "target")
	writeFile(t, filepath.Join(repo, "one", "alpha", "SKILL.md"), "one")
	writeFile(t, filepath.Join(repo, "two", "alpha", "SKILL.md"), "two")
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: alpha\n        base_path: one\n      - name: '*'\n        base_path: two\n", target, repo))
	cmd := exec.Command(binary, "install", "-y", "--overwrite")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+store)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("conflicting sources should fail: %s", out)
	}
	assertOutputContains(t, string(out), "alpha", "conflict")
	assertPathMissing(t, filepath.Join(store, "alpha"))
}

func TestExecutableUninstallContinuesAfterWildcardSourceFailure(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "cli")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	store, target := filepath.Join(root, "store"), filepath.Join(root, "target")
	writeFile(t, filepath.Join(store, "alpha", "SKILL.md"), "alpha")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(store, "alpha"), filepath.Join(target, "alpha")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\nskills:\n  - repo: missing\n    provider:\n      type: custom\n      base_url: file://%s\n    skills:\n      - name: alpha\n      - name: '*'\n", target, root))
	cmd := exec.Command(binary, "uninstall", "-y")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+store)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failed source acquisition: %s", out)
	}
	assertOutputContains(t, string(out), "alpha (uninstalled)", "* (failed:")
	assertPathMissing(t, filepath.Join(store, "alpha"))
	assertPathMissing(t, filepath.Join(target, "alpha"))
}
