#!/usr/bin/env bash
# check-windmill.sh — test gate for Windmill clients, no worker required.
#
# Runs `wmill lint --fail-on-warn --locks-required` over every Windmill
# directory in the checkout. Lint validates flows/schedules/triggers against
# the OpenFlow/API schemas and fails on missing lockfiles; it does not execute
# scripts. Go logic is covered by `go test ./...` separately.
#
# Usage:
#   ./scripts/check-windmill.sh
#   WMILL_CONFIG_DIR=/tmp/ci-wmill ./scripts/check-windmill.sh
#
# Exits 0 when there is nothing to check (dagnats-only checkout) or all lint
# passes; exits 1 when wmill is missing but a Windmill dir exists, or lint fails.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DIRS="$(find workflows clients -type d -name windmill -prune -print 2>/dev/null | sort || true)"

if [ -z "$DIRS" ]; then
  echo "windmill: no windmill directories — nothing to lint"
  exit 0
fi

if ! command -v wmill >/dev/null 2>&1; then
  echo "windmill: wmill CLI is required to lint Windmill clients" >&2
  echo "install: npm install -g windmill-cli" >&2
  exit 1
fi

fail=0
count=0
for dir in $DIRS; do
  count=$((count + 1))
  echo "== wmill lint $dir"
  if ! wmill lint "$dir" --fail-on-warn --locks-required; then
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "windmill: lint failed" >&2
  exit 1
fi
echo "windmill: lint ok ($count dir(s))"
