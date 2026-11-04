# Homebrew formula for symphony v1.7.2, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.7.2"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.2/symphony_darwin_arm64.tar.gz"
      sha256 "e58e0e5088182a8715fe11f3ba45fddddf15f28fde5c120af73e2d64bc831b57"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.2/symphony_darwin_amd64.tar.gz"
      sha256 "ceba484ba63aacbc2bb50ec4dab8c285f8874039e79453cf5c96ff59c5ac10b9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.2/symphony_linux_arm64.tar.gz"
      sha256 "39d501d2c323ed81774c07ba5f283c0add9389cb11309b59df118f094fc9c94b"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.2/symphony_linux_amd64.tar.gz"
      sha256 "bf47fba5d8fa832d64eb65c00b8654cfc7f2ecf49c873ff0a14d27ffd986f59e"
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
