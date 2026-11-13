.DEFAULT_GOAL := prepare
.PHONY: prepare release-snapshot

# Build release archives and checksums locally without publishing a release.
release-snapshot:
	goreleaser release --snapshot --clean

# Install the tools used by the git hooks and enable the hooks (works in linked
# git worktrees: the hooks live in the shared git hooks directory).
prepare:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@if ! command -v lefthook >/dev/null 2>&1; then \
		if command -v brew >/dev/null 2>&1; then \
			brew install lefthook; \
		else \
			echo "lefthook is not installed. Install a prebuilt binary: https://lefthook.dev/installation/" >&2; \
			exit 1; \
		fi; \
	fi
	lefthook install
