# SMC-4: Operation report sections and overwrite mode

## Phase 1: Requirements

**User story:** As a skill-manager CLI user, I want each operation result reported once under a clear outcome section and grouped by repository, so I can tell which skills changed, which were already in the requested state, and where each came from.

**Acceptance criteria (EARS):**

1. WHEN an operation summary is emitted THEN it SHALL use four distinct sections: `Installed skills`, `Skills to install but skipped (already installed)`, `Uninstalled skills`, and `Skills to uninstall but skipped (already uninstalled)`.
2. WHEN a section contains outcomes from multiple repositories THEN the summary SHALL group those outcomes by repository.
3. WHEN a section has no outcomes THEN the summary SHALL omit that section; each outcome SHALL appear under exactly one heading.
4. WHEN `--overwrite` is enabled for `apply` or `install` THEN the CLI SHALL replace conflicting target entries and synchronize the central skill-store directory with upstream, including removing stale files; without the flag it SHALL retain current non-overwrite behavior.
5. WHEN uninstalling a skill THEN the command SHALL remove only managed links and its central-store copy; uninstall SHALL not expose `--overwrite`, because overwrite has no safe additional meaning for removal.
6. WHEN a target path resolves to or overlaps its central-store path THEN apply/install SHALL report failure and SHALL NOT replace the target with a link into itself.

**Constraints and edge cases:** Dry-run remains non-mutating. Ordinary target files remain protected by default. Failures remain visible in the report and produce a non-zero command result. Overwrite remains disabled by default.

## Phase 2: Design

### Overview

Correct the existing operation summary and complete the existing overwrite behavior with focused regression coverage. Reuse the existing `operationOutcome`, repository-grouping helper, `store.StoreSkill`, and `store.LinkSkill`; no new dependencies or output formats are needed.

### Approaches

1. **Keep the current summary helper and render one heading per outcome group (chosen).** This preserves the existing grouping and ordering while removing duplicate synonym headings. Empty groups remain omitted.
2. **Replace the summary format or add a structured output mode.** This introduces unrelated compatibility and interface work; the issue asks for clear text sections.
3. **Give uninstall overwrite destructive semantics.** This could delete unowned files and directories, which is outside the managed-skill contract and creates data-loss risk. Uninstall therefore does not expose the flag.

### Architecture and interfaces

`apply`, `install`, and `uninstall` continue to collect `operationOutcome` values and pass them to `logSummary`. `logSummary` classifies each outcome by requested state and action, groups within each class by repository, prints each required heading at most once, and keeps the existing completion count and exit behavior. The existing apply/install overwrite flag flows to the store-copy and target-link operations. The uninstall command uses its own direct-command flags without overwrite.

### Data models

No model changes are needed. `operationOutcome` already records operation, skill, action, requested state, repository, paths, dry-run state, and error. The four report sections are projections of those fields.

### Error handling and safety

Keep current behavior: failed outcomes appear in the section for their requested installed/uninstalled state, include the error detail, and count toward the final non-zero result. Without overwrite, user-owned non-symlink target paths continue to be protected. With overwrite, replacing a target path requires explicit opt-in. Before writing the store, reject target overlap with the whole central store (including its resolved symlink target) and reject overlapping target paths; after synchronizing the store, validate again before replacing targets, and validate each target immediately before linking it. These checks catch dangling aliases, symlink layout changes caused by replacing a store symlink, and overlap introduced by an earlier target link. This prevents a target alias from deleting the store or creating a self-referencing link. Preserve each failed selection's requested state, including repository-acquisition failures, so wildcard uninstall failures remain in the uninstall section. Uninstall removes only managed symlinks and the store entry after at least one managed link was removed.

### Testing strategy

- Add an app-level unit test proving all four headings are distinct, each appears once, and outcomes are grouped by repository.
- Update skipped-operation CLI assertions to use the corresponding precise headings.
- Add store and CLI tests proving overwrite synchronizes changed content, removes stale entries, replaces an existing target path only when explicitly enabled, and rejects existing, dangling, symlinked-store, post-copy, and overlapping-target paths.
- Verify failed wildcard uninstalls and repository acquisition failures retain their uninstall classification, and direct install acquisition failures name the selected skill.
- Verify nested target paths are rechecked after earlier links change filesystem resolution.
- Check CLI help/flag registration to ensure apply and install expose `--overwrite` and uninstall does not.
- Run the repository Go test suite and build.

## Phase 3: Implementation tasks

- [x] 1. Capture requirements and existing behavior in this document; post Phase 1 requirements to the Kanpal issue.
- [x] 2. Add a failing summary regression test for one heading per outcome group and repository grouping.
- [x] 3. Simplify `logSummary` to emit only the four specified headings and update skipped-operation expectations.
- [x] 4. Scope `--overwrite` to apply/install, verify replacement/synchronization behavior with focused tests, and keep uninstall limited to managed removals.
- [ ] 5. Run Go validation, inspect the final worktree diff, commit only SMC-4 files, and move the issue to `in-review`.
