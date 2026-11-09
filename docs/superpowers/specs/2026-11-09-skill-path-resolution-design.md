# Skill Path Resolution Design

## Overview

SMC-7 resolves configured skill entries against a repository workspace before later install/uninstall orchestration consumes them. It must return every valid skill directory while retaining one failure for each independent invalid selection.

## Architecture

`internal/source` already owns repository workspaces, so it will also own selection resolution. A pure `ResolveSkills(root string, entries []config.SkillEntry) Resolution` function accepts an explicit repository root and configuration entries; it does not clone, install, or mutate files.

```
config.SkillEntry[] + repository root
              |
              v
      source.ResolveSkills
       |              |
resolved directories  selection failures
```

## Components and interfaces

```go
package source

type ResolvedSkill struct {
    Entry config.SkillEntry
    Path  string
}

type SelectionFailure struct {
    Entry config.SkillEntry
    Err   error
}

type SkillResolution struct {
    Skills   []ResolvedSkill
    Failures []SelectionFailure
}

func ResolveSkills(root string, entries []config.SkillEntry) SkillResolution
```

For each entry, an empty `BasePath` becomes `skills`. A named entry is one candidate directory at `<base>/<name>`. The literal `*` enumerates only immediate child directories under the effective base path and sorts their names lexically.

## Path safety and failures

A base path or named skill path that is absolute or lexically escapes `root` is rejected. Existing candidate paths are evaluated through symlinks and must still remain inside the canonical repository root before they are accepted. Missing paths and non-directories become `SelectionFailure` records. The resolver continues with later entries after any failure.

## Decision

The alternative is to defer resolution to the future CLI apply/install command. That would couple validation and partial-failure behavior to mutation orchestration. A source-package resolver is chosen because it is deterministic, testable with real fixture directories, and reusable by both configured and command-line sources.

## Testing strategy

Fixture-backed unit tests exercise default and custom bases, named entries, wildcards, deterministic ordering, ignored files, missing bases/names, continued processing, lexical traversal, and an in-root symlink pointing outside the repository.
