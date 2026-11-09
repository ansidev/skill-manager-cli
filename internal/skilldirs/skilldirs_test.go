package skilldirs

import (
	"reflect"
	"testing"

	"github.com/ansidev/skill-manager-cli/internal/config"
)

func TestResolveExplicitDirsTakePrecedence(t *testing.T) {
	cfg := &config.Config{Dirs: map[string]string{"pi": "/configured/pi"}}
	got, err := Resolve(" /a , /b ,/a ", "pi", cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := []string{"/a", "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %v, want %v", got, want)
	}
}

func TestResolveRejectsEmptyExplicitEntry(t *testing.T) {
	if _, err := Resolve("/a,,/b", "", &config.Config{}); err == nil {
		t.Fatal("Resolve() with empty entry returned nil error")
	}
}

func TestResolveSelectedAgents(t *testing.T) {
	cfg := &config.Config{Dirs: map[string]string{
		"pi":    "/skills/pi",
		"codex": "/skills/codex",
	}}
	got, err := Resolve("", "codex, pi", cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := []string{"/skills/codex", "/skills/pi"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %v, want %v", got, want)
	}
}

func TestResolveUnknownAgentFails(t *testing.T) {
	cfg := &config.Config{Dirs: map[string]string{"pi": "/skills/pi"}}
	if _, err := Resolve("", "nope", cfg); err == nil {
		t.Fatal("Resolve() with unknown agent returned nil error")
	}
}

func TestResolveAllConfiguredDirsWhenNoSelection(t *testing.T) {
	cfg := &config.Config{Dirs: map[string]string{
		"zeta":  "/skills/zeta",
		"alpha": "/skills/alpha",
		"blank": "  ",
	}}
	got, err := Resolve("", "", cfg)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := []string{"/skills/alpha", "/skills/zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %v, want sorted configured dirs %v", got, want)
	}
}

func TestResolveHandlesNilConfig(t *testing.T) {
	got, err := Resolve("", "", nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Resolve() = %v, want empty", got)
	}
}
