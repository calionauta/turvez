#!/usr/bin/env bash
# Ranked tool suggestions via degustia's deterministic ranker.
# No network, no auth, no report — stdout is the ranked JSON list.
#
# Usage:
#   ./scripts/tool-pick.sh --dor "avisar o time no slack quando um deal fechar" \
#     [--tag Automação] [--perfil Intermediario] [--k 4]
#
# Requires a degustia checkout:
#   export DEGUSTIA_SKILL_DIR=~/Development/degustia/.agents/skills/cali-degustia-diagnostico
# Without it, exits 2 with the manual fallback (hierarchy + websearch) instead
# of failing silently — a fresh clone without degustia still gets direction.
set -euo pipefail

DOR=""
TAG=""
PERFIL="Intermediario"
K="4"

while [ $# -gt 0 ]; do
  case "$1" in
    --dor=*) DOR="${1#--dor=}"; shift ;;
    --dor) DOR="${2:-}"; shift 2 ;;
    --tag=*) TAG="${1#--tag=}"; shift ;;
    --tag) TAG="${2:-}"; shift 2 ;;
    --perfil=*) PERFIL="${1#--perfil=}"; shift ;;
    --perfil) PERFIL="${2:-}"; shift 2 ;;
    --k=*) K="${1#--k=}"; shift ;;
    --k) K="${2:-}"; shift 2 ;;
    --*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) echo "unknown arg: $1 (use --dor \"...\")" >&2; exit 2 ;;
  esac
done

[ -n "$DOR" ] || { echo "usage: $0 --dor=\"...\" [--tag=X] [--perfil=Y] [--k=N]" >&2; exit 2; }

RANKER="${DEGUSTIA_SKILL_DIR:-}/scripts/rank_candidates.py"
if [ -z "${DEGUSTIA_SKILL_DIR:-}" ] || [ ! -f "$RANKER" ]; then
  cat >&2 <<'EOF'
tool-pick: degustia checkout not found (DEGUSTIA_SKILL_DIR unset or missing
scripts/rank_candidates.py). Manual fallback, same hierarchy:
  1. what the client already has that covers the pain (configure/integrate);
  2. a free option covering requirements + volume;
  3. paid only with a documented gap; verify capabilities on the web, never guess.
Clone degustia e exporte DEGUSTIA_SKILL_DIR para o ranking automático.
EOF
  exit 2
fi

ARGS=(--dor "$DOR" --perfil "$PERFIL" --k "$K")
[ -n "$TAG" ] && ARGS+=(--tag "$TAG")
python3 "$RANKER" "${ARGS[@]}"
