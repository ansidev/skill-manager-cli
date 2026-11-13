# Design: Parallel skill operations

## Overview

Apply repository operations as their sources become available and retain a readable final record of each successful symlink operation. The requirements are in [requirements.md](requirements.md).

## Architecture

`apply` handles named uninstalls without cloning and processes local repositories directly. A bounded worker pool clones remote repositories; each worker resolves and applies its repository's skills as soon as its clone finishes. Skill operations are serialized to protect shared store entries and target links when repositories contain the same skill name. Repository cloning continues independently of those operations.

## Components and interfaces

- `internal/app` orchestrates repository acquisition, skill operations, progress, and the final summary.
- `internal/source` acquires repositories and resolves their configured skills.
- `internal/store` copies skills and manages links in the selected target directories.
- The progress display tracks clone and skill tasks and clears completed rows. The operation summary groups outcomes by repository and result and lists each symlink actually created or deleted beneath its skill, using its full destination path.

## Data models

Each operation produces an outcome identifying the skill, repository, requested state, result, error, and paths of links actually created or deleted.

## Error handling

A clone or skill failure is recorded without suppressing independent work. Link creation attempts continue after a target failure; only links actually created or deleted are included in the final report. The command reports failure when its outcomes include failures. Existing user-owned target paths remain protected unless overwrite is explicitly requested, and dry runs do not mutate files.

## Testing strategy

Executable integration tests use real local Git repositories and temporary stores and target directories. They pause one clone and verify another repository's install and uninstall, symlinks, and progress before releasing it. Separate tests exercise `install` and `apply` with multiple skills and mapped directories, verify full destination paths and actual links, and check conflicts and headings. Run the full Go test suite and the application race detector.
