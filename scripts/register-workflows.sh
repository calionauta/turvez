#!/usr/bin/env bash
# Validate and register workflows, optionally filtered to one client.
#
# Usage:
#   ./scripts/register-workflows.sh              # all clients
#   ./scripts/register-workflows.sh <client-id>   # only workflows/<client-id>
#   CLIENT_ID=acme ./scripts/register-workflows.sh
#
# Tenancy rule (stage 2): every workflow name must start with
# "<client-id>/" matching its folder. Registration aborts otherwise —
# a misnamespaced recipe never reaches the shared engine.
set -euo pipefail

CLIENT="${1:-${CLIENT_ID:-}}"

command -v dagnats >/dev/null || {
  echo "dagnats CLI not found. Install: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.24" >&2
  exit 1
}
command -v python3 >/dev/null || {
  echo "python3 is required for the namespace check" >&2
  exit 1
}

if [ -n "$CLIENT" ]; then
  DIRS=("workflows/$CLIENT")
  [ -d "workflows/$CLIENT" ] || { echo "unknown client: workflows/$CLIENT" >&2; exit 1; }
else
  DIRS=(workflows/*/)
fi

fail=0
for dir in "${DIRS[@]}"; do
  # Skip the templates dir (TODO-CLIENT placeholders, not registerable).
  case "$dir" in workflows/_templates/*) continue;; esac
  expected="$(basename "$dir")"
  for f in "$dir"/*.json; do
    [ -e "$f" ] || continue
    echo "== $f"
    dagnats workflow validate "$f"
    name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["name"])' "$f")"
    case "$name" in
      "$expected"/*) ;;
      *)
        echo "TENANCY VIOLATION: $f names workflow '$name', want '$expected/…'" >&2
        fail=1
        continue
        ;;
    esac
    dagnats workflow register "$f"
  done
done

[ "$fail" -eq 0 ] || { echo "registration aborted: namespace violations above" >&2; exit 1; }
echo "Registered. Next: dagnats workflow list"
