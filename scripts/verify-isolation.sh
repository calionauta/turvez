#!/usr/bin/env bash
# verify-isolation.sh — report where every client actually runs.
#
# `runtime:` and `target:` are the fields that decide whether a client shares
# the main stack or has its own host. They used to be written into client.yaml
# and read by nothing, so "is this client isolated?" was answered by memory.
# This script reads the manifest and answers it.
#
# It is READ-ONLY: it reports, it does not move anything. The move itself is
# docs/isolating-a-client.md, packaged by scripts/extract-client.sh.
#
# Usage:
#   ./scripts/verify-isolation.sh                 # whole checkout
#   ./scripts/verify-isolation.sh <client>        # one client
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/verify-isolation.sh
#
# Exit: 0 when every manifest is coherent, 1 when a dedicated client is missing
# its target host or two dedicated clients claim the same host.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${TURVEZ_OPS_DIR:-$(cd "$HERE/.." && pwd)}"
WANT="${1:-}"

CLIENTS_ROOT="$(TURVEZ_OPS_DIR="$ROOT" "$HERE/clients-root.sh")"
[ -d "$CLIENTS_ROOT" ] || { echo "no clients root at $CLIENTS_ROOT" >&2; exit 1; }

# manifest_field <file> <key> — flat `key: value` reader, matching client.yaml's
# documented shape (see parseManifest in clients/recipes_test.go).
manifest_field() {
  [ -f "$1" ] || return 0
  awk -F':' -v k="$2" '
    $0 ~ "^[[:space:]]*"k":" {
      sub(/^[[:space:]]*[^:]*:[[:space:]]*/, "")
      sub(/[[:space:]]*#.*/, "")
      gsub(/^[[:space:]]+|[[:space:]]+$/, "")
      gsub(/^["\047]|["\047]$/, "")
      print; exit
    }' "$1"
}

status=0
rows_shared=()
rows_dedicated=()
# Duplicate-host detection without an associative array: macOS ships bash 3.2,
# which has no `declare -A`, and this script must run there.
host_seen=""

for d in "$CLIENTS_ROOT"/*/; do
  [ -d "$d" ] || continue
  name="$(basename "${d%/}")"
  case "$name" in _template|_templates) continue ;; esac
  [ -n "$WANT" ] && [ "$name" != "$WANT" ] && continue

  manifest="$d/client.yaml"
  if [ ! -f "$manifest" ]; then
    echo "! $name: no client.yaml (phase-1 folder or draft) — runtime unknown"
    continue
  fi

  engine="$(manifest_field "$manifest" engine)"
  runtime="$(manifest_field "$manifest" runtime)"
  host="$(manifest_field "$manifest" host)"
  runtime="${runtime:-shared}"
  host="${host:-}"

  case "$runtime" in
    dedicated)
      if [ -z "$host" ]; then
        echo "FAIL $name: runtime: dedicated but target.host is empty"
        status=1
        rows_dedicated+=("$name|$engine|MISSING-HOST")
        continue
      fi
      if printf '%s\n' "$host_seen" | cut -d'=' -f1 | grep -Fxq "$host"; then
        owner="$(printf '%s\n' "$host_seen" | awk -F= -v h="$host" '$1==h{print $2; exit}')"
        echo "FAIL $name and $owner both claim host $host"
        status=1
      else
        host_seen="${host_seen}${host}=${name}"$'\n'
      fi
      rows_dedicated+=("$name|$engine|$host")
      ;;
    shared)
      rows_shared+=("$name|$engine|shared ($host)")
      ;;
    *)
      echo "FAIL $name: runtime: '$runtime' must be shared or dedicated"
      status=1
      ;;
  esac
done

if [ -n "$WANT" ] && [ ${#rows_shared[@]} -eq 0 ] && [ ${#rows_dedicated[@]} -eq 0 ]; then
  echo "no client '$WANT' under $CLIENTS_ROOT" >&2
  exit 1
fi

printf '%s\n' "---"
printf '%-18s %-10s %s\n' CLIENT ENGINE WHERE
for r in "${rows_shared[@]:-}"; do [ -n "$r" ] && printf '%-18s %-10s %s\n' "${r%%|*}" "$(echo "$r" | cut -d'|' -f2)" "$(echo "$r" | cut -d'|' -f3)"; done
for r in "${rows_dedicated[@]:-}"; do [ -n "$r" ] && printf '%-18s %-10s %s\n' "${r%%|*}" "$(echo "$r" | cut -d'|' -f2)" "$(echo "$r" | cut -d'|' -f3)"; done

echo "---"
if [ "$status" -eq 0 ]; then
  echo "isolation: coherent (${#rows_shared[@]} shared, ${#rows_dedicated[@]} dedicated)"
else
  echo "isolation: FAILED — fix the manifests above before deploying" >&2
fi
exit "$status"
