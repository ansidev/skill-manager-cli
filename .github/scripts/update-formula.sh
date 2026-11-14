#!/usr/bin/env bash
# Generate Formula/skill-manager-cli.rb for a release tag.
# Usage: update-formula.sh <tag> [formula-path]
# Env: GITHUB_REPOSITORY (default ansidev/skill-manager-cli), ASSET_DIR (use local assets).
set -euo pipefail

tag="${1:?usage: update-formula.sh <tag> [formula-path]}"
formula="${2:-Formula/skill-manager-cli.rb}"
repo="${GITHUB_REPOSITORY:-ansidev/skill-manager-cli}"
version="${tag#v}"
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ || ! "$repo" =~ ^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$ ]]; then
  echo "Invalid release tag or repository" >&2
  exit 1
fi

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

workdir="$(mktemp -d)"
temporary=""
trap 'rm -rf "$workdir"; if [ -n "$temporary" ]; then rm -f "$temporary"; fi' EXIT

get_asset() {
  if [ -n "${ASSET_DIR:-}" ]; then
    cp "${ASSET_DIR}/$1" "${workdir}/$1"
  else
    curl -fsSL --retry 3 -o "${workdir}/$1" "https://github.com/${repo}/releases/download/${tag}/$1"
  fi
}

prefix="skill-manager-cli_${version}"
manifest="${prefix}_checksums.txt"
get_asset "$manifest"
for target in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
  asset="${prefix}_${target}.tar.gz"
  get_asset "$asset"
  expected="$(awk -v name="$asset" '
    $2 == name || $2 == "*" name { count++; hash = $1 }
    END { if (count != 1 || length(hash) != 64 || hash !~ /^[[:xdigit:]]+$/) exit 1; print tolower(hash) }
  ' "${workdir}/${manifest}")" || { echo "Missing, duplicate, or invalid checksum: $asset" >&2; exit 1; }
  actual="$(sha256 "${workdir}/${asset}")"
  if [ "$expected" != "$actual" ]; then
    echo "Checksum mismatch: $asset" >&2
    exit 1
  fi
  case "$target" in
    darwin_arm64) darwin_arm64="$actual" ;;
    darwin_amd64) darwin_amd64="$actual" ;;
    linux_arm64) linux_arm64="$actual" ;;
    linux_amd64) linux_amd64="$actual" ;;
  esac
done

mkdir -p "$(dirname "$formula")"
temporary="$(mktemp "${formula}.XXXXXX")"
cat > "$temporary" <<EOF
class SkillManagerCli < Formula
  desc "Manage agent skills"
  homepage "https://github.com/ansidev/skill-manager-cli"
  version "$version"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/$repo/releases/download/$tag/${prefix}_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
    on_arm do
      url "https://github.com/$repo/releases/download/$tag/${prefix}_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/$repo/releases/download/$tag/${prefix}_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
    end
    on_arm do
      url "https://github.com/$repo/releases/download/$tag/${prefix}_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
  end

  def install
    bin.install "skill-manager-cli"
  end

  test do
    assert_match "skill-manager-cli version #{version} (commit ", shell_output("#{bin}/skill-manager-cli --version")
  end
end
EOF
mv "$temporary" "$formula"
temporary=""
