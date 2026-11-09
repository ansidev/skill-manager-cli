// Package targetdirs resolves the set of agent skill directories a command
// should operate on from the shared --skill-dirs and -a/--agents flags.
package skilldirs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ansidev/skill-manager-cli/internal/config"
)

// Resolve returns the normalized, deduplicated list of skill directories.
// --skill-dirs takes precedence; otherwise selected agent identifiers are
// resolved from the config's Dirs map. With no agents supplied every
// configured directory is selected.
func Resolve(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.Dirs == nil {
		cfg.Dirs = map[string]string{}
	}

	// Explicit directories take precedence.
	if strings.TrimSpace(skillDirsFlag) != "" {
		raw := strings.Split(skillDirsFlag, ",")
		paths := make([]string, 0, len(raw))
		for _, p := range raw {
			p = strings.TrimSpace(p)
			if p == "" {
				return nil, fmt.Errorf("invalid empty path entry")
			}
			p = expandHome(p)
			paths = append(paths, p)
		}
		return dedup(paths), nil
	}

	agents := splitComma(agentsFlag)
	if len(agents) == 0 {
		// select all configured directories, predictably ordered by key.
		keys := sortedKeys(cfg.Dirs)
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			if v := strings.TrimSpace(cfg.Dirs[k]); v != "" {
				out = append(out, expandHome(v))
			}
		}
		return dedup(out), nil
	}

	out := make([]string, 0, len(agents))
	for _, a := range agents {
		dir, ok := cfg.Dirs[a]
		if !ok {
			return nil, fmt.Errorf("unknown agent: %s", a)
		}
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return nil, fmt.Errorf("agent %q has empty skill directory", a)
		}
		out = append(out, expandHome(dir))
	}
	return dedup(out), nil
}

// Aliases for compatibility with various callers.
func ResolveSkillDirs(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	return Resolve(skillDirsFlag, agentsFlag, cfg)
}
func ResolveTargetSkillDirs(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	return Resolve(skillDirsFlag, agentsFlag, cfg)
}
func TargetSkillDirs(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	return Resolve(skillDirsFlag, agentsFlag, cfg)
}
func ParseSkillDirs(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	return Resolve(skillDirsFlag, agentsFlag, cfg)
}
func ResolveDirs(skillDirsFlag, agentsFlag string, cfg *config.Config) ([]string, error) {
	return Resolve(skillDirsFlag, agentsFlag, cfg)
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			// reject empty path entries only for explicit dirs; for agents empty ignored
			// We'll filter here; empty entries in agents handled by skip.
			continue
		}
		out = append(out, p)
	}
	return out
}

// normalizePaths rejects empty entries (already filtered) and dedups.
func normalizePaths(paths []string) ([]string, error) {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, fmt.Errorf("invalid empty path entry")
		}
	}
	return dedup(paths), nil
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
