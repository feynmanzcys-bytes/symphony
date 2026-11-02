#!/usr/bin/env bash
# Writes the Homebrew formula (Formula/symphony.rb) and the Scoop manifest (bucket/symphony.json) of a release into a
# folder, from the release's tag and its checksums.txt (.github/release.sh writes it). They install the release's own
# archives, checked against those SHA-256s; this repository is the tap and the bucket. The release workflow runs it once
# a release is out and commits the two files to main; run it by hand to see what it would write:
#   .github/packages.sh v1.2.0 dist/checksums.txt /tmp/packages
# What it writes depends on the tag and the checksums alone. Every release writes the files again, so change them here.
set -euo pipefail

usage='usage: .github/packages.sh <tag, like v1.2.0> <checksums.txt> <output folder>'
tag=${1:?$usage}
sums=${2:?$usage}
out=${3:?$usage}

release='^v[0-9]+\.[0-9]+\.[0-9]+$' # patterns in variables: bash 3.2 (macOS) reads them the same way
if ! [[ $tag =~ $release ]]; then
  echo "packages.sh: $tag is not a release tag like v1.2.0 (pre-releases get no packages)" >&2
  exit 1
fi
if [ ! -r "$sums" ]; then
  echo "packages.sh: can't read $sums" >&2
  exit 1
fi
version=${tag#v}
download=https://github.com/feynmanzcys-bytes/symphony/releases/download/$tag

# sha <archive> prints the archive's SHA-256 from checksums.txt (sha256sum's or shasum's lines, a "*" before a name
# hashed in binary mode) in lowercase; it fails unless exactly one line names the archive
sha256='^[0-9a-f]{64}$'
sha() {
  local h
  h=$(awk -v name="$1" '$2 == name || $2 == "*" name { n++; h = tolower($1) } END { if (n == 1) print h }' "$sums")
  if ! [[ $h =~ $sha256 ]]; then
    echo "packages.sh: $sums has no single SHA-256 line for $1" >&2
    exit 1
  fi
  printf '%s' "$h"
}
# all of them before anything is written: a failure inside a here-document wouldn't stop the script
linux_amd64=$(sha symphony_linux_amd64.tar.gz)
linux_arm64=$(sha symphony_linux_arm64.tar.gz)
darwin_amd64=$(sha symphony_darwin_amd64.tar.gz)
darwin_arm64=$(sha symphony_darwin_arm64.tar.gz)
windows_amd64=$(sha symphony_windows_amd64.zip)
windows_arm64=$(sha symphony_windows_arm64.zip)

mkdir -p "$out/Formula" "$out/bucket"

# the archives hold the program, LICENSE and README.md at their top level
cat >"$out/Formula/symphony.rb" <<EOF
# Homebrew formula for symphony $tag, written by .github/packages.sh; every release writes it again, so change the
# script, not this file. This repository is the tap:
#   brew tap feynmanzcys-bytes/symphony https://github.com/feynmanzcys-bytes/symphony
#   brew install symphony
class Symphony < Formula
  desc "Unofficial pixel-art office for a team of Claude Code sessions"
  homepage "https://github.com/feynmanzcys-bytes/symphony"
  version "$version"
  license "MIT"

  on_macos do
    on_arm do
      url "$download/symphony_darwin_arm64.tar.gz"
      sha256 "$darwin_arm64"
    end
    on_intel do
      url "$download/symphony_darwin_amd64.tar.gz"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    on_arm do
      url "$download/symphony_linux_arm64.tar.gz"
      sha256 "$linux_arm64"
    end
    on_intel do
      url "$download/symphony_linux_amd64.tar.gz"
      sha256 "$linux_amd64"
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
EOF

# "##" is Scoop's comment key; \$version and \$baseurl are Scoop's own variables, for its autoupdate
cat >"$out/bucket/symphony.json" <<EOF
{
    "##": "Scoop manifest for symphony $tag, written by .github/packages.sh; every release writes it again, so change the script, not this file. This repository is the bucket: scoop bucket add symphony https://github.com/feynmanzcys-bytes/symphony",
    "version": "$version",
    "description": "Unofficial pixel-art office for a team of Claude Code sessions",
    "homepage": "https://github.com/feynmanzcys-bytes/symphony",
    "license": "MIT",
    "notes": [
        "symphony needs its Claude Code mod as well:",
        "  claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony",
        "Stop the bridge before an update (symphony -stop) and start it again afterwards",
        "(symphony -background -open=false, or log in again if it starts at login)."
    ],
    "architecture": {
        "64bit": {
            "url": "$download/symphony_windows_amd64.zip",
            "hash": "$windows_amd64"
        },
        "arm64": {
            "url": "$download/symphony_windows_arm64.zip",
            "hash": "$windows_arm64"
        }
    },
    "bin": "symphony.exe",
    "checkver": {
        "github": "https://github.com/feynmanzcys-bytes/symphony"
    },
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "https://github.com/feynmanzcys-bytes/symphony/releases/download/v\$version/symphony_windows_amd64.zip"
            },
            "arm64": {
                "url": "https://github.com/feynmanzcys-bytes/symphony/releases/download/v\$version/symphony_windows_arm64.zip"
            }
        },
        "hash": {
            "url": "\$baseurl/checksums.txt"
        }
    }
}
EOF

echo "packages.sh: wrote $out/Formula/symphony.rb and $out/bucket/symphony.json for $tag"
