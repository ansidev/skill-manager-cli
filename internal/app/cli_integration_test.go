package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyProcessesAvailableRepositoryWhileCloneIsPending(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "skill-cli")
	build := exec.Command("go", "build", "-o", binary, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fast", "slow"} {
		repo := filepath.Join(root, name)
		writeFile(t, filepath.Join(repo, "skills", name, "SKILL.md"), "# "+name+"\n")
		if name == "fast" {
			writeFile(t, filepath.Join(repo, "old", "retired", "SKILL.md"), "# retired\n")
		}
		for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture"}} {
			cmd := exec.Command(git, args...)
			cmd.Dir = repo
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
	}
	binDir := filepath.Join(root, "bin")
	wrapper := filepath.Join(binDir, "git")
	writeFile(t, wrapper, "#!/bin/sh\ncase \"$4\" in */slow) touch \"$WAITING\"; while [ ! -e \"$RELEASE\" ]; do sleep 0.05; done ;; esac\nexec \"$REAL_GIT\" \"$@\"\n")
	if err := os.Chmod(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(root, "one"), filepath.Join(root, "two")
	writeFile(t, filepath.Join(root, "store", "retired", "SKILL.md"), "# retired\n")
	for _, target := range []string{one, two} {
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "store", "retired"), filepath.Join(target, "retired")); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\n  two: %s\nskills:\n  - repo: %s\n    provider:\n      type: custom\n      base_url: file://%s\n    skills:\n      - name: fast\n      - name: '*'\n        base_path: old\n        state: uninstalled\n  - repo: slow\n    provider:\n      type: custom\n      base_url: file://%s\n    skills:\n      - name: slow\n", one, two, "fast", root, root))
	outputPath := filepath.Join(root, "output")
	output, err := os.Create(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	release := filepath.Join(root, "release")
	waiting := filepath.Join(root, "waiting")
	cmd := exec.Command(binary, "apply", "-y")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"), "REAL_GIT="+git, "WAITING="+waiting, "RELEASE="+release, "SKILL_MANAGER_STORE_DIR="+filepath.Join(root, "store"))
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = os.WriteFile(release, nil, 0o600)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, waitingErr := os.Stat(waiting)
		_, oneErr := os.Lstat(filepath.Join(one, "fast"))
		_, twoErr := os.Lstat(filepath.Join(two, "fast"))
		data, _ := os.ReadFile(outputPath)
		_, removedErr := os.Lstat(filepath.Join(root, "store", "retired"))
		if waitingErr == nil && oneErr == nil && twoErr == nil && os.IsNotExist(removedErr) && strings.Contains(string(data), "Install skill: fast") && strings.Contains(string(data), "Uninstall skill: retired") {
			break
		}
		select {
		case err := <-done:
			finished = true
			t.Fatalf("command finished while clone should be blocked: %v\n%s", err, data)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("fast install or live progress blocked by slow clone: %s", data)
		}
		time.Sleep(25 * time.Millisecond)
	}
	assertLinkTarget(t, filepath.Join(one, "fast"), filepath.Join(root, "store", "fast"))
	assertLinkTarget(t, filepath.Join(two, "fast"), filepath.Join(root, "store", "fast"))
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err = <-done
	finished = true
	if err != nil {
		data, _ := os.ReadFile(outputPath)
		t.Fatalf("apply: %v\n%s", err, data)
	}
	assertLinkTarget(t, filepath.Join(one, "slow"), filepath.Join(root, "store", "slow"))
	assertLinkTarget(t, filepath.Join(two, "slow"), filepath.Join(root, "store", "slow"))
}

func TestExecutableRetainsEverySymlinkTask(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "skill-cli")
	build := exec.Command("go", "build", "-o", binary, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	repoA, repoB := filepath.Join(root, "repo-a"), filepath.Join(root, "repo-b")
	for _, fixture := range []struct{ repo, skill string }{{repoA, "alpha"}, {repoA, "beta"}, {repoB, "gamma"}} {
		writeFile(t, filepath.Join(fixture.repo, "skills", fixture.skill, "SKILL.md"), "# "+fixture.skill+"\n")
	}
	for _, command := range []string{"install", "apply"} {
		t.Run(command, func(t *testing.T) {
			work := filepath.Join(root, command)
			one, two, three := filepath.Join(work, "one"), filepath.Join(work, "two"), filepath.Join(work, "three")
			store := filepath.Join(work, "store")
			writeFile(t, filepath.Join(work, "config.yaml"), fmt.Sprintf("dirs:\n  one: %s\n  two: %s\n  three: %s\nskills:\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: alpha\n      - name: beta\n  - repo: %s\n    provider:\n      type: local\n    skills:\n      - name: gamma\n", one, two, three, repoA, repoB))
			args := []string{command, "-y"}
			skills := []string{"alpha", "beta", "gamma"}
			if command == "install" {
				args = append(args, "--repo", repoA, "--provider-type", "local", "--skill", "*")
				skills = []string{"alpha", "beta"}
			}
			cmd := exec.Command(binary, args...)
			cmd.Dir = work
			cmd.Env = append(os.Environ(), "HOME="+filepath.Join(work, "home"), "SKILL_MANAGER_STORE_DIR="+store)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", command, err, output)
			}
			for _, skill := range skills {
				section := strings.SplitN(string(output), "      - "+skill+" (installed)\n", 2)
				if len(section) != 2 {
					t.Fatalf("missing skill in final report: %s", output)
				}
				details := strings.SplitN(section[1], "\n      - ", 2)[0]
				for _, dir := range []string{one, two, three} {
					link := filepath.Join(dir, skill)
					assertLinkTarget(t, link, filepath.Join(store, skill))
					assertOutputContains(t, details, "symlink created: "+link)
				}
			}
		})
	}
}

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
	dryRunOutput := run(true, "install", "--dry-run")
	if strings.Contains(dryRunOutput, "symlink created:") || strings.Contains(dryRunOutput, "symlink deleted:") {
		t.Fatalf("dry run reported modified links: %s", dryRunOutput)
	}
	assertPathMissing(t, stored)
	installOutput := run(true, "install", "-y")
	assertOutputContains(t, installOutput, "demo (installed)", "symlink created: "+filepath.Join(one, "demo"), "symlink created: "+filepath.Join(two, "demo"))
	if !strings.Contains(installOutput, "\n\n\x1b[1;36mInstalled skills\x1b[0m") {
		t.Fatalf("summary heading needs highlighting and a preceding blank line: %q", installOutput)
	}
	checkInstalled()
	skippedOutput := run(true, "install", "-y")
	assertOutputContains(t, skippedOutput, "demo (skipped)")
	if strings.Contains(skippedOutput, "symlink created:") || strings.Contains(skippedOutput, "symlink deleted:") {
		t.Fatalf("unchanged links reported as modified: %s", skippedOutput)
	}
	assertOutputContains(t, run(false, "uninstall", "-y", "--agents", "one"), filepath.Join(two, "demo"))
	checkInstalled()
	uninstallDirectOutput := run(true, "uninstall", "-y")
	assertOutputContains(t, uninstallDirectOutput, "demo (uninstalled)", "symlink deleted: "+filepath.Join(one, "demo"), "symlink deleted: "+filepath.Join(two, "demo"))
	checkRemoved()
	applyOutput := run(true, "apply", "-y")
	assertOutputContains(t, applyOutput, "demo (installed)", "\n\x1b[1;36mSkills to install\x1b[0m")
	checkInstalled()
	config("uninstalled")
	uninstallOutput := run(true, "apply", "-y")
	assertOutputContains(t, uninstallOutput, "demo (uninstalled)", "symlink deleted: "+filepath.Join(one, "demo"), "symlink deleted: "+filepath.Join(two, "demo"), "\n\x1b[1;36mSkills to uninstall\x1b[0m", "\n\n\x1b[1;36mUninstalled skills\x1b[0m")
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
			if command == "install" {
				assertOutputContains(t, output, "symlink created: "+filepath.Join(one, "demo"))
			}
			if strings.Contains(output, "symlink created: "+filepath.Join(two, "demo")) {
				t.Fatalf("conflicting symlink reported as completed: %s", output)
			}
			if strings.Contains(output, "demo (installed)") {
				t.Fatalf("misleading success: %s", output)
			}
			assertFileExists(t, filepath.Join(two, "demo", "user-file"))
		})
	}
	config("uninstalled")
	t.Run("unlink-conflict-uninstall", func(t *testing.T) {
		output := run(true, "uninstall", "-y")
		assertOutputContains(t, output, "demo (uninstalled)", "symlink deleted: "+filepath.Join(one, "demo"))
		if strings.Contains(output, "symlink deleted: "+filepath.Join(two, "demo")) {
			t.Fatalf("unmanaged path reported as deleted: %s", output)
		}
		assertFileExists(t, filepath.Join(two, "demo", "user-file"))
		assertPathMissing(t, filepath.Join(one, "demo"))
		assertPathMissing(t, stored)
	})
	t.Run("unlink-conflict-apply", func(t *testing.T) {
		output := run(true, "apply", "-y")
		assertOutputContains(t, output, "demo (skipped)")
		assertFileExists(t, filepath.Join(two, "demo", "user-file"))
		assertPathMissing(t, filepath.Join(one, "demo"))
		assertPathMissing(t, stored)
	})
}
