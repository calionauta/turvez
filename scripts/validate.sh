#!/usr/bin/env bash
# validate.sh — run every engine gate that applies to this checkout.
#
# The repo has two engines (docs/client-spine.md), so `make validate` cannot be
# a single engine's validator: a Windmill-only ops repo has no dagnats recipes
# and must still pass, and a dagnats repo must not be forced to install wmill.
#
# What runs:
#   dagnats   -> `dagnats workflow validate` for every recipe (needs the CLI)
#   windmill  -> `scripts/check-windmill.sh` (needs the wmill CLI)
#
# A gate is skipped, not failed, when its engine has no source in this
# checkout. If client folders exist but NO engine has any source, that is a
# real failure: the checkout is malformed. The per-client structural rules
# (client.yaml present, engine dir matches the manifest, one engine per client)
# are `go test ./clients/`'s job, not this script's.
#
# Usage:
#   ./scripts/validate.sh
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/validate.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${TURVEZ_OPS_DIR:-$(cd "$HERE/.." && pwd)}"

# Both roots may exist during a migration. clients-root.sh picks the live one;
# the legacy root is still scanned so a half-migrated checkout is fully
# validated instead of silently skipping the old files.
LIVE_ROOT="$(TURVEZ_OPS_DIR="$ROOT" "$HERE/clients-root.sh")"
ROOTS=("$LIVE_ROOT")
LEGACY="$ROOT/workflows"
case "$LIVE_ROOT" in */workflows) LEGACY="" ;; esac
[ -n "$LEGACY" ] && [ -d "$LEGACY" ] && ROOTS+=("$LEGACY")

shopt -s nullglob
recipes=()
for root in "${ROOTS[@]}"; do
  [ -d "$root" ] || continue
  for d in "$root"/*/; do
    [ -d "$d" ] || continue
    d="${d%/}"
    # Template sources are validated by `go test` (they carry TODO-CLIENT
    # placeholders); they are not clients and hold no real automation.
    case "$(basename "$d")" in _template|_templates) continue ;; esac
    for f in "$d"/automation/dagnats/*.json; do
      [ -e "$f" ] && recipes+=("$f")
    done
    # Legacy flat layout: <client>/*.json, but never automation/<engine>/*.
    for f in "$d"/*.json; do
      [ -e "$f" ] && recipes+=("$f")
    done
  done
done

runs=0
failed=0

# --- dagnats --------------------------------------------------------------
if [ "${#recipes[@]}" -gt 0 ]; then
  if ! command -v dagnats >/dev/null 2>&1; then
    pinned="$(awk '$1=="github.com/danmestas/dagnats"{print $2}' "$HERE/../go.mod" 2>/dev/null | head -1)"
    echo "validate: ${#recipes[@]} dagnats recipe(s) found but the dagnats CLI is missing" >&2
    echo "  install: go install github.com/danmestas/dagnats/cmd/dagnats@${pinned:-latest}" >&2
    exit 1
  fi
  for f in "${recipes[@]}"; do
    echo "== $f"
    dagnats workflow validate "$f" || failed=1
  done
  runs=$((runs + 1))
fi

# --- windmill -------------------------------------------------------------
wm_dirs=()
for root in "${ROOTS[@]}"; do
  [ -d "$root" ] || continue
  while IFS= read -r d; do
    [ -n "$d" ] && wm_dirs+=("$d")
  done < <(find "$root" -type d -name windmill -prune -print 2>/dev/null)
done

if [ "${#wm_dirs[@]}" -gt 0 ]; then
  echo "== windmill (${#wm_dirs[@]} dir(s))"
  "$HERE/check-windmill.sh" || failed=1
  runs=$((runs + 1))
fi

if [ "$failed" -ne 0 ]; then
  echo "validate: FAILED" >&2
  exit 1
fi

if [ "$runs" -eq 0 ]; then
  # No engine source. Fine for a bare template checkout with no client folders;
  # a malformed checkout when client folders exist.
  clients=0
  for root in "${ROOTS[@]}"; do
    [ -d "$root" ] || continue
    for d in "$root"/*/; do
      [ -d "$d" ] || continue
      case "$(basename "$d")" in _template|_templates) continue ;; esac
      clients=$((clients + 1))
    done
  done
  if [ "$clients" -gt 0 ]; then
    echo "validate: $clients client folder(s) but no automation source" >&2
    echo "  expected <root>/<client>/automation/{dagnats,windmill,dagu}/" >&2
    exit 1
  fi
  echo "validate: no automation source in this checkout (nothing to validate)"
  exit 0
fi

echo "validate: ok (${#recipes[@]} dagnats recipe(s), $runs engine gate(s))"
