# Homebrew formula for symphony v1.2.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.2.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.0/symphony_darwin_arm64.tar.gz"
      sha256 "3e2d21837c791d69c51a7fb2f51a1e2037edc98f9410ef79c5035a012154efa3"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.0/symphony_darwin_amd64.tar.gz"
      sha256 "c7e66c909e4e4a07b4dfe8940c1aab3a3db829177b52d08ebf3e6fffed3bd2a2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.0/symphony_linux_arm64.tar.gz"
      sha256 "3b55f5b43bd5589044e97237fd66c0555bfbdf99d57f28cd9fabaf4440942158"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.0/symphony_linux_amd64.tar.gz"
      sha256 "817dfec98bba9df22c9d2abf50e8d67b0347f1b1e59a2e318e26ef788ead4b1e"
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
