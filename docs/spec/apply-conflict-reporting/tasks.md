# Tasks: Apply conflict reporting

Requirements are numbered in [requirements.md](requirements.md); the implementation is described in [design.md](design.md).

- [x] 1. Collect all repositories' resolved skills before acting and export content comparison from `internal/store`.
  - Components: `internal/app`, `internal/store`.
  - _Requirements: 8_
- [x] 2. Detect state and content conflicts by effective skill name across repositories and print the "Skill conflicts" report before any action.
  - Component: `internal/app/conflicts.go`.
  - _Requirements: 1, 2, 3, 4_
- [x] 3. Gate conflicting skill names: no action performed, failed outcomes, non-zero exit; non-conflicting skills still apply.
  - Component: `internal/app` apply phases.
  - _Requirements: 5, 6_
- [x] 4. Report conflicts in dry-run mode without mutating files.
  - Component: `internal/app` apply phases.
  - _Requirements: 7_
- [x] 5. Cover conflict detection and gating with unit and integration tests; update the paused-clone test to the resolve-first behavior.
  - Components: `internal/app/apply_conflicts_test.go`, `internal/app/cli_integration_test.go`.
  - _Requirements: 1–8_
