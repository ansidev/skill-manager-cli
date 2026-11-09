# Direct Skill Commands Design

## Intent

Provide single-repository `install` and `uninstall` commands without requiring
a complete configuration file. They use the existing source resolver, safe
store/link primitives, and target-directory resolver.

## Goals

- Accept explicit repository, provider, and skill-selection values.
- Fall back to a matching single configuration repository when explicit values
  are absent.
- Preserve the central store at `~/.agents/skills` and never replace
  user-owned target paths.
- Give all mutations a confirmation and make dry runs non-mutating.

## Non-goals

- Reconcile multiple configuration repositories; that remains `apply` work.
- Add providers or change storage/link semantics.

## Approaches considered

1. Implement filesystem behavior directly in each CLI action. This duplicates
   path, dry-run, and logging decisions.
2. Add a command orchestration layer in `internal/app`, which composes the
   existing source, store, and skill-directory packages. Chosen: it preserves
   the safety invariants already tested by those packages and gives install and
   uninstall one shared UX path.

## Architecture

`cli.Context` is parsed into direct-command options. The app loads
`config.yaml` only when present, combines direct values with a matching
configuration repository, resolves target directories, and runs one of two
flows:

```
install: source.Obtain -> source.ResolveSkills -> store.StoreSkill -> store.LinkSkill
uninstall: store.UnlinkManagedSkill -> store.RemoveSkill
```

Each resolved skill is stored below `~/.agents/skills/<skill-name>`. For
uninstall, the selected name identifies that store entry; the repository is
not needed. Before mutation, the flow either receives `-y`, asks via the CLI
reader/writer, or reports only when `--dry-run` is set.

## Interfaces and data

An internal `commandOptions` value holds repository/provider/skill values plus
shared target, confirmation, debug, and dry-run settings. `resolveSelection`
returns a normalized repository and skill entry. Missing `repo` for install,
missing `skill`, or an empty resolved target set are validation errors.

## Errors

- Invalid provider values are rejected before acquiring a source.
- Source selection failures retain the existing resolver context.
- User-owned target paths retain `store.ErrPathConflict` behavior.
- A denied confirmation returns an explicit cancellation error.

## Testing strategy

App tests use temporary local repositories, temporary target directories, and
an overridden home directory to prove install, uninstall, dry-run, validation,
and command flags. Existing source/store unit tests remain the detailed safety
coverage for traversal and link ownership.

## Task plan

- [ ] 1. Add failing app-level tests for direct install/uninstall flows.
  - Cover local source installation, managed unlink/removal, dry-run, and
    required input validation.
  - _Requirements: 1-6, 8_
- [ ] 2. Add shared CLI flags and selection/confirmation helpers.
  - Resolve direct values with optional config defaults and target directories.
  - _Requirements: 1, 2, 5_
- [ ] 3. Implement install and uninstall orchestration with structured logs.
  - Use only `source`, `store`, and `skilldirs` public APIs.
  - _Requirements: 3, 4, 6-8_
- [ ] 4. Run formatting and the full Go test suite.
  - _Requirements: all_
