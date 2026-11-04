# Homebrew formula for symphony v1.7.3, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.7.3"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.3/symphony_darwin_arm64.tar.gz"
      sha256 "9f9f506bf18fc1f90747db23ccc68961064191aef19244639e6c5cfafb0627fe"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.3/symphony_darwin_amd64.tar.gz"
      sha256 "9a562daeb64d24deeb304273ffba9e67b1c8b5fc44c4e580f6306ce90a8c541c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.3/symphony_linux_arm64.tar.gz"
      sha256 "fe5d1e8a8e054e9a90abd914179917583747371930dca6f589924a220d20c618"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.7.3/symphony_linux_amd64.tar.gz"
      sha256 "7944cc3edc73e16576992db47415b2fa98ba06dfa56a81e8fb7c7c22fd993514"
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
