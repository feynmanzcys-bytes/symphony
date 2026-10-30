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
  if [ "$os" = windows ]; then
    rm -f "$out/$name.zip"
    (cd "$work/$name" && zip -q -X "$out/$name.zip" "$exe" LICENSE README.md)
    archives+=("$name.zip")
  else
    tar -C "$work/$name" -czf "$out/$name.tar.gz" "$exe" LICENSE README.md
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
