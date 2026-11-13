# Tasks: Parallel skill operations

Requirements are numbered in [requirements.md](requirements.md); the implementation is described in [design.md](design.md).

- [x] 1. Resolve all selected target directories and attempt each skill link independently.
  - Components: `internal/app` and `internal/store`.
  - _Requirements: 1, 2_
- [x] 2. Process each available repository without waiting for unrelated clones and show clone and skill progress.
  - Components: `internal/app` and `internal/source`.
  - _Requirements: 3, 4, 7_
- [x] 3. Retain a full-path completed record for every successful symlink operation.
  - Component: `internal/app` progress reporting.
  - _Requirement: 5_
- [x] 4. Highlight operation headings and precede them with a blank line.
  - Component: `internal/app` output rendering.
  - _Requirement: 6_
- [x] 5. Cover executable workflows, the paused clone, conflict handling, and output with integration tests; run the Go suite and race detector.
  - Component: `internal/app/cli_integration_test.go` and the Go test suite.
  - _Requirements: 1–7_
