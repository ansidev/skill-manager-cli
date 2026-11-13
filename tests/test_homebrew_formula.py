import hashlib
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / ".github" / "scripts" / "update-formula.sh"
VERSION = "1.2.3"


class FormulaUpdateTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)
        self.output = self.directory / "formula.rb"
        self.entries = []
        for platform in ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"):
            name = f"skill-manager-cli_{VERSION}_{platform}.tar.gz"
            data = platform.encode()
            (self.directory / name).write_bytes(data)
            self.entries.append(f"{hashlib.sha256(data).hexdigest()}  {name}\n")
        self.checksums = self.directory / f"skill-manager-cli_{VERSION}_checksums.txt"
        self.checksums.write_text("".join(self.entries))

    def run_update(self):
        return subprocess.run(["bash", str(SCRIPT), f"v{VERSION}", str(self.output)], env={**os.environ, "GITHUB_REPOSITORY": "ansidev/skill-manager-cli", "ASSET_DIR": str(self.directory)}, capture_output=True, text=True)

    def test_generates_formula_for_all_verified_assets(self):
        result = self.run_update()
        self.assertEqual(result.returncode, 0, result.stderr)
        formula = self.output.read_text()
        for entry in self.entries:
            digest, name = entry.split()
            self.assertIn(f"https://github.com/ansidev/skill-manager-cli/releases/download/v1.2.3/{name}", formula)
            self.assertIn(digest, formula)
        self.assertIn('version "1.2.3"', formula)
        self.assertIn('--version', formula)

    @unittest.skipUnless(shutil.which("ruby"), "Ruby is needed to evaluate Homebrew formulae")
    def test_formula_selects_matching_url_and_checksum_on_each_platform(self):
        self.assertEqual(self.run_update().returncode, 0)
        harness = r'''
class Formula
  def self.desc(*); end
  def self.homepage(*); end
  def self.version(*); end
  def self.license(*); end
  def self.on_macos(&block); block.call if ENV["OS"] == "darwin"; end
  def self.on_linux(&block); block.call if ENV["OS"] == "linux"; end
  def self.on_intel(&block); block.call if ENV["ARCH"] == "amd64"; end
  def self.on_arm(&block); block.call if ENV["ARCH"] == "arm64"; end
  def self.url(value); puts(value); end
  def self.sha256(value); puts(value); end
  def self.test(&block); end
end
load ARGV.fetch(0)
'''
        for platform, entry in zip(("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"), self.entries):
            system, arch = platform.split("_")
            digest, name = entry.split()
            result = subprocess.run(["ruby", "-e", harness, str(self.output)], env={**os.environ, "OS": system, "ARCH": arch}, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), [f"https://github.com/ansidev/skill-manager-cli/releases/download/v1.2.3/{name}", digest])

    def test_missing_asset_preserves_existing_formula(self):
        self.output.write_text("old formula")
        (self.directory / f"skill-manager-cli_{VERSION}_linux_arm64.tar.gz").unlink()
        self.assertNotEqual(self.run_update().returncode, 0)
        self.assertEqual(self.output.read_text(), "old formula")

    def test_checksum_mismatch_preserves_existing_formula(self):
        self.output.write_text("old formula")
        (self.directory / f"skill-manager-cli_{VERSION}_darwin_amd64.tar.gz").write_bytes(b"tampered")
        self.assertNotEqual(self.run_update().returncode, 0)
        self.assertEqual(self.output.read_text(), "old formula")

    def test_missing_manifest_preserves_existing_formula(self):
        self.output.write_text("old formula")
        self.checksums.unlink()
        self.assertNotEqual(self.run_update().returncode, 0)
        self.assertEqual(self.output.read_text(), "old formula")

    def test_invalid_tag_is_rejected(self):
        result = subprocess.run(["bash", str(SCRIPT), "v1.2.3/invalid", str(self.output)], env={**os.environ, "ASSET_DIR": str(self.directory)}, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.output.exists())

    def test_missing_or_duplicate_checksum_is_rejected(self):
        for contents in ("".join(self.entries[:-1]), "".join(self.entries + [self.entries[0]])):
            with self.subTest(contents=contents):
                self.checksums.write_text(contents)
                self.assertNotEqual(self.run_update().returncode, 0)
                self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
