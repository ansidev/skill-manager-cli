# Design: Direct skill selection

## Overview and architecture

Direct commands use the existing configuration, source resolver, and store operations. Selection returns one repository and a list of skill entries: an explicit name selects one entry; otherwise the matching repository supplies its entries. Explicit base path takes precedence, followed by a matching configured named entry and the default `skills` directory. An explicit wildcard is resolved against the source.

## Components and interfaces

`internal/app` validates selection, resolves sources when necessary, and reports each operation. `internal/source.ResolveSkills` expands wildcard entries. Named uninstalls need no source, even with multiple configured repositories; wildcard uninstalls acquire the source and expand it before removing the resulting names. Install and uninstall apply the requested command regardless of configured state. Selection and operation failures are summarized without stopping independent entries. The existing store and target safety checks remain authoritative.

## Data and error handling

A repository contains configured skill entries with names, base paths, and states. Direct commands choose entries by name and override state by the requested operation. Missing or ambiguous repositories, missing selections, unsafe names, unavailable sources, conflicting source paths for a stored name, and failed file operations produce actionable errors and a failing exit status. Before storing skills, reject different source paths resolving to the same name; repeated selections of the same path are processed once. Do not infer repository ownership from the shared store.

## Decision

Use configured selections for a repository without `--skill`. An alternative is treating the omission as `*`, but that would silently include unconfigured skills and disregard custom base paths. For an unconfigured repository, require `--skill`; users can select every immediate child explicitly with `--skill '*'`.

## Testing strategy

Build and execute the CLI against temporary local repositories and isolated central and target directories. Check selection, configured base paths, wildcard expansion, partial failures, output, status, and safety. Run the full Go suite.
