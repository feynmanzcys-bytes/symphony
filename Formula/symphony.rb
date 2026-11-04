# Homebrew formula for symphony v1.5.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.5.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.5.0/symphony_darwin_arm64.tar.gz"
      sha256 "3db7dc5d34498a63f844097ee72201ee99d84dca8964f9ebd329329370d0f989"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.5.0/symphony_darwin_amd64.tar.gz"
      sha256 "eea134c06dfaa2c83231979da1763498449bffff2dd87608afea2ad589f83923"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.5.0/symphony_linux_arm64.tar.gz"
      sha256 "e563447ae9323de9d30b8a3d5403627f1859c5b282409ba3ac5572a88085b353"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.5.0/symphony_linux_amd64.tar.gz"
      sha256 "2d2af06282ab89ade39d7188a6aa182b0c0b929381d50ff2f8d567d5c5dc1b39"
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
