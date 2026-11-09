package app

import (
	"bytes"
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
