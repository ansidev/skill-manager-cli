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
- [x] 6. Fail fast on configuration-detectable state conflicts before any repository acquisition or confirmation.
  - When the configuration declares no skill entry named "*", detect state conflicts between repositories from the configuration alone, print the "Skill conflicts" report, and exit non-zero before requesting confirmation and before obtaining any repository.
  - Components: `internal/app` (configuration-level conflict detection feeding the apply flow), `internal/app` tests.
  - _Requirements: 9, 10_
- [ ] 7. Preserve the normal flow without configuration state conflicts; a wildcard entry disables fail-fast; same-named installed entries defer to post-resolution content comparison.
  - Components: `internal/app` tests.
  - _Requirements: 11, 12, 13_

## Implementation Notes

- `config.Load` normalizes empty states to "installed"; `normalizeSkillState` in `internal/app/app.go` is the canonical state normalizer for conflict detection.
- `printSkillConflicts` in `internal/app/conflicts.go` renders the "Skill conflicts" report; `describeStateConflict` builds state-conflict entries from `conflictCandidate` values.
- In-process app tests set `HOME` to a temp dir and use `changeDirectory(t, root)`; executable tests live in `internal/app/cli_integration_test.go`.
