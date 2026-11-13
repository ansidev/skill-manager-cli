# Requirements: Direct skill selection

## User story

As a CLI user, I want to install or remove a named skill or the skills selected by a repository configuration so that I can manage just the skills I intend.

## Acceptance criteria

1. WHEN a user selects a named skill with `--skill` THEN the command SHALL manage that skill, using matching repository configuration for its base path unless explicitly overridden.
2. WHEN a user selects a configured repository without `--skill` THEN the command SHALL manage all its configured selections, including `*` immediate children at the configured base path.
3. WHEN no repository is given THEN the command SHALL use the sole configured repository, or SHALL reject an ambiguous selection. A named uninstall can operate without a repository even if several are configured.
4. WHEN a repository has no configured selection and `--skill` is absent THEN the command SHALL reject the request with an actionable error.
5. WHEN a selection or operation fails THEN the command SHALL report the affected skill or path, return a nonzero status, and continue independent operations where safe. WHEN multiple selected source paths resolve to the same stored name THEN the command SHALL reject the conflicting name without overwriting it.
6. WHEN uninstalling THEN the command SHALL preserve existing managed-link and user-owned-file protections; WHEN a dry run is requested THEN it SHALL make no changes.
7. WHEN executable integration tests run THEN they SHALL use isolated sources, stores, and targets and verify output, exit status, and filesystem effects for successful and failed selections.

Named uninstall does not require the source repository. Wildcard uninstall requires access to the source to enumerate its names. The central store is global and does not track repository ownership: removing a selected name removes the stored skill of that name even if it was installed from another repository. Both commands use configured target directories or the `--skill-dirs` or `--agents` flags. Explicit `--skill-base-path` overrides configured paths. The command determines the operation regardless of a configured entry's state.

For example, `install --repo owner/repo --skill example` selects one skill, `install --repo owner/repo` selects that repository's configured entries, and `uninstall --repo owner/repo --skill '*'` removes the immediate child skill names discovered under the selected base path. A repository not present in the configuration requires an explicit skill or `'*'`.
