#!/usr/bin/env bash
# check-dagnats-pin.sh — one engine version, everywhere it is written down.
#
# go.mod is the pin for the Go libraries the worker links. Dockerfile.dagnats
# builds the *server* from an upstream tag, and scripts/setup.sh tells a
# developer which CLI to install. A server older than the client libraries
# rejects recipe fields the tests accept (the console route shape changed in
# v0.0.32), and the failure shows up at run time, not at build time.
#
# Ops repos (TURVEZ_PIN, no go.mod) pin deliberately to a release: nothing to
# compare there, so the check skips instead of guessing.
#
# Usage: ./scripts/check-dagnats-pin.sh [root]
set -euo pipefail

ROOT="${1:-$(cd "$(dirname "$0")/.." && pwd)}"

if [ ! -f "$ROOT/go.mod" ]; then
  if [ -f "$ROOT/TURVEZ_PIN" ]; then
    echo "dagnats pin: ops repo (TURVEZ_PIN owns the version) — nothing to compare"
    exit 0
  fi
  echo "dagnats pin: no go.mod in $ROOT" >&2
  exit 1
fi

want="$(cd "$ROOT" && go list -m -f '{{.Version}}' github.com/danmestas/dagnats 2>/dev/null || true)"
if [ -z "$want" ]; then
  echo "dagnats pin: go.mod does not require github.com/danmestas/dagnats" >&2
  exit 1
fi

failed=0
report() { # <file> <found>
  if [ "$2" != "$want" ]; then
    echo "dagnats pin: $1 says ${2:-<none>}, go.mod says $want" >&2
    failed=1
  fi
}

docker_v="$(grep -oE 'ARG DAGNATS_VERSION=v[0-9]+\.[0-9]+\.[0-9]+' "$ROOT/Dockerfile.dagnats" | head -1 | sed 's/.*=//')"
report Dockerfile.dagnats "${docker_v:-}"

setup_v="$(grep -oE 'want v[0-9]+\.[0-9]+\.[0-9]+' "$ROOT/scripts/setup.sh" | head -1 | sed 's/want //')"
report scripts/setup.sh "${setup_v:-}"

# setup.sh mentions the version on several lines (install URL, comment,
# comparison). A stale install URL hands the developer an older CLI than the
# one the recipes are validated against, so scan them all.
strays="$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' "$ROOT/scripts/setup.sh" | sort -u | grep -vx "$want" || true)"
if [ -n "$strays" ]; then
  echo "dagnats pin: scripts/setup.sh also mentions $(echo "$strays" | tr '\n' ' ')— every copy must be $want" >&2
  failed=1
fi

ops_v="$(grep -oE 'DAGNATS_VERSION=v[0-9]+\.[0-9]+\.[0-9]+' "$ROOT/docs/ops-repo.md" | head -1 | sed 's/.*=//')"
report docs/ops-repo.md "${ops_v:-}"

if [ "$failed" -ne 0 ]; then
  echo "dagnats pin: align every copy with go.mod ($want) — the worker links one engine, the server container must run the same one" >&2
  exit 1
fi
echo "dagnats pin: $want (go.mod, Dockerfile.dagnats, setup.sh, ops-repo example agree)"
