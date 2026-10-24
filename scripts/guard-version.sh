#!/usr/bin/env bash
# Policy: the project stays 0.x until a human explicitly approves 1.0.0.
# release-please's bump-minor-pre-major keeps breaking changes in 0.x; this
# guard catches a manual manifest change that would cross 1.0.0.
#
# To promote intentionally: change this guard in the same reviewed commit.
set -euo pipefail

MANIFEST="${1:-.release-please-manifest.json}"
[ -f "$MANIFEST" ] || { echo "guard-version: $MANIFEST not found" >&2; exit 1; }

version="$(jq -r '."."' "$MANIFEST")"
[ -n "$version" ] && [ "$version" != "null" ] || {
  echo "guard-version: no \".\" version in $MANIFEST" >&2
  exit 1
}

major="${version%%.*}"
case "$major" in
  ''|*[!0-9]*) echo "guard-version: cannot parse major from $version" >&2; exit 1 ;;
esac

if [ "$major" -ge 1 ]; then
  cat >&2 <<MSG
guard-version: $version is >= 1.0.0.
This project stays 0.x until a human explicitly asks for 1.0.0 (AGENTS.md).
To promote intentionally, change this guard in the same reviewed commit.
MSG
  exit 1
fi

echo "guard-version: $version is pre-1.0 (ok)"
