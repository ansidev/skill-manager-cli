# Repository guidance

## Project

This repository is the Go CLI `github.com/ansidev/skill-manager-cli`. It requires Go 1.27.3. The command entry point is `main.go`; application and domain packages live under `internal/`.

## Development

- Run the full test suite with `go test ./...` after Go code changes.
- Format Go files with `gofmt` and `goimports`. The pre-commit hooks run these formatters on staged Go files and run `golangci-lint run ./...`.
- Read the relevant requirements/design documents under `docs/spec/` before changing behavior they cover.
- Keep changes focused and add or update tests alongside behavior changes.
- Use the repository's Conventional Commits format: `<type>[(scope)][!]: <description>`. Allowed types are `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, and `revert`.

## Release packaging

- `.goreleaser.yaml` defines the release archives and checksums.
- Run `make release-snapshot` to build local snapshot artifacts without publishing. This command cleans and recreates `dist/`.
- The existing release workflow publishes the changelog-backed GitHub Release before uploading GoReleaser assets. Preserve its release notes and tag flow when editing it.
