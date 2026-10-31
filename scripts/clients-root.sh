#!/usr/bin/env bash
# clients-root.sh — print the client root directory of a checkout.
#
# Layouts (docs/client-spine.md):
#   clients/     phase 3, the neutral spine (preferred)
#   workflows/   legacy phase 1, still valid during a migration
#
# Resolution order:
#   1. WORKFLOWS_DIR, when set (an explicit override always wins)
#   2. <TURVEZ_OPS_DIR|repo>/clients, when it exists
#   3. <TURVEZ_OPS_DIR|repo>/workflows, when it exists
#   4. <TURVEZ_OPS_DIR|repo>/clients  (canonical target, may not exist yet)
#
# Usage:
#   ./scripts/clients-root.sh              # print the path
#   ./scripts/clients-root.sh --layout     # print "clients" or "workflows"
#
# Every script that needs the client root calls this, so a layout change stays
# a single edit (the AGENTS.md "one source of truth" rule).
set -euo pipefail

LAYOUT_ONLY=0
if [ "${1:-}" = "--layout" ]; then
  LAYOUT_ONLY=1
fi

ROOT="${TURVEZ_OPS_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

# has_clients <dir> — true when the root holds at least one client folder that
# is not a template. clients/ exists but is empty mid-migration, and then the
# legacy workflows/ root is still the real one.
has_clients() {
  local d
  for d in "$1"/*/; do
    [ -d "$d" ] || continue
    case "$(basename "$d")" in _templates|_template) continue ;; esac
    return 0
  done
  return 1
}

if [ -n "${WORKFLOWS_DIR:-}" ]; then
  if [ "$LAYOUT_ONLY" -eq 1 ]; then printf '%s\n' "$(basename "${WORKFLOWS_DIR%/}")"; else printf '%s\n' "${WORKFLOWS_DIR%/}"; fi
  exit 0
fi

for name in clients workflows; do
  if [ -d "$ROOT/$name" ] && has_clients "$ROOT/$name"; then
    if [ "$LAYOUT_ONLY" -eq 1 ]; then printf '%s\n' "$name"; else printf '%s\n' "$ROOT/$name"; fi
    exit 0
  fi
done

if [ "$LAYOUT_ONLY" -eq 1 ]; then printf 'clients\n'; else printf '%s\n' "$ROOT/clients"; fi
