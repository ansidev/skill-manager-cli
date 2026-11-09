# Operation Summary and End-to-End Coverage Implementation Plan

**Goal:** Add structured operation summaries to `apply`, `install`, and
`uninstall`, and prove the lifecycle with an end-to-end fixture.

**Architecture:** `internal/app` builds `operationOutcome` values during
orchestration and logs them through one `slog` emitter before returning a
failure-derived error.

**Tech Stack:** Go standard library (`log/slog`, `testing`), existing
`source`, `store`, `config`, and `skilldirs` packages.

**Spec:** `docs/superpowers/specs/2026-11-09-operation-summary-e2e-design.md`

## Global Constraints

- Dry runs and declined confirmations never mutate the store or targets.
- A failing skill or target does not stop unrelated work.
- The summary is logged before a non-zero exit is returned.
- The canonical store root is `~/.agents/skills`.

## Review Focus

- Wildcard selection produces one outcome per selected skill.
- `skipped` is only used for genuine no-ops.
- Uninstall preserves the stored skill when no managed link was removed.
- A partial failure still reports the successful outcomes.

### Task 1: Add failing end-to-end and summary tests

**Files:** Modify `internal/app/app_test.go`.

- [ ] Add an end-to-end fixture covering `init`, wildcard and named local
      installs, dry-run, confirmed install, direct uninstall, and target
      symlink verification.
- [ ] Add summary assertions for field presence, action classification, and
      `dry_run`.
- [ ] Add a partial-failure test asserting non-nil error plus a complete
      summary.
- [ ] Run `go test ./internal/app` and confirm the new tests fail for the
      expected reasons.

### Task 2: Add the outcome model and summary emitter

**Files:** Modify `internal/app/app.go`.

- [ ] Add action constants and `operationOutcome`.
- [ ] Add the shared store-root resolver used by all commands.
- [ ] Add `logSummary` with per-outcome fields and the aggregate record.
- [ ] Re-run the focused tests and confirm only orchestration assertions
      remain failing.

### Task 3: Refactor orchestration to collect outcomes

**Files:** Modify `internal/app/app.go`.

- [ ] Make `applySkill` classify installed, uninstalled, skipped, and failed
      outcomes and aggregate target errors.
- [ ] Make `apply` emit source/path information and the summary for every
      repository and skill, including repository-level failures.
- [ ] Make `install` continue across resolved skills and resolution failures
      and emit a summary.
- [ ] Make `uninstall` continue across targets, classify the outcome, and emit
      a summary.

### Task 4: Verify

**Files:** No production changes expected.

- [ ] Run `gofmt -l -w internal/app/*.go`.
- [ ] Run `go test ./...` and `go vet ./...`.
- [ ] Confirm each spec requirement maps to a passing test or an explicit
      orchestration boundary.
