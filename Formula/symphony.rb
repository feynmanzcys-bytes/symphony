# Homebrew formula for symphony v1.4.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.4.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.4.0/symphony_darwin_arm64.tar.gz"
      sha256 "0101036957cbdd8bf60e1ed618e8d5d119aaafd62ab7d578128d4b2a43b8d06d"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.4.0/symphony_darwin_amd64.tar.gz"
      sha256 "cdd4593bdf1ca60d9aa81655188c5fc375a6428c72b7d11325e21f8e19190ae9"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.4.0/symphony_linux_arm64.tar.gz"
      sha256 "e9f3dbdaf1060690ffffe192f0df9726db22ebe7920fabb560f44330f0fe351c"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.4.0/symphony_linux_amd64.tar.gz"
      sha256 "49f6ca3934a0f74c8e70156cd008b80622fe6c3d417f1389a6fe932c9864d9c1"
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
