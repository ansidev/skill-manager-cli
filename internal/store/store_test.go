package store

import (
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
