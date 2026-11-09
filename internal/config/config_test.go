package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateProviderType(t *testing.T) {
	c := &Config{Dirs: map[string]string{}, Skills: []Repository{{Repo: "r", Provider: &Provider{Type: "gitlab"}}}}
	if err := Validate(c); err == nil {
		t.Fatal("expected error")
	}
}

func TestBaseURLGithubRejected(t *testing.T) {
	c := &Config{Dirs: map[string]string{}, Skills: []Repository{{Repo: "r", Provider: &Provider{Type: "github", BaseURL: "https://x"}}}}
	if err := Validate(c); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteExampleRefusesOverwrite(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg")
	path := filepath.Join(dir, "config.yaml")
	if err := WriteExample(path); err != nil {
		t.Fatal(err)
	}
	if err := WriteExample(path); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestWriteExampleGeneratesValidYAML(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg")
	path := filepath.Join(dir, "config.yaml")
	if err := WriteExample(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("generated example did not parse: %v", err)
	}
	if cfg.Dirs["pi"] == "" {
		t.Error("expected pi dir in generated config")
	}
}
