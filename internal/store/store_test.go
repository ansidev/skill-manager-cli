package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSkillCopiesMissingSource(t *testing.T) {
	source := writeSkill(t, t.TempDir(), "source", map[string]string{
		"SKILL.md": "# Example\n",
		"bin/run":  "#!/bin/sh\necho run\n",
	})
	destination := filepath.Join(t.TempDir(), "store", "example")

	changed, err := StoreSkill(source, destination)
	if err != nil {
		t.Fatalf("StoreSkill() error = %v", err)
	}
	if !changed {
		t.Fatal("StoreSkill() changed = false, want true")
	}
	assertSkillContents(t, destination, map[string]string{
		"SKILL.md": "# Example\n",
		"bin/run":  "#!/bin/sh\necho run\n",
	})
}

func TestStoreSkillSkipsIdenticalDestination(t *testing.T) {
	root := t.TempDir()
	source := writeSkill(t, root, "source", map[string]string{"SKILL.md": "# Example\n"})
	destination := writeSkill(t, root, "store/example", map[string]string{"SKILL.md": "# Example\n"})

	changed, err := StoreSkill(source, destination)
	if err != nil {
		t.Fatalf("StoreSkill() error = %v", err)
	}
	if changed {
		t.Fatal("StoreSkill() changed = true, want false for identical directories")
	}
}

func TestStoreSkillReplacesChangedDestination(t *testing.T) {
	root := t.TempDir()
	source := writeSkill(t, root, "source", map[string]string{
		"SKILL.md": "# New\n",
		"bin/run":  "new\n",
	})
	destination := writeSkill(t, root, "store/example", map[string]string{
		"SKILL.md":  "# Old\n",
		"stale.txt": "remove me\n",
	})

	changed, err := StoreSkill(source, destination)
	if err != nil {
		t.Fatalf("StoreSkill() error = %v", err)
	}
	if !changed {
		t.Fatal("StoreSkill() changed = false, want true")
	}
	assertSkillContents(t, destination, map[string]string{
		"SKILL.md": "# New\n",
		"bin/run":  "new\n",
	})
	if _, err := os.Lstat(filepath.Join(destination, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file error = %v, want not exist", err)
	}
}

func writeSkill(t *testing.T, root, name string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(root, name)
	for relativePath, contents := range files {
		filePath := filepath.Join(path, relativePath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filePath, err)
		}
		if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", filePath, err)
		}
	}
	return path
}

func assertSkillContents(t *testing.T, path string, want map[string]string) {
	t.Helper()
	for relativePath, wantContents := range want {
		got, err := os.ReadFile(filepath.Join(path, relativePath))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", relativePath, err)
		}
		if string(got) != wantContents {
			t.Errorf("contents of %q = %q, want %q", relativePath, got, wantContents)
		}
	}
}

func TestLinkSkillCreatesAndPreservesManagedLink(t *testing.T) {
	root := t.TempDir()
	storePath := writeSkill(t, root, "store/example", map[string]string{"SKILL.md": "# Example\n"})
	linkPath := filepath.Join(root, "agent", "skills", "example")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}

	changed, err := LinkSkill(storePath, linkPath)
	if err != nil {
		t.Fatalf("LinkSkill() error = %v", err)
	}
	if !changed {
		t.Fatal("LinkSkill() changed = false, want true")
	}
	assertLinkTarget(t, linkPath, storePath)

	changed, err = LinkSkill(storePath, linkPath)
	if err != nil {
		t.Fatalf("LinkSkill() on managed link error = %v", err)
	}
	if changed {
		t.Fatal("LinkSkill() changed = true, want false for matching link")
	}
}

func TestLinkSkillReplacesDifferentSymlink(t *testing.T) {
	root := t.TempDir()
	storePath := writeSkill(t, root, "store/example", map[string]string{"SKILL.md": "# Example\n"})
	otherPath := writeSkill(t, root, "other/example", map[string]string{"SKILL.md": "# Other\n"})
	linkPath := filepath.Join(root, "agent", "skills", "example")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherPath, linkPath); err != nil {
		t.Fatal(err)
	}

	changed, err := LinkSkill(storePath, linkPath)
	if err != nil {
		t.Fatalf("LinkSkill() error = %v", err)
	}
	if !changed {
		t.Fatal("LinkSkill() changed = false, want true")
	}
	assertLinkTarget(t, linkPath, storePath)
}

func TestLinkSkillRejectsNonSymlinkDestination(t *testing.T) {
	root := t.TempDir()
	storePath := writeSkill(t, root, "store/example", map[string]string{"SKILL.md": "# Example\n"})
	linkPath := filepath.Join(root, "agent", "skills", "example")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("user-owned"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := LinkSkill(storePath, linkPath)
	if changed {
		t.Fatal("LinkSkill() changed = true, want false for conflict")
	}
	if !errors.Is(err, ErrPathConflict) {
		t.Fatalf("LinkSkill() error = %v, want ErrPathConflict", err)
	}
	contents, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "user-owned" {
		t.Errorf("conflicting file contents = %q, want preserved contents", contents)
	}
}

func assertLinkTarget(t *testing.T, linkPath, wantTarget string) {
	t.Helper()
	gotTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%q): %v", linkPath, err)
	}
	if gotTarget != wantTarget {
		t.Errorf("link target = %q, want %q", gotTarget, wantTarget)
	}
}
