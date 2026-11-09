// Package source resolves repository locations for different providers and
// obtains local workspaces, cloning remote repositories on demand.
package source

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/config"
)

// Workspace is a local directory containing repository content. For non-local
// repositories Cleanup removes the managed clone.
type Workspace struct {
	Path    string
	Cleanup func() error
}

// ResolvedSource describes the resolved location for a repository.
type ResolvedSource struct {
	Provider config.Provider
	Repo     string
	URL      string // git/remote URL or filesystem path for local providers
	Local    bool
}

// Resolver abstracts command execution so URL/clone logic is testable.
type Resolver struct {
	Runner Runner
}

// Runner executes external commands.
type Runner interface {
	Run(name string, args ...string) (stdout, stderr []byte, err error)
}

// DefaultRunner uses os/exec.
type DefaultRunner struct{}

func (DefaultRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return out.Bytes(), errBuf.Bytes(), err
}

// NewResolver constructs a resolver with the default runner.
func NewResolver() *Resolver { return &Resolver{Runner: DefaultRunner{}} }

func resolver(r *Resolver) Runner {
	if r == nil || r.Runner == nil {
		return DefaultRunner{}
	}
	return r.Runner
}

// ResolveURL constructs the repository URL for the given provider. Local
// providers return the filesystem path unchanged.
func ResolveURL(repo string, provider config.Provider) string {
	if provider.Type == "local" {
		return repo
	}
	base := provider.BaseURL
	if provider.Type == "github" && base == "" {
		base = "https://github.com"
	}
	base = strings.TrimRight(base, "/")
	repo = strings.TrimLeft(repo, "/")
	if base == "" {
		return repo
	}
	return base + "/" + repo
}

// ResolveSource resolves a repository into a ResolvedSource.
func ResolveSource(repo string, provider config.Provider) ResolvedSource {
	provider.Normalize()
	return ResolvedSource{
		Provider: provider,
		Repo:     repo,
		URL:      ResolveURL(repo, provider),
		Local:    provider.Type == "local",
	}
}

// Resolve helpers/aliases.
func Resolve(repo string, provider config.Provider) ResolvedSource {
	return ResolveSource(repo, provider)
}

// Clone clones url into dest using git clone --depth 1. It returns an error
// retaining command context on failure.
func (r *Resolver) Clone(url, dest string) error {
	runner := resolver(r)
	_, stderr, err := runner.Run("git", "clone", "--depth", "1", url, dest)
	if err != nil {
		return fmt.Errorf("git clone --depth 1 %s %s: %w: %s", url, dest, err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

func Clone(url, dest string) error {
	return NewResolver().Clone(url, dest)
}

// Obtain returns a workspace for the repository. Local repositories are used
// in place without cloning; remote repositories are cloned into a managed
// temporary directory.
func (r *Resolver) Obtain(repo string, provider config.Provider) (*Workspace, error) {
	provider.Normalize()
	resolved := ResolveSource(repo, provider)
	if resolved.Local {
		return &Workspace{Path: resolved.URL, Cleanup: func() error { return nil }}, nil
	}
	tempRoot, err := os.MkdirTemp("", "skill-manager-repo-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary directory: %w", err)
	}
	if err := r.Clone(resolved.URL, tempRoot); err != nil {
		os.RemoveAll(tempRoot)
		return nil, err
	}
	return &Workspace{Path: tempRoot, Cleanup: func() error {
		return os.RemoveAll(tempRoot)
	}}, nil
}

func Obtain(repo string, provider config.Provider) (*Workspace, error) {
	return NewResolver().Obtain(repo, provider)
}

// bytes import safeguard
var _ = bytes.Buffer{}
