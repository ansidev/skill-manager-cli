# Central Skill Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe central skill copying and managed symbolic-link lifecycle operations.

**Architecture:** A focused `internal/store` package performs filesystem work against explicitly supplied paths. Future CLI orchestration decides whether dry-run permits a mutation before calling this package.

**Tech Stack:** Go standard library; `testing` and `t.TempDir`.

**Spec:** `docs/superpowers/specs/2026-11-09-central-skill-store-design.md`

## Global Constraints

- A non-symlink target at a desired link path is a conflict and is never removed.
- The package must use `Lstat` for destination ownership checks.
- Dry-run is enforced by callers, not by the filesystem package.

## Review Focus

- A relative symlink that resolves to the store must be treated as managed.
- A broken symlink must be classified from its link itself, not its target.
- Replacing a changed stored skill must not retain stale files.
- An identical store copy must not churn destination metadata.
- An unowned symlink must survive uninstall unchanged.

### Task 1: Store an exact skill copy

**Files:** Create `internal/store/store.go`; create `internal/store/store_test.go`.

- [ ] Write tests for copying an absent skill, no-op identical copies, and replacement of changed contents.
- [ ] Run `go test ./internal/store -run 'TestStoreSkill'` and confirm it fails because `StoreSkill` is missing.
- [ ] Implement `StoreSkill(sourcePath, destinationPath string) (bool, error)` using directory comparison and sibling staging/rename replacement.
- [ ] Re-run the focused test and confirm it passes.

### Task 2: Create and safely replace links

**Files:** Modify `internal/store/store.go`; modify `internal/store/store_test.go`.

- [ ] Write tests for missing destination creation, idempotent matching links, different-link replacement, and regular-file conflict.
- [ ] Run `go test ./internal/store -run 'TestLinkSkill'` and confirm it fails because `LinkSkill` is missing.
- [ ] Implement `LinkSkill(storePath, linkPath string) (bool, error)` and exported `ErrPathConflict`.
- [ ] Re-run the focused test and confirm it passes.

### Task 3: Remove only managed paths

**Files:** Modify `internal/store/store.go`; modify `internal/store/store_test.go`.

- [ ] Write tests for managed-link deletion, absent paths, unowned symlinks, regular files, and store removal.
- [ ] Run `go test ./internal/store -run 'Test(UnlinkManagedSkill|RemoveSkill)'` and confirm it fails because the functions are missing.
- [ ] Implement `UnlinkManagedSkill(storePath, linkPath string) (bool, error)` and `RemoveSkill(storePath string) error` with `Lstat` ownership checks.
- [ ] Re-run the focused tests and confirm they pass.

### Task 4: Integrate validation

**Files:** No production changes expected.

- [ ] Run `gofmt -w internal/store/*.go`.
- [ ] Run `go test ./...` and `go vet ./...`.
- [ ] Confirm each requirement in the spec maps to a passing test or an orchestration boundary.
