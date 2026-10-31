#!/usr/bin/env bash
# Validate and register workflows, optionally filtered to one client.
#
# Supports BOTH layouts (docs/client-spine.md):
#   phase 1: workflows/<client>/*.json
#   phase 3: clients/<client>/automation/dagnats/*.json
#
# Usage:
#   ./scripts/register-workflows.sh              # all clients
#   ./scripts/register-workflows.sh <client-id>   # only that client
#   CLIENT_ID=acme ./scripts/register-workflows.sh
#
# Tenancy rule (stage 2): every workflow name must start with
# "<client-id><separator>" matching its folder. The separator comes from
# naming.json at the template root. Registration aborts otherwise, so a
# misnamespaced recipe never reaches the shared engine.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEP="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["client_workflow_separator"])' "$ROOT/naming.json")"
CLIENT="${1:-${CLIENT_ID:-}}"
# Data-only ops repos: WORKFLOWS_DIR (or TURVEZ_OPS_DIR) points at the private
# checkout. It may be either root: .../clients (phase 3) or .../workflows
# (legacy phase 1). Default = this repo's clients/, falling back to workflows/
# when clients/ holds no client recipes yet (mid-migration checkout).
WORKFLOWS_DIR="${WORKFLOWS_DIR:-${TURVEZ_OPS_DIR:+$(cd "${TURVEZ_OPS_DIR}" && "$ROOT/scripts/clients-root.sh")}}"
WORKFLOWS_DIR="${WORKFLOWS_DIR:-$("$ROOT/scripts/clients-root.sh")}"

command -v dagnats >/dev/null || {
  echo "dagnats CLI not found. Install: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30" >&2
  exit 1
}
command -v python3 >/dev/null || {
  echo "python3 is required for the namespace check" >&2
  exit 1
}

# client_recipes <root> <client> — echo the recipes dir for one client, in
# either layout. Matches on *.json so a bare client folder (evals only) is
# not mistaken for a recipe dir.
client_recipes() {
  local root="$1" client="$2" c
  for c in \
    "$root/$client/automation/dagnats" \
    "$root/$client" \
    "$root/clients/$client/automation/dagnats" \
    "$root/clients/$client"; do
    if [ -d "$c" ] && compgen -G "$c/*.json" >/dev/null; then
      printf '%s\n' "$c"
      return 0
    fi
  done
  return 1
}

# client_root <recipes-dir> — echo the client folder that owns client.yaml
# and drives the tenancy prefix.
client_root() {
  case "$1" in
    */automation/dagnats) printf '%s\n' "${1%/automation/dagnats}" ;;
    *) printf '%s\n' "$1" ;;
  esac
}

# all_recipes <root> — every client recipes dir under either layout.
all_recipes() {
  local root="$1" d
  for d in "$root"/*/; do
    d="${d%/}"
    [ -d "$d" ] || continue
    case "$(basename "$d")" in
      _templates|_template) continue ;;
    esac
    if [ -d "$d/automation/dagnats" ] && compgen -G "$d/automation/dagnats/*.json" >/dev/null; then
      printf '%s\n' "$d/automation/dagnats"
    elif compgen -G "$d/*.json" >/dev/null; then
      printf '%s\n' "$d"
    fi
  done
}

DIRS=()
if [ -n "$CLIENT" ]; then
  resolved="$(client_recipes "$WORKFLOWS_DIR" "$CLIENT" || true)"
  [ -n "$resolved" ] || {
    echo "unknown client: no recipes for '$CLIENT' under $WORKFLOWS_DIR (workflows/<client>/ or clients/<client>/automation/dagnats/)" >&2
    exit 1
  }
  DIRS+=("$resolved")
else
  while IFS= read -r d; do
    [ -n "$d" ] && DIRS+=("$d")
  done < <(all_recipes "$WORKFLOWS_DIR")
fi

if [ "${#DIRS[@]}" -eq 0 ]; then
  echo "no client recipes under $WORKFLOWS_DIR (nothing to register)"
  exit 0
fi

fail=0
for dir in "${DIRS[@]}"; do
  croot="$(client_root "$dir")"
  # Engine gate: a client.yaml with engine != dagnats is not registered here.
  if [ -f "$croot/client.yaml" ]; then
    engine="$(awk -F': *' '/^engine:/{print $2; exit}' "$croot/client.yaml" | sed 's/#.*//; s/[[:space:]]//g')"
    if [ -n "$engine" ] && [ "$engine" != "dagnats" ]; then
      echo "SKIP $croot: engine=$engine (not dagnats)"
      continue
    fi
  fi
  expected="$(basename "$croot")"
  for f in "$dir"/*.json; do
    [ -e "$f" ] || continue
    echo "== $f"
    dagnats workflow validate "$f"
    name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["name"])' "$f")"
    case "$name" in
      "$expected$SEP"*) ;;
      *)
        echo "TENANCY VIOLATION: $f names workflow '$name', want '$expected$SEP…'" >&2
        fail=1
        continue
        ;;
    esac
    dagnats workflow register "$f"
  done
done

[ "$fail" -eq 0 ] || { echo "registration aborted: namespace violations above" >&2; exit 1; }
echo "Registered. Next: dagnats workflow list"
