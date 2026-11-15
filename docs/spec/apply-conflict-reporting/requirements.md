# Requirements: Apply conflict reporting

## User story

As a CLI user managing skills from several repositories, I want `apply` to warn me before it performs any action when the same skill name comes from different repositories with conflicting requests or content, so no repository silently wins.

## Background

Previously, when two repositories resolved the same skill name (explicitly or through `"*"`), the later entry silently took effect: a later `uninstalled` removed a skill another repository installed, and a later `installed` copy overwrote the content of an earlier one.

## Acceptance criteria

1. WHEN two different repositories resolve the same effective skill name (including names resolved from `"*"`) with different states (e.g. `installed` vs `uninstalled`) THEN `apply` SHALL report a state conflict to the user before performing any action.
2. WHEN two different repositories resolve the same effective skill name with state `installed` and their skill directories have differing content THEN `apply` SHALL report a content conflict to the user before performing any action.
3. WHEN two different repositories resolve the same effective skill name with state `installed` and identical content THEN `apply` SHALL NOT report a conflict (re-applying identical content is a no-op).
4. WHEN any conflict is detected THEN `apply` SHALL print a "Skill conflicts" report listing the skill name, each involved repository with its requested state and path, and the reason.
5. WHEN a skill name is in conflict THEN `apply` SHALL NOT perform any install or uninstall action for that name; the operation is reported as failed and the command exits non-zero.
6. WHEN a skill is not in conflict THEN `apply` SHALL still perform its configured action.
7. WHEN conflicts are detected THEN `apply` SHALL report them even in dry-run mode, and dry runs SHALL remain non-mutating.
8. WHEN applying, `apply` SHALL resolve every repository (including remote clones) before performing the first skill action, so the conflict report covers all repositories.

## Fail-fast conflict detection without wildcard entries

9. WHEN the configuration declares no skill entry named `"*"` THEN the CLI SHALL determine state conflicts between repositories from the configuration alone before obtaining any repository.
10. WHEN such a configuration contains a state conflict across repositories THEN the CLI SHALL print the "Skill conflicts" report and exit non-zero before requesting confirmation and before obtaining any repository.
11. WHEN such a configuration contains no state conflict THEN the CLI SHALL continue its normal flow and behave as specified by criteria 1–8.
12. WHEN the configuration declares at least one skill entry named `"*"` THEN the CLI SHALL NOT fail before repositories are resolved and SHALL follow the resolve-then-report flow of criteria 1–8.
13. IF same-named entries from different repositories are all requested as `installed` THEN the CLI SHALL NOT treat them as a conflict before their skill content has been resolved (identical content is permitted per criterion 3).

## Constraints

- Requirement 8 supersedes requirement 3 of `docs/spec/parallel-skill-operations` (apply-as-available streaming): `apply` now resolves all repositories before acting. Remote clones still run with bounded parallelism.
- Conflicts are reported and the affected operations fail; other skills remain applied. The user resolves the conflict and re-runs.
- Boundary: fail-fast detection covers only configuration-detectable (state) conflicts. Content conflicts require resolved skill directories and remain part of the post-resolution conflict phase; a wildcard entry anywhere in the configuration disables fail-fast entirely.
