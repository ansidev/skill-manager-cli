// Package source resolves repository locations for different providers and
// obtains local workspaces, cloning remote repositories on demand.
package source

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/config"
)

// Workspace is a local directory containing repository content. For non-local
// repositories Cleanup removes the managed clone.
type Workspace struct {
	Path    string
	Cleanup func() error
}

// ResolvedSkill is a configured skill entry resolved to a directory inside a
// repository workspace.
type ResolvedSkill struct {
	Entry config.SkillEntry
	Path  string
}

// SelectionFailure records one configured skill entry that could not be
// resolved. A failure for one entry does not prevent later entries resolving.
type SelectionFailure struct {
	Entry config.SkillEntry
	Err   error
}

// SkillResolution is the result of resolving all configured skill entries.
type SkillResolution struct {
	Skills   []ResolvedSkill
	Failures []SelectionFailure
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

// ResolveSkills resolves configured skill entries within root. Empty base paths
// default to "skills". The wildcard name "*" selects direct child directories
// in lexical order. Invalid entries are collected as failures so independent
// entries can still resolve.
func ResolveSkills(root string, entries []config.SkillEntry) SkillResolution {
	resolution := SkillResolution{}
	lexicalRoot, err := filepath.Abs(root)
	if err != nil {
		for _, entry := range entries {
			resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: fmt.Errorf("make repository root absolute: %w", err)})
		}
		return resolution
	}
	canonicalRoot, err := canonicalDirectory(lexicalRoot)
	if err != nil {
		for _, entry := range entries {
			resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: fmt.Errorf("resolve repository root: %w", err)})
		}
		return resolution
	}

	for _, entry := range entries {
		basePath, err := resolveBasePath(lexicalRoot, canonicalRoot, entry.BasePath)
		if err != nil {
			resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: err})
			continue
		}

		if entry.Name == "*" {
			children, err := os.ReadDir(basePath)
			if err != nil {
				resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: fmt.Errorf("read skill base path: %w", err)})
				continue
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				path, err := resolveDirectory(lexicalRoot, canonicalRoot, filepath.Join(basePath, child.Name()))
				if err != nil {
					failed := entry
					failed.Name = child.Name()
					resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: failed, Err: err})
					continue
				}
				resolved := entry
				resolved.Name = child.Name()
				resolution.Skills = append(resolution.Skills, ResolvedSkill{Entry: resolved, Path: path})
			}
			continue
		}

		if filepath.IsAbs(entry.Name) {
			resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: fmt.Errorf("skill name %q must be relative to repository root", entry.Name)})
			continue
		}
		path, err := resolveDirectory(lexicalRoot, canonicalRoot, filepath.Join(basePath, entry.Name))
		if err != nil {
			resolution.Failures = append(resolution.Failures, SelectionFailure{Entry: entry, Err: err})
			continue
		}
		resolution.Skills = append(resolution.Skills, ResolvedSkill{Entry: entry, Path: path})
	}

	return resolution
}

func resolveBasePath(root, canonicalRoot, base string) (string, error) {
	if base == "" {
		base = "skills"
	}
	if filepath.IsAbs(base) {
		return "", fmt.Errorf("base path %q must be relative to repository root", base)
	}
	return resolveDirectory(root, canonicalRoot, filepath.Join(root, base))
}

func resolveDirectory(root, canonicalRoot, path string) (string, error) {
	canonicalPath, err := canonicalDirectory(path)
	if err != nil {
		return "", err
	}
	if !isWithin(canonicalRoot, canonicalPath) {
		return "", fmt.Errorf("path %q resolves outside repository root", path)
	}
	return canonicalPath, nil
}

func canonicalDirectory(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", absolutePath, err)
	}
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return "", fmt.Errorf("inspect path %q: %w", canonicalPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", canonicalPath)
	}
	return canonicalPath, nil
}

func isWithin(root, path string) bool {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) && !filepath.IsAbs(relativePath)
}

// bytes import safeguard
var _ = bytes.Buffer{}
