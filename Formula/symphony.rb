# Homebrew formula for symphony v1.6.0, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "1.6.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.6.0/symphony_darwin_arm64.tar.gz"
      sha256 "a0430b497bc1b44ebe7960588e77a38a4561c8f727c70d728814fe8a9f10ec39"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.6.0/symphony_darwin_amd64.tar.gz"
      sha256 "1941fd4488c2cd0540bd8fe6cde4b36732b58901292c6207c0428b9c209c1d0e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.6.0/symphony_linux_arm64.tar.gz"
      sha256 "f612a84da757f32b1ea9213644d95f63bcf305276bc7a22c47a4c8c4fba24282"
    end
    on_intel do
      url "https://github.com/feynmanzcys-bytes/symphony/releases/download/v1.6.0/symphony_linux_amd64.tar.gz"
      sha256 "fa04fcd3b94bc8da32f1d8f7a85e79a8779ce83988fbeee31c8d4adb87adef6d"
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
