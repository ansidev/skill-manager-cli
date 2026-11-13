package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableSkillLifecycle(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "skill-cli")
	build := exec.Command("go", "build", "-o", binary, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	repo := filepath.Join(root, "repo")
	universal := filepath.Join(root, "universal")
	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	writeFile(t, filepath.Join(repo, "skills", "demo", "SKILL.md"), "# demo\n")
	configPath := filepath.Join(root, "config.yaml")
	config := func(state string) {
		writeFile(t, configPath, fmt.Sprintf("dirs:\n  one: %s\n  two: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: demo\n        state: %s\n", one, two, repo, state))
	}
	run := func(success bool, args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+filepath.Join(root, "home"), "SKILL_MANAGER_STORE_DIR="+universal)
		out, err := cmd.CombinedOutput()
		if (err == nil) != success {
			t.Fatalf("%v: err=%v output=%s", args, err, out)
		}
		return string(out)
	}
	stored := filepath.Join(universal, "demo")
	checkInstalled := func() {
		t.Helper()
		assertFileExists(t, filepath.Join(stored, "SKILL.md"))
		for _, target := range []string{one, two} {
			assertLinkTarget(t, filepath.Join(target, "demo"), stored)
		}
	}
	checkRemoved := func() {
		t.Helper()
		assertPathMissing(t, stored)
		for _, target := range []string{one, two} {
			assertPathMissing(t, filepath.Join(target, "demo"))
		}
	}
	config("installed")
	assertOutputContains(t, run(true, "install", "-y"), "demo (installed)")
	checkInstalled()
	assertOutputContains(t, run(false, "uninstall", "-y", "--agents", "one"), filepath.Join(two, "demo"))
	checkInstalled()
	assertOutputContains(t, run(true, "uninstall", "-y"), "demo (uninstalled)")
	checkRemoved()
	assertOutputContains(t, run(true, "apply", "-y"), "demo (installed)")
	checkInstalled()
	config("uninstalled")
	assertOutputContains(t, run(true, "apply", "-y"), "demo (uninstalled)")
	checkRemoved()
	for _, command := range []string{"uninstall", "apply"} {
		t.Run("orphan-store-"+command, func(t *testing.T) {
			writeFile(t, filepath.Join(stored, "SKILL.md"), "# orphan\n")
			output := run(true, command, "-y")
			assertOutputContains(t, output, "demo (uninstalled)")
			checkRemoved()
		})
	}
	config("installed")
	writeFile(t, filepath.Join(two, "demo", "user-file"), "keep")
	for _, command := range []string{"install", "apply"} {
		t.Run("link-conflict-"+command, func(t *testing.T) {
			output := run(false, command, "-y")
			assertOutputContains(t, output, "demo (failed:", filepath.Join(two, "demo"))
			if strings.Contains(output, "demo (installed)") {
				t.Fatalf("misleading success: %s", output)
			}
			assertFileExists(t, filepath.Join(two, "demo", "user-file"))
		})
	}
	config("uninstalled")
	for _, command := range []string{"uninstall", "apply"} {
		t.Run("unlink-conflict-"+command, func(t *testing.T) {
			output := run(false, command, "-y")
			assertOutputContains(t, output, "demo (failed:", filepath.Join(two, "demo"))
			assertFileExists(t, filepath.Join(two, "demo", "user-file"))
			assertFileExists(t, filepath.Join(stored, "SKILL.md"))
		})
	}
}
