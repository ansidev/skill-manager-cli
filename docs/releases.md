# Release binaries

The release workflow publishes the existing changelog-backed GitHub Release, then GoReleaser attaches archives for macOS, Linux, and Windows on amd64 and arm64. Archive names follow `skill-manager-cli_<version>_<os>_<arch>`; Windows archives use `.zip`, and the other targets use `.tar.gz`. `skill-manager-cli_<version>_checksums.txt` contains SHA-256 checksums for the archives. The binaries report their embedded release version and commit with `--version`.

The workflow uses the `GH_TOKEN` repository secret both for the existing release action and GoReleaser's asset upload. Configure that secret with permission to write repository contents. The workflow job also declares `contents: write`. GoReleaser keeps existing release notes, so the notes generated from `.changes/v<VERSION>.md` remain authoritative.

To build and package all six targets locally without creating or changing a GitHub Release:

```sh
make release-snapshot
```

This requires GoReleaser v2 and the Go version declared by `go.mod`. The output is written to `dist/`.
