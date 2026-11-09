# Skill Path Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve configured named and wildcard skill directories safely inside repository workspaces.

**Architecture:** Extend `internal/source` with a pure resolver that returns successful directories plus independent per-selection failures. It validates lexical and symlink-resolved containment before inspecting a candidate.

**Tech Stack:** Go standard library; `testing`; repository fixture directories.

**Spec:** `docs/superpowers/specs/2026-11-09-skill-path-resolution-design.md`

## Global Constraints

- An empty `base_path` means `skills`.
- Only the exact name `*` has wildcard behavior.
- A selection may not access a location outside the repository root, including through a symlink.
- An invalid selection must not suppress later independent selections.

## Review Focus

- An absolute or `..`-containing base/name must not access paths outside the workspace.
- A symlink inside the workspace pointing outside it must be rejected.
- Wildcards must include directories only, ordered lexically, without recursive discovery.
- A file used as a named selection or base must be reported as a failure.
- Valid selections after failures must still be returned.

---

### Task 1: Specify resolver behavior with fixtures

**Files:**
- Create: `internal/source/source_test.go`

**Interfaces:**
- Produces tests for `ResolveSkills(root string, entries []config.SkillEntry) SkillResolution`.

- [ ] **Step 1: Write failing tests** for default-base named resolution; custom-base named resolution; wildcard directory-only lexical expansion; missing/non-directory selections; continuation after failures; and lexical/symlink traversal rejection.
- [ ] **Step 2: Run the focused test**

Run: `go test ./internal/source -run TestResolveSkills`

Expected: FAIL because `ResolveSkills` is undefined.

### Task 2: Implement safe path resolution

**Files:**
- Modify: `internal/source/source.go`
- Modify: `internal/source/source_test.go`

**Interfaces:**
- Consumes: the Task 1 behavior tests.
- Produces: `ResolvedSkill`, `SelectionFailure`, `SkillResolution`, and `ResolveSkills`.

- [ ] **Step 1: Implement** the exported data types and `ResolveSkills`, using the default base, `os.ReadDir` for immediate wildcard children, lexical sorting, and containment checks before accepting an existing directory.
- [ ] **Step 2: Run the focused test**

Run: `go test ./internal/source -run TestResolveSkills`

Expected: PASS.

### Task 3: Format and validate repository integration

**Files:**
- Modify: `internal/source/source.go`
- Modify: `internal/source/source_test.go`

- [ ] **Step 1: Format modified Go files**

Run: `gofmt -w internal/source/source.go internal/source/source_test.go`

- [ ] **Step 2: Run full validation**

Run: `go test ./... && go vet ./...`

Expected: PASS with no failures or vet diagnostics.
- [ ] **Step 3: Commit issue-only files**

Run: `git add internal/source/source.go internal/source/source_test.go docs/superpowers/specs/2026-11-09-skill-path-resolution-design.md docs/superpowers/plans/2026-11-09-skill-path-resolution.md && git commit -m "feat(source): resolve configured skill paths"`

## Self-review

- Default, custom, named, wildcard, missing-path, non-directory, deterministic-order, continuation, lexical-traversal, and symlink-escape requirements map to Task 1 tests and Task 2 implementation.
- The resolver interface carries all output required by later orchestration without adding mutation responsibilities.
- Review Focus items each have a corresponding Task 1 test.
