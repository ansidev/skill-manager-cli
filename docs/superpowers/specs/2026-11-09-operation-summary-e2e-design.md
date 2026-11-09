# Operation Summary and End-to-End Coverage Design

## Intent

Give `apply`, `install`, and `uninstall` one structured, complete result summary
and prove the full lifecycle with an end-to-end fixture.

## Goals

- Emit one summary record per outcome with stable fields for humans and
  automation.
- Classify each outcome as `installed`, `uninstalled`, `skipped`, or `failed`.
- Continue processing remaining skills and targets after an individual failure.
- Return a non-zero status after logging the full summary when anything failed.
- Cover `init`, named and wildcard local skills, dry-run, confirmed install,
  uninstall, and target symlink verification in one fixture.
- Keep `slog` as the single progress and summary channel.

## Non-goals

- JSON output mode, summary file output, or provider changes.
- Reconciling multiple configuration repositories differently than `apply`
  already does.

## Architecture

The orchestration in `internal/app` builds an `operationOutcome` per skill (or
per repository-level failure) and passes the collected outcomes to one summary
emitter.

```text
apply:     config -> resolve targets -> confirm -> per repo -> per skill -> outcomes -> summary
install:   selection -> resolve targets -> confirm -> per resolved skill -> outcomes -> summary
uninstall: selection -> resolve targets -> confirm -> per target -> outcome -> summary
```

The emitter logs each record at info level with `msg=operation`, then one
`msg=summary` aggregate. A non-zero failed count becomes the returned error
after the aggregate is written, so the process exit status changes but the
summary is never lost.

### Summary fields

| Field | Meaning |
| --- | --- |
| `operation` | `apply`, `install`, or `uninstall` |
| `skill` | Resolved skill name; empty only for a repository-level failure |
| `action` | `installed`, `uninstalled`, `skipped`, or `failed` |
| `repository` | Configured repository identifier |
| `source` | Local repository path or resolved remote URL |
| `path` | Skill path relative to the repository root |
| `targets` | Comma-separated target directories the outcome applied to |
| `dry_run` | Whether the command was a dry run |
| `error` | Error text, present only when `action=failed` |

The aggregate record carries `operation`, `operations`, and `failed`.

## Decisions

### Decision: reuse `log/slog` for the summary

**Context:** Progress already uses `slog`; automation needs stable keys.

**Options Considered:**
1. Plain `fmt` text - Pros: simple / Cons: unstable for automation, duplicates
   the logger.
2. JSON lines - Pros: easiest to parse / Cons: new output mode and dependency
   expectations the issue does not ask for.
3. `slog` text records - Pros: consistent with progress, key/value pairs are
   parseable and readable / Cons: includes timestamps and level.

**Decision:** option 3.

**Rationale:** It satisfies both audiences with no new format contract while
keeping the final summary complete and ordered.

### Decision: classify no-op outcomes as `skipped`

An install whose store copy is unchanged and whose links already match, or an
uninstall that finds no managed link, produces `skipped`. This distinguishes
"already converged" from a real mutation.

### Decision: a dry run reports the intended action

Dry-run outcomes keep the intended `installed`/`uninstalled` action and set
`dry_run=true`; no store or target call is made.

### Decision: unify the central store root

`apply` historically used `~/.skill-manager/skills` while `install` and
`uninstall` used `~/.agents/skills`. The end-to-end lifecycle exposes the
mismatch. The canonical root is `~/.agents/skills` (direct-skill-commands
design); all commands share one resolver.

## Error handling

- A store or link failure marks that skill `failed` and processing continues
  with the next skill.
- A link failure for one target does not stop other targets; the first error is
  retained on the outcome.
- Repository acquisition and skill resolution failures become `failed`
  outcomes so independent repositories and skills still run.
- Uninstall never removes the stored skill when no managed link was removed.

## Testing strategy

- App-level end-to-end test drives the CLI through `init`, wildcard and named
  local installs, dry-run, confirmed install, direct uninstall, and
  apply-driven uninstall while asserting store copies and target symlinks.
- Focused tests assert summary records and exit status for success and partial
  failure.
- Mutation checks compare the store and target trees before and after dry-run
  and declined confirmation.
