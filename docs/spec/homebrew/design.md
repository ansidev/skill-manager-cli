# Homebrew release integration

## Requirements

Homebrew users can install the binary for macOS or Linux on Intel or ARM and upgrade after releases. The formula must test the reported version. An update must verify all four release archives against the published checksum manifest before changing the formula; missing or invalid inputs must fail without a partial update.

## Design

The existing release job uploads the assets, then downloads those assets from the published release. A Bash updater follows the resume-cli-ce updater's tag, formula path, and local asset interface; it verifies all four archives against the release manifest and atomically writes the formula. The same job commits the generated formula to the default `main` branch, where the source repository can also serve as a Homebrew tap. This avoids relying on a GitHub release event emitted by another workflow. The job serializes releases and only updates the formula for GitHub's latest release. A failed push fails the job; no partial formula is committed.

The release version and four SHA-256 values are inputs; generated URLs refer to the corresponding versioned GitHub assets. The first formula is created by the first release after this integration is present. The release token needs permission to push to `main`, including any branch protection requirements.

## Tasks

- [x] Verify complete release assets and checksums before generating the formula.
- [x] Generate platform-specific URLs and a version-checking Homebrew test.
- [x] Integrate generation after asset upload and publish the formula on `main`.
- [x] Document installation and repository permissions.
- [x] Test all platform selections and rejected missing or corrupt input.
