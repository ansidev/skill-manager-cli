class SkillManagerCli < Formula
  desc "Manage agent skills"
  homepage "https://github.com/ansidev/skill-manager-cli"
  version "1.0.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.0.0/skill-manager-cli_1.0.0_darwin_amd64.tar.gz"
      sha256 "271a7ebb133d94859c757ca125b1aaac764558c6630c77dd3bf0c100dae4b377"
    end
    on_arm do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.0.0/skill-manager-cli_1.0.0_darwin_arm64.tar.gz"
      sha256 "7c372a26173ffc94f221d47282ed9222229d823434dd9dda829e319bd5dc14a3"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.0.0/skill-manager-cli_1.0.0_linux_amd64.tar.gz"
      sha256 "fe6e74ca5b7eae93ab5284ed4e1114beadeb699cc88f9fe4dd9804c300403991"
    end
    on_arm do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.0.0/skill-manager-cli_1.0.0_linux_arm64.tar.gz"
      sha256 "8277b3f269bb9be82858d7013bad2961de067eb2aa4dbf90bd0601a6ca7d8c41"
    end
  end

  def install
    bin.install "skill-manager-cli"
  end

  test do
    assert_match "skill-manager-cli version #{version} (commit ", shell_output("#{bin}/skill-manager-cli --version")
  end
end
