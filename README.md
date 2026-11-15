# skill-manager-cli

A Go CLI that manages agent skills for coding assistants. It installs skills from GitHub, local paths, or custom Git hosts into each agent's skill directory, and keeps that state reproducible with a declarative `config.yaml`.

## Features

- **Declarative state** — describe the skills you want in `config.yaml`, then run `apply` to make it so.
- **Multiple providers** — fetch skills from GitHub repositories, local filesystem paths, or custom Git hosts.
- **Multi-agent targeting** — install into several agents' skill directories at once via a `dirs` map or the `-a/--agents` flag.
- **Central skill store** — skills are cached in `~/.agents/skills` and symlinked into each agent directory.
- **Conflict detection** — conflicting skill entries across repositories are reported before any changes are made.
- **Safe operation modes** — `--dry-run` previews changes, confirmation prompts guard destructive actions, and `--overwrite` opts into replacing existing skills and symlinks.
- **Live progress** — per-repository and per-skill progress bars during operations.

## Installation

### Homebrew (macOS)

```sh
brew tap ansidev/skill-manager-cli https://github.com/ansidev/skill-manager-cli
brew trust --formula ansidev/skill-manager-cli/skill-manager-cli
brew install skill-manager-cli
```

### Download a release

Grab a binary from the [releases page](https://github.com/ansidev/skill-manager-cli/releases) — archives are built for `darwin`/`linux` on `amd64`/`arm64`.

### Build from source

Requires [Go 1.27+](https://go.dev/dl/):

```sh
git clone https://github.com/ansidev/skill-manager-cli.git
cd skill-manager-cli
go build -o skill-manager-cli .
```

## Quick start

```sh
# 1. Create a config.yaml in the current directory
skill-manager-cli init

# 2. Edit config.yaml to describe your skills (see below)

# 3. Preview what would change
skill-manager-cli apply --dry-run

# 4. Apply the configured state
skill-manager-cli apply
```

## Configuration

`skill-manager-cli init` writes a commented example config:

```yaml
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
```

## Commands

### `apply`

Applies the configured skill state: installs missing skills, removes skills marked `uninstalled`, and reports conflicts before touching files.

| Flag | Description |
| --- | --- |
| `--yes`, `-y` | Skip confirmation |
| `--dry-run` | Show planned operations without changing files |
| `--debug`, `-d` | Show operation diagnostics |
| `--overwrite` | Overwrite existing skills and symlinks |
| `--skill-dirs` | Comma-separated skill directories |
| `--agents`, `-a` | Comma-separated configured agent names |

### `install`

Install selected skills from one repository without touching the rest of the configured state.

### `uninstall`

Remove selected managed skills.

### `init`

Create a `config.yaml` example in the current directory (refuses to overwrite an existing one).

`install` and `uninstall` accept the shared flags: `--repo`, `--provider-type` (`github`, `local`, or `custom`), `--provider-base-url`, `--skill` (name or `*` for all), `--skill-base-path`, `--skill-dirs`, `--agents`/`-a`, `--yes`/`-y`, `--debug`/`-d`, and `--dry-run`. `install` additionally supports `--overwrite`.

## How it works

1. Skills are acquired into a central store at `~/.agents/skills`.
2. Each target agent directory gets a symlink to the stored skill.
3. Target directories come from the `--skill-dirs` flag, the `-a/--agents` flag (mapped through `dirs` in the config), or all configured dirs when neither is given.

## Development

```sh
# Install tooling (golangci-lint, goimports, lefthook) and enable git hooks
make prepare

# Run the full test suite
go test ./...

# Build local release archives and checksums into dist/ (no publishing)
make release-snapshot
```

Conventional Commits are enforced: `<type>[(scope)][!]: <description>` with types `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, and `revert`.

## License

[MIT](LICENSE)
