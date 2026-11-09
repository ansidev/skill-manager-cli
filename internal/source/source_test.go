package source

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ansidev/skill-manager-cli/internal/config"
)

func TestResolveSkillsFixtures(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "repository"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		entries    []config.SkillEntry
		wantPaths  []string
		wantFailed []string
	}{
		{
			name:      "named skill uses default base path",
			entries:   []config.SkillEntry{{Name: "alpha"}},
			wantPaths: []string{filepath.Join(root, "skills", "alpha")},
		},
		{
			name:      "named skill uses custom base path",
			entries:   []config.SkillEntry{{Name: "gamma", BasePath: "custom"}},
			wantPaths: []string{filepath.Join(root, "custom", "gamma")},
		},
		{
			name:      "wildcard selects immediate directories in lexical order",
			entries:   []config.SkillEntry{{Name: "*"}},
			wantPaths: []string{filepath.Join(root, "skills", "alpha"), filepath.Join(root, "skills", "beta")},
		},
		{
			name:       "missing selections do not prevent valid selections",
			entries:    []config.SkillEntry{{Name: "alpha"}, {Name: "missing"}, {Name: "beta"}},
			wantPaths:  []string{filepath.Join(root, "skills", "alpha"), filepath.Join(root, "skills", "beta")},
			wantFailed: []string{"missing"},
		},
		{
			name:       "non-directory base path is a failure",
			entries:    []config.SkillEntry{{Name: "alpha", BasePath: "skills/readme.txt"}},
			wantFailed: []string{"alpha"},
		},
		{
			name:       "base path traversal is a failure",
			entries:    []config.SkillEntry{{Name: "alpha", BasePath: "../outside"}},
			wantFailed: []string{"alpha"},
		},
		{
			name:       "skill name traversal is a failure",
			entries:    []config.SkillEntry{{Name: "../outside"}},
			wantFailed: []string{"../outside"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveSkills(root, tt.entries)
			if paths := resolvedPaths(got.Skills); !reflect.DeepEqual(paths, tt.wantPaths) {
				t.Errorf("resolved paths = %v, want %v", paths, tt.wantPaths)
			}
			if failed := failedNames(got.Failures); !reflect.DeepEqual(failed, tt.wantFailed) {
				t.Errorf("failed selections = %v, want %v", failed, tt.wantFailed)
			}
		})
	}
}

func TestResolveSkillsRejectsSymlinkOutsideRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "skills", "escape")); err != nil {
		t.Fatal(err)
	}

	got := ResolveSkills(root, []config.SkillEntry{{Name: "escape"}})
	if len(got.Skills) != 0 {
		t.Errorf("resolved skills = %v, want none", got.Skills)
	}
	if failed := failedNames(got.Failures); !reflect.DeepEqual(failed, []string{"escape"}) {
		t.Errorf("failed selections = %v, want [escape]", failed)
	}
}

func resolvedPaths(skills []ResolvedSkill) []string {
	if len(skills) == 0 {
		return nil
	}
	paths := make([]string, len(skills))
	for i, skill := range skills {
		paths[i] = skill.Path
	}
	return paths
}

func failedNames(failures []SelectionFailure) []string {
	if len(failures) == 0 {
		return nil
	}
	names := make([]string, len(failures))
	for i, failure := range failures {
		names[i] = failure.Entry.Name
	}
	return names
}
