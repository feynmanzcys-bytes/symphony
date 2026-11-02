# Homebrew formula for symphony v1.2.3, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.2.3"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.3/symphony_darwin_arm64.tar.gz"
      sha256 "2222222222222222222222222222222222222222222222222222222222222222"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.3/symphony_darwin_amd64.tar.gz"
      sha256 "2121212121212121212121212121212121212121212121212121212121212121"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.3/symphony_linux_arm64.tar.gz"
      sha256 "1212121212121212121212121212121212121212121212121212121212121212"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.3/symphony_linux_amd64.tar.gz"
      sha256 "1111111111111111111111111111111111111111111111111111111111111111"
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
