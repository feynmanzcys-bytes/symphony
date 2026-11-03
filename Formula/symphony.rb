# Homebrew formula for symphony v1.2.1, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.2.1"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.1/symphony_darwin_arm64.tar.gz"
      sha256 "6759fda42ceb56271f89cad71b7479ac7d3eb43528501da666eebb36ed16702a"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.1/symphony_darwin_amd64.tar.gz"
      sha256 "cfd72e96022c79f22f85fb827648ea37e15691c2461acb94b4c5da0e7985bee4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.1/symphony_linux_arm64.tar.gz"
      sha256 "b545d04715878f621526d947dbd879b94d2347326268228a2c5a8308b889376d"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.1/symphony_linux_amd64.tar.gz"
      sha256 "ad4c3d1a18865c0d1d8b37a7bd88be2e1dd89d66f622b2d1c725ff2ae9403af7"
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
