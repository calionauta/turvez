#!/usr/bin/env bash
# migrate-to-clients.sh — move the phase-1 layout (workflows/<client>) onto
# the neutral client spine (clients/<client>). See docs/client-spine.md.
#
# Mapping (only these paths move):
#   workflows/<client>/*.json               -> clients/<client>/automation/dagnats/
#   workflows/<client>/{README.md,client.yaml} -> clients/<client>/
#   workflows/_templates                    -> clients/_template
#   evals/<client>                          -> clients/<client>/evals
#
# The client loop skips _templates. evals/_template moves into
# clients/_template/evals so the eval skeleton travels with the rest of the
# template. Other files inside a client folder (windmill/, dagu/, …)
# are NOT mapped: they are reported so they can be moved by hand, because the
# engine decision belongs in client.yaml.
#
# Usage:
#   ./scripts/migrate-to-clients.sh            # --check (default): print the plan
#   ./scripts/migrate-to-clients.sh --check
#   ./scripts/migrate-to-clients.sh --apply    # perform the moves
#
# Moves use `git mv` when the path is tracked and `mv` otherwise, so an ops
# repo and a plain checkout both work. Idempotent: a fully migrated tree has
# an empty plan. --check exits 0 even when a plan exists (it is a report);
# --apply aborts on a destination that already exists.
set -euo pipefail

MODE="check"
for arg in "$@"; do
  case "$arg" in
    --check) MODE="check" ;;
    --apply) MODE="apply" ;;
    -h|--help)
      sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown argument: $arg (use --check or --apply)" >&2
      exit 2
      ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SRCS=()
DSTS=()
NOTES=()

queue() {
  SRCS+=("$1")
  DSTS+=("$2")
}

note() {
  NOTES+=("$1")
}

# print_notes walks by index: bash 3.2 chokes on "${arr[@]}" when empty with set -u.
print_notes() {
  local j=0
  while [ "$j" -lt "${#NOTES[@]}" ]; do
    echo "  note: ${NOTES[$j]}"
    j=$((j + 1))
  done
}

# --- templates -------------------------------------------------------------
if [ -d "workflows/_templates" ]; then
  queue "workflows/_templates" "clients/_template"
fi
# The eval skeleton belongs to the template too: clients/_template/evals is
# what new-client.sh copies. A pre-existing clients/_template/evals wins
# (nothing to do then; the plan simply omits the move).
if [ -d "evals/_template" ] && [ ! -d "clients/_template/evals" ]; then
  queue "evals/_template" "clients/_template/evals"
fi

# --- per client ------------------------------------------------------------
shopt -s nullglob
for dir in workflows/*/; do
  dir="${dir%/}"
  [ -d "$dir" ] || continue
  client="$(basename "$dir")"
  case "$client" in
    _templates) continue ;;
  esac

  moved_any=0

  for f in "$dir"/*.json; do
    [ -e "$f" ] || continue
    queue "$f" "clients/$client/automation/dagnats/$(basename "$f")"
    moved_any=1
  done

  for name in README.md client.yaml; do
    [ -e "$dir/$name" ] || continue
    queue "$dir/$name" "clients/$client/$name"
    moved_any=1
  done

  if [ -d "evals/$client" ]; then
    queue "evals/$client" "clients/$client/evals"
    moved_any=1
  fi

  # Unmapped leftovers: visible, never silent.
  if [ "$moved_any" -eq 1 ]; then
    for entry in "$dir"/*; do
      base="$(basename "$entry")"
      case "$base" in
        *.json|README.md|client.yaml) continue ;;
      esac
      note "$entry not mapped (move by hand; set engine: in client.yaml)"
    done
  fi
done

if [ "${#SRCS[@]}" -eq 0 ]; then
  echo "layout: already on clients/ (nothing to migrate)"
  print_notes
  exit 0
fi

# --- plan ------------------------------------------------------------------
conflicts=0
echo "layout migration plan ($MODE):"
i=0
while [ "$i" -lt "${#SRCS[@]}" ]; do
  src="${SRCS[$i]}"
  dst="${DSTS[$i]}"
  i=$((i + 1))
  if [ -e "$dst" ]; then
    echo "  CONFLICT $src -> $dst (destination exists)"
    conflicts=1
  else
    echo "  $src -> $dst"
  fi
done
print_notes

if [ "$MODE" = "check" ]; then
  echo "check only: ${#SRCS[@]} path(s) would move. Re-run with --apply."
  [ "$conflicts" -eq 0 ] || exit 1
  exit 0
fi

# --- apply -----------------------------------------------------------------
[ "$conflicts" -eq 0 ] || { echo "aborting: resolve the conflicts above first" >&2; exit 1; }

move_one() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "$dst")"
  if git rev-parse --git-dir >/dev/null 2>&1 \
     && [ -n "$(git ls-files -- "$src" | head -n 1)" ]; then
    git mv "$src" "$dst"
  else
    mv "$src" "$dst"
  fi
}

i=0
while [ "$i" -lt "${#SRCS[@]}" ]; do
  src="${SRCS[$i]}"
  dst="${DSTS[$i]}"
  i=$((i + 1))
  echo "moving $src -> $dst"
  move_one "$src" "$dst"
done

# Drop legacy client folders that are now empty (git does not track dirs).
for dir in workflows/*/; do
  dir="${dir%/}"
  [ -d "$dir" ] || continue
  case "$(basename "$dir")" in _templates) continue ;; esac
  rmdir "$dir" 2>/dev/null || true
done
rmdir workflows 2>/dev/null || true
rmdir evals 2>/dev/null || true

echo "moved ${#SRCS[@]} path(s). Next: git status, then WORKFLOWS_DIR=clients go test ./clients/"
