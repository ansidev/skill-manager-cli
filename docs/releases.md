# Release binaries

The release workflow publishes the existing changelog-backed GitHub Release, then GoReleaser attaches archives for macOS, Linux, and Windows on amd64 and arm64. Archive names follow `skill-manager-cli_<version>_<os>_<arch>`; Windows archives use `.zip`, and the other targets use `.tar.gz`. `skill-manager-cli_<version>_checksums.txt` contains SHA-256 checksums for the archives. The binaries report their embedded release version and commit with `--version`.

The workflow uses the `GH_TOKEN` repository secret both for the existing release action and GoReleaser's asset upload. Configure that secret with permission to write repository contents. The workflow job also declares `contents: write`. GoReleaser keeps existing release notes, so the notes generated from `.changes/v<VERSION>.md` remain authoritative.

To build and package all six targets locally without creating or changing a GitHub Release:

```sh
make release-snapshot
```

This requires GoReleaser v2 and the Go version declared by `go.mod`. The output is written to `dist/`.

## Homebrew

After the first release containing this integration, install from the source repository as a tap:

```sh
brew tap ansidev/skill-manager-cli https://github.com/ansidev/skill-manager-cli
brew trust --tap ansidev/skill-manager-cli
brew install ansidev/skill-manager-cli/skill-manager-cli
```

For Homebrew versions without `brew trust`, follow the prompt for third-party taps. Upgrade with `brew update` followed by `brew upgrade skill-manager-cli`.

After GoReleaser uploads the assets, the release job downloads all four macOS and Linux archives and the checksum manifest, verifies each SHA-256, and generates `Formula/skill-manager-cli.rb` on `main`. The first formula is created by the first successful release; it cannot be installed before then. A missing asset, invalid manifest, mismatched checksum, or rejected push fails the job without publishing an incomplete formula. Retry the release workflow after correcting the cause. Only the latest GitHub release updates the tap.

The existing `GH_TOKEN` secret must be allowed to push commits to `main`, in addition to creating releases and uploading assets. Configure branch protection or rulesets to permit that identity to push, or the formula update will fail. The repository's default branch must be `main` for Homebrew to pick up the generated formula. No separate release-event trigger is required: generation runs in the same job as the upload. Test the generator locally with `python3 -m unittest discover -s tests -v` (Ruby is needed for platform selection tests).
