package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Provider describes how a repository is fetched.
type Provider struct {
	Type    string `yaml:"type"`
	BaseURL string `yaml:"base_url"`
}

// SkillEntry names one skill inside a repository, where to find it, and whether
// it should be installed or uninstalled.
type SkillEntry struct {
	Name     string `yaml:"name"`
	BasePath string `yaml:"base_path"`
	State    string `yaml:"state"`
}

// Repository is one source of skills: the repo location, how to fetch it, and
// which skills from it to manage.
type Repository struct {
	Repo     string       `yaml:"repo"`
	Provider *Provider    `yaml:"provider"`
	Skills   []SkillEntry `yaml:"skills"`
}

// Config is the top-level config.yaml schema.
type Config struct {
	Dirs   map[string]string `yaml:"dirs"`
	Skills []Repository      `yaml:"skills"`
}

// DefaultProvider returns the implicit provider when none is declared.
func DefaultProvider() Provider { return Provider{Type: "github", BaseURL: "https://github.com"} }

// Normalize fills in defaults for a provider: type defaults to github, and the
// github base URL defaults to https://github.com.
func (p *Provider) Normalize() {
	if p == nil {
		return
	}
	if p.Type == "" {
		p.Type = "github"
	}
	if p.Type == "github" && p.BaseURL == "" {
		p.BaseURL = "https://github.com"
	}
}

// EffectiveProvider returns the repository's provider with defaults applied.
func (r Repository) EffectiveProvider() Provider {
	p := r.Provider
	if p == nil {
		d := DefaultProvider()
		return d
	}
	p.Normalize()
	return *p
}

// Validate checks provider type/base_url rules and state values, mutating only
// to fill in nil dirs.
func Validate(c *Config) error {
	if c.Dirs == nil {
		c.Dirs = map[string]string{}
	}
	for _, r := range c.Skills {
		prov := r.EffectiveProvider()
		if prov.Type != "github" && prov.Type != "local" && prov.Type != "custom" {
			return fmt.Errorf("repository %q: invalid provider.type %q (want github, local, or custom)", r.Repo, prov.Type)
		}
		if prov.Type != "custom" && prov.BaseURL != "" && !(prov.Type == "github" && prov.BaseURL == "https://github.com") {
			return fmt.Errorf("repository %q: base_url is only valid for provider.type: custom", r.Repo)
		}
		for _, s := range r.Skills {
			st := s.State
			if st == "" {
				st = "installed"
			}
			if st != "installed" && st != "uninstalled" {
				return fmt.Errorf("repository %q skill %q: invalid state %q (want installed or uninstalled)", r.Repo, s.Name, s.State)
			}
		}
	}
	return nil
}

// Load reads and validates a config.yaml, applying defaults (provider github,
// state installed).
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := Validate(&c); err != nil {
		return nil, err
	}
	for i := range c.Skills {
		p := c.Skills[i].EffectiveProvider()
		c.Skills[i].Provider = &p
		for j := range c.Skills[i].Skills {
			if c.Skills[i].Skills[j].State == "" {
				c.Skills[i].Skills[j].State = "installed"
			}
		}
	}
	return &c, nil
}

// ExampleConfig returns a commented example demonstrating the full schema.
func ExampleConfig() string {
	return `# skill-manager-cli config
#
# dirs: agent identifier -> that agent's skill directory.
dirs:
  pi: ~/.pi/skills
  codex: ~/.codex/skills

# skills: repositories to manage.
# provider.type is one of:
#   github (default)  - repo "owner/name" resolves to https://github.com/owner/name
#   local             - repo is a filesystem path, no clone
#   custom            - repo "owner/name" resolves to <base_url>/owner/name
skills:
  # GitHub repository, install every skill under skills/:
  - repo: owner/repo
    provider:
      type: github
    skills:
      - name: "*"          # every immediate subdirectory of base_path
        base_path: skills  # default: skills
        state: installed   # default: installed; use "uninstalled" to remove

  # Local repository, install two named skills:
  - repo: /path/to/repo
    provider:
      type: local
    skills:
      - name: my-skill     # found at skills/my-skill
      - name: other-skill
        base_path: pi/skills  # found at pi/skills/other-skill

  # Custom git host:
  - repo: owner/repo
    provider:
      type: custom
      base_url: https://git.example.com
    skills:
      - name: "*"
`
}

// WriteExample writes the example config, refusing to overwrite an existing
// file.
func WriteExample(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists: %s", path)
	}
	return os.WriteFile(path, []byte(ExampleConfig()), 0644)
}

// JoinBase joins base_path (defaulting to "skills") with a skill name.
func JoinBase(p, name string) string {
	if p == "" {
		p = "skills"
	}
	return filepath.Join(p, name)
}
