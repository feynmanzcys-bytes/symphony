# Homebrew formula for symphony v1.2.2, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.2.2"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.2/symphony_darwin_arm64.tar.gz"
      sha256 "03926b5eddf6b1f2c778df7e4c09a510b11b21c3158a20115a41d0feba1f3e82"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.2/symphony_darwin_amd64.tar.gz"
      sha256 "820cc00a869f10af21ff0752bdc87e9d5d87fc162c8f1779e614e8eadf86bfaa"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.2/symphony_linux_arm64.tar.gz"
      sha256 "cd581a72f547c24c8da2d52952f508b68a9dd5bec3fab788653f8d4044d9e06e"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.2.2/symphony_linux_amd64.tar.gz"
      sha256 "65bec29e663a0722f5f8ddd5ece336ed4954a8c2237cb4369a69603a48469e38"
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
