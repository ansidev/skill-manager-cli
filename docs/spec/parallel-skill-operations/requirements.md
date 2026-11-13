# Requirements: Parallel skill operations

## User story

As a CLI user managing skills across several repositories and agent directories, I want independent repository operations to proceed as soon as their sources are available and want the output to show every completed symlink, so I can see what was installed without waiting for unrelated clones.

## Acceptance criteria

1. WHEN a skill is installed THEN the CLI SHALL create a symlink in every selected mapped skill directory.
2. WHEN a target path conflicts with a symlink operation THEN the CLI SHALL report the failure and continue attempting the other selected targets.
3. WHEN a repository becomes available THEN the CLI SHALL resolve and apply its configured skills without waiting for other repository clones to finish.
4. WHEN skills are installed or uninstalled while other clones remain in progress THEN the CLI SHALL show live progress for those operations.
5. WHEN an operation finishes THEN the final report SHALL list each symlink actually created or deleted beneath its skill, including its full destination path.
6. WHEN an operation heading is printed THEN the CLI SHALL highlight it and place a blank line immediately before it.
7. WHEN a clone or skill operation fails THEN the CLI SHALL report the failure and continue independent repository operations.
8. WHEN all tasks finish THEN the CLI SHALL clear the live progress display before presenting the final report.
9. WHEN a target is unchanged, conflicts, or is included in a dry run THEN the CLI SHALL NOT report its symlink as created or deleted.

## Constraints

- Existing user-owned target paths remain protected unless overwrite is explicitly requested.
- Dry runs remain non-mutating.
- Failed symlink operations must not appear as created or deleted in the final report.
