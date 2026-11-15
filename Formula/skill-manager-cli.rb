class SkillManagerCli < Formula
  desc "Manage agent skills"
  homepage "https://github.com/ansidev/skill-manager-cli"
  version "1.1.0"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.1.0/skill-manager-cli_1.1.0_darwin_amd64.tar.gz"
      sha256 "791f980e144c061956c3a6193ff0c002d9c32093ab37fa89c5e3ddd43762612a"
    end
    on_arm do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.1.0/skill-manager-cli_1.1.0_darwin_arm64.tar.gz"
      sha256 "670c664600d56765489c197fe07783a1f6dc586e59ae126867c2b37b66513558"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.1.0/skill-manager-cli_1.1.0_linux_amd64.tar.gz"
      sha256 "35577cb4ae8887beb480d59f8e5de3f85e4666bf5c216260bf2bf09ef89a3517"
    end
    on_arm do
      url "https://github.com/ansidev/skill-manager-cli/releases/download/v1.1.0/skill-manager-cli_1.1.0_linux_arm64.tar.gz"
      sha256 "eaa8ba917c693b90cb29d09135d30810981f312320e3f4511f7119ade247d1ad"
    end
  end

  def install
    bin.install "skill-manager-cli"
  end

  test do
    assert_match "skill-manager-cli version #{version} (commit ", shell_output("#{bin}/skill-manager-cli --version")
  end
end
