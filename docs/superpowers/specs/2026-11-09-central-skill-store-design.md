# Central Skill Store Design

## Requirements

### User stories

- As a CLI user, I want resolved skills copied into a central store so every selected agent directory can share one managed source.
- As a CLI user, I want existing managed links updated safely so repeated installs converge on the selected skill.
- As a CLI user, I want uninstall to remove only links owned by the skill manager so local files are never silently deleted.

### Acceptance criteria

1. WHEN a source skill differs from its central-store destination THEN the store SHALL replace the managed destination with an exact copy.
2. WHEN a source skill is identical to its central-store destination THEN the store SHALL leave the destination unchanged.
3. WHEN a selected agent destination is absent THEN the store SHALL create a symlink to the central-store skill.
4. WHEN a selected agent destination is a symlink to a different target THEN the store SHALL replace it with a symlink to the central-store skill.
5. IF a selected agent destination exists and is not a symlink THEN the store SHALL return a conflict without modifying that destination.
6. WHEN uninstalling a link that points to the managed central-store skill THEN the store SHALL remove the link.
7. IF uninstalling a selected destination that is absent, a regular file, a directory, or a symlink to another target THEN the store SHALL leave it unchanged.
8. WHEN removing a stored skill THEN the store SHALL remove the central-store directory only after managed links have been handled by the caller.
9. IF the caller requests dry-run THEN the orchestration layer SHALL not invoke these destructive store operations.

### Constraints and non-goals

- Operations are local filesystem operations; source resolution and CLI flags are out of scope.
- The store package receives explicit paths, allowing tests to use temporary directories instead of `$HOME`.
- No operation follows destination symlinks while deciding whether to replace or delete them.

## Design

### Approaches considered

1. Put filesystem operations in CLI actions. This couples path safety to flags and makes isolated tests awkward.
2. Use an injected filesystem abstraction. This adds indirection not required by the standard library operations and temporary-directory tests.
3. Use a small `internal/store` package accepting explicit paths. This keeps the safety boundary reusable, uses real filesystem behavior in tests, and avoids premature abstractions.

**Decision:** use option 3.

### Architecture

`internal/store` owns physical copies, symbolic links, and removal checks. Future install/uninstall actions own source selection, agent-directory selection, and the `--dry-run` decision before calling mutating store functions.

```text
install orchestration --(source, store root)--> StoreSkill
install orchestration --(store skill, agent path)--> LinkSkill
uninstall orchestration --(store skill, agent path)--> UnlinkManagedSkill
uninstall orchestration --(store skill)--> RemoveSkill
```

### Components and interfaces

```go
package store

func StoreSkill(sourcePath, destinationPath string) (changed bool, err error)
func LinkSkill(storePath, linkPath string) (changed bool, err error)
func UnlinkManagedSkill(storePath, linkPath string) (removed bool, err error)
func RemoveSkill(storePath string) error
```

- `StoreSkill` compares complete directory trees before replacement. It stages a new copy next to the destination and renames it into place, so a copying failure cannot leave a partial destination.
- `LinkSkill` creates a link for a missing path, no-ops for an identical link, replaces a different symlink, and returns `ErrPathConflict` for a non-symlink target.
- `UnlinkManagedSkill` uses `Lstat` and link-target comparison, never `Stat`, so it deletes only a matching symlink.
- `RemoveSkill` removes the supplied central-store path. The caller sequences unlinking and dry-run enforcement.

### Error handling

`ErrPathConflict` identifies an existing non-symlink link destination. Standard filesystem errors retain their path context. Absent managed links are a no-op; an absent store directory is also a no-op removal.

### Testing strategy

Tests run exclusively in `t.TempDir()` directories against real files and symlinks. They cover initial copy, identical-copy no-op, changed-copy replacement, create/relink/conflict, safe unlink behavior, and removal. Each behavior starts as a focused failing test before production code is added.

## Implementation tasks

- [ ] 1. Add failing copy/store tests, then implement recursive comparison and atomic replacement. _Requirements: 1, 2_
- [ ] 2. Add failing link tests, then implement create, idempotent link, relink, and conflict behavior. _Requirements: 3, 4, 5_
- [ ] 3. Add failing unlink/removal tests, then implement managed-only unlink and central store removal. _Requirements: 6, 7, 8_
- [ ] 4. Run package and repository tests; verify dry-run responsibility remains outside the store package. _Requirements: 9_
