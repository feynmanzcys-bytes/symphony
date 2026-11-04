# Homebrew formula for symphony v1.8.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.8.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.8.0/symphony_darwin_arm64.tar.gz"
      sha256 "878ec3200fb6f6d3b1a4a4da96fbe4811f63eab4a4dc73479adef084f0ff1e3b"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.8.0/symphony_darwin_amd64.tar.gz"
      sha256 "cdcfb7e4b7cbc63421ca34d03fac13f56290a83f52e2e6b546fdea3dc5e5b421"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.8.0/symphony_linux_arm64.tar.gz"
      sha256 "667bebc35b3309903346d5b01f31254923e1a2dcdcff954623af57f071b90253"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.8.0/symphony_linux_amd64.tar.gz"
      sha256 "d611138d6c742a4e7ec2b370ca7af95c5380f8efecf4d26478d3d5525b892f20"
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
