#!/usr/bin/env bash
# Prints the section of CHANGELOG.md for a version, without its heading and the blank lines around it; fails when
# there is no such section or it is empty. The release workflow puts it at the top of the release's notes:
#   .github/changelog.sh v0.4.0
set -euo pipefail

version=${1:?usage: .github/changelog.sh <version, like v0.4.0>}
root=$(cd "$(dirname "$0")/.." && pwd)
section=$(awk -v v="$version" '
  /^## / { if (found) exit; found = ($2 == v); next }
  found { print }
' "$root/CHANGELOG.md" | sed -e '/./,$!d') # blank lines before it go here, and $(...) drops those after it
if [ -z "$section" ]; then
  echo "CHANGELOG.md has no section for $version (a heading like \"## $version (date)\" and the changes below it)" >&2
  exit 1
fi
printf '%s\n' "$section"
