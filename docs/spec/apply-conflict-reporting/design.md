# Design: Apply conflict reporting

## Overview

`apply` becomes two-phase: it resolves every repository first, detects conflicts between repositories by effective skill name, reports them, and only then performs the non-conflicting operations. See [requirements.md](requirements.md).

## Architecture

`internal/app` orchestrates three phases inside the `apply` command:

1. **Resolve phase** — each repository is obtained (local in-line, remote via the bounded clone pool) and its configured entries resolved, including `"*"` expansion. No skill action runs here. Named uninstalls (no workspace needed) and resolved skills are collected as pending operations.
2. **Conflict phase** — pending operations are grouped by effective skill name; a group spanning multiple repositories is a conflict when the requested states differ, or when all are `installed` and the skill directories differ. `internal/store.DirectoriesEqual` (exported from the existing comparator) compares content. Conflicts are printed under a highlighted "Skill conflicts" heading before any action.
3. **Apply phase** — pending operations whose name is in conflict are recorded as failed outcomes without performing any filesystem change; all others execute through the existing `applySkill` path. Workspaces are cleaned up at the end.

## Error handling

- Conflicting operations fail with a reason naming the repositories involved; the command completes with failures and exits non-zero.
- Acquisition and resolution failures are reported as before and do not suppress independent repositories.
- Dry runs surface the conflict report and perform no mutation.

## Testing strategy

- Unit tests for `detectSkillConflicts`: state conflict, content conflict, identical content, single-repository groups.
- In-process integration tests: state conflict blocks install and uninstall (a previously stored skill is preserved), content conflict across explicit and `"*"`-resolved names blocks both repositories, non-conflicting skills are still applied, dry run reports without mutating.
- The former paused-clone streaming test now asserts that no skill action runs while any clone is pending and that all actions complete once every repository resolves.
