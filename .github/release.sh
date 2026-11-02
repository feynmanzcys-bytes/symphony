#!/usr/bin/env bash
# Builds the symphony program (the bridge, with the pages built in) for every platform a release offers, packs
# each with the license and the README, and writes checksums.txt. The release workflow runs it; run it by hand
# to see what a release would hold:
#   .github/release.sh v0.3.0 dist
# Archive names carry no version, so .../releases/latest/download/<archive> always points at the newest.
set -euo pipefail

version=${1:?usage: .github/release.sh <version> [output folder]}
out=${2:-dist}
targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The same commit gives the same archives (Go's builds are reproducible with -trimpath; the archives too once their
# files have one time, one owner and fixed permissions): every file gets the commit's time (SOURCE_DATE_EPOCH when
# set), and a GNU tar (the release runs on Linux) records owner 0 in name order with gzip -n. Anyone can rebuild an
# archive on Linux and compare it with the release's checksums.
when=${SOURCE_DATE_EPOCH:-$(git -C "$root" log -1 --format=%ct 2>/dev/null || echo 0)}
stamp=$(TZ=UTC date -u -d "@$when" +%Y%m%d%H%M.%S 2>/dev/null || TZ=UTC date -u -r "$when" +%Y%m%d%H%M.%S)
gnutar=false
if tar --version 2>/dev/null | grep -q 'GNU tar'; then gnutar=true; fi

archives=()
for target in $targets; do
  os=${target%/*}
  arch=${target#*/}
  name=symphony_${os}_$arch
  exe=symphony
  if [ "$os" = windows ]; then exe=symphony.exe; fi
  mkdir "$work/$name"
  (cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/$name/$exe" ./cmd/bridge)
  cp "$root/LICENSE" "$root/README.md" "$work/$name/"
  chmod 755 "$work/$name/$exe"
  chmod 644 "$work/$name/LICENSE" "$work/$name/README.md"
  TZ=UTC touch -t "$stamp" "$work/$name/$exe" "$work/$name/LICENSE" "$work/$name/README.md"
  if [ "$os" = windows ]; then
    rm -f "$out/$name.zip"
    (cd "$work/$name" && TZ=UTC zip -q -X "$out/$name.zip" "$exe" LICENSE README.md)
    archives+=("$name.zip")
  elif $gnutar; then
    tar -C "$work/$name" --sort=name --owner=0 --group=0 --numeric-owner --format=gnu -cf - "$exe" LICENSE README.md |
      gzip -9n >"$out/$name.tar.gz"
    archives+=("$name.tar.gz")
  else # macOS's bsdtar, for a local look: the same contents, not the same bytes
    tar -C "$work/$name" --uid 0 --gid 0 -cf - "$exe" LICENSE README.md | gzip -9n >"$out/$name.tar.gz"
    archives+=("$name.tar.gz")
  fi
done

cd "$out"
if command -v sha256sum >/dev/null; then
  sha256sum "${archives[@]}" >checksums.txt
else
  shasum -a 256 "${archives[@]}" >checksums.txt # macOS
fi
ls -l "${archives[@]}" checksums.txt
