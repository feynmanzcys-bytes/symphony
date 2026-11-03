# Homebrew formula for symphony v1.3.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.3.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.3.0/symphony_darwin_arm64.tar.gz"
      sha256 "52e144f263a9a64aa99bd995e709b719d6fdb3bbf76fd1b3595e9703673fa45f"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.3.0/symphony_darwin_amd64.tar.gz"
      sha256 "28cd96d8bb818346c4620779791bafe7de54d90c0ada5de3c46a41a451579a20"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.3.0/symphony_linux_arm64.tar.gz"
      sha256 "76c2d318865d56d5b497838c7fb7bba9fa84a898d080bc9066d595bf6dab6b6a"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.3.0/symphony_linux_amd64.tar.gz"
      sha256 "e6b80103aa70369f80d833671677fd7d62cd7a6ca37f7f8fd0fb6acb072fde48"
    end
  end

  def install
    bin.install "symphony"
  end

  def caveats
    <<~EOS
      symphony needs its Claude Code mod as well:
        claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony
      Stop the bridge before an upgrade (symphony -stop) and start it again afterwards (symphony
      -background -open=false, or log in again if it starts at login).
    EOS
  end

  test do
    assert_match "symphony v#{version} (", shell_output("#{bin}/symphony -version")
  end
end
