package app

import (
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

func TestNew_HasVersionFlag(t *testing.T) {
	app := New("1.0.0", "abc123")
	
	versionFlag := false
	for _, flag := range app.Flags {
		if flag.Names()[0] == "version" {
			versionFlag = true
			break
		}
	}
	
	if !versionFlag {
		t.Error("New() should have --version flag")
	}
}
