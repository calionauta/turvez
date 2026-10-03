#!/usr/bin/env bash
# Validate and register every example workflow.
# Usage: ./scripts/register-workflows.sh [workflows dir]
set -euo pipefail

DIR="${1:-workflows/example-client}"

command -v dagnats >/dev/null || {
  echo "dagnats CLI not found. Install: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.24" >&2
  exit 1
}

for f in "$DIR"/*.json; do
  echo "== $f"
  dagnats workflow validate "$f"
  dagnats workflow register "$f"
done

echo "Registered. Next: dagnats workflow list"
