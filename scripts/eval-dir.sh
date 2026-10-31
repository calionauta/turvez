#!/usr/bin/env bash
# eval-dir.sh — print the eval directory for one client, in either layout.
#
#   clients/<client>/evals     (phase 3, docs/client-spine.md)
#   evals/<client>             (legacy phase 1)
#
# Root resolution: TURVEZ_OPS_DIR wins (private ops checkout), else the
# template checkout this script lives in.
#
# Usage:
#   ./scripts/eval-dir.sh <client>          # print the path
#   ./scripts/eval-dir.sh --exists <client> # exit 1 when neither layout exists
#
# One source of truth for `make eval-client`, `eval-loop` and the skill, so a
# layout change is a single edit.
set -euo pipefail

EXISTS_ONLY=0
if [ "${1:-}" = "--exists" ]; then
  EXISTS_ONLY=1
  shift
fi

[ -n "${1:-}" ] || { echo "Usage: eval-dir.sh [--exists] <client>" >&2; exit 2; }
CLIENT="$1"
[[ "$CLIENT" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad client id: $CLIENT" >&2; exit 2; }

ROOT="${TURVEZ_OPS_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

for candidate in "$ROOT/clients/$CLIENT/evals" "$ROOT/evals/$CLIENT"; do
  if [ -d "$candidate" ]; then
    if [ "$EXISTS_ONLY" -eq 1 ]; then exit 0; fi
    printf '%s\n' "$candidate"
    exit 0
  fi
done

if [ "$EXISTS_ONLY" -eq 1 ]; then
  exit 1
fi

# Neither layout exists: print the canonical target so the caller can create
# it, and say so on stderr.
echo "no eval dir for '$CLIENT' under $ROOT (looked for clients/$CLIENT/evals and evals/$CLIENT)" >&2
printf '%s\n' "$ROOT/clients/$CLIENT/evals"
exit 0
