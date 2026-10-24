#!/usr/bin/env bash
# Setup check (+ optional init) for a fresh clone.
#
# Usage:
#   ./scripts/setup.sh                # check only — no side effects
#   ./scripts/setup.sh --init [client]# also copies *.env.example → *.env when missing
#                                     # (gitignored, never overwrites existing)
#
# What a new user needs, in order:
#   1. Go toolchain (worker builds + go test)
#   2. dagnats CLI pinned at v0.0.30 (validate/register/run)
#   3. Docker daemon (compose stack: engine + worker)
#   4. env files (names only in git; values are yours)
#   5. (optional) degustia checkout → scripts/tool-pick.sh ranking
set -euo pipefail

INIT=0
CLIENT="${2:-personal}"
if [ "${1:-}" = "--init" ]; then INIT=1; fi
[ "${1:-}" = "--init" ] && [ -n "${2:-}" ] && CLIENT="$2"

pass=0; fail=0
ok()   { echo "  ✅ $1"; pass=$((pass+1)); }
warn() { echo "  ⚠️  $1"; fail=$((fail+1)); }

echo "== 1. Go toolchain =="
if command -v go >/dev/null; then ok "$(go version)"; else warn "go not found — https://go.dev/dl/"; fi

echo "== 2. dagnats CLI (want v0.0.30) =="
if command -v dagnats >/dev/null; then
  V="$(go version -m "$(command -v dagnats)" 2>/dev/null | awk '$1=="mod" && $2=="github.com/danmestas/dagnats" {print $3}')"
  if [ "$V" = "v0.0.30" ]; then ok "dagnats $V at $(command -v dagnats)";
  else warn "dagnats version ${V:-unknown}, want v0.0.30"; fi
  echo "     install/refresh: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30"
else
  warn "dagnats CLI not found"
  echo "     install: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30"
fi

echo "== 3. Docker daemon =="
if docker info >/dev/null 2>&1; then ok "$(docker --version)"; else warn "docker daemon not reachable (colima start?)"; fi

echo "== 4. env files (values, gitignored) =="
check_env() {
  local example="$1" target="$2"
  if [ -f "$target" ]; then ok "$target exists";
  elif [ "$INIT" = "1" ] && [ -f "$example" ]; then cp "$example" "$target"; ok "$target created from $example (fill values)";
  elif [ -f "$example" ]; then warn "$target missing — cp $example $target";
  else warn "no $example template"; fi
}
check_env ".env.example" ".env"
check_env "env/$CLIENT.env.example" "env/$CLIENT.env"

echo "== 5. degustia ranker (optional, for scripts/tool-pick.sh) =="
RANKER="${DEGUSTIA_SKILL_DIR:-}/scripts/rank_candidates.py"
if [ -n "${DEGUSTIA_SKILL_DIR:-}" ] && [ -f "$RANKER" ]; then ok "ranker at $RANKER";
else warn "DEGUSTIA_SKILL_DIR unset or missing ranker — tool-pick.sh degrades to manual hierarchy"; fi

echo
echo "== next =="
echo "  make up                      # engine :8080/:4222 + worker (docker)"
echo "  # …or locally: dagnats serve &  make worker"
echo "  ./scripts/setup.sh --init $CLIENT   # scaffold env files"
echo "  ./scripts/install-hooks.sh       # pre-commit secret scan (opt-in, local)"
echo "  make register CLIENT=$CLIENT && make smoke"
echo "  dagnats run start \"$CLIENT/ping\" '{\"hello\":\"world\"}' --watch"
echo
echo "== optional companions (informed, never installed automatically) =="
echo "  FDEOps methodology (discover/scope/runbook): npx skills add suboss87/fdeops --skill fde"
echo "  degustia MCP, free (recommend/render/validate): https://degustia-mcp.calionauta.com/mcp"
echo "  turvez skill for your harness: npx skills add calionauta/turvez --skill turvez -g"
[ "$fail" -eq 0 ] || { echo; echo "$fail check(s) need attention (see ⚠️  above)"; exit 1; }
echo; echo "setup OK ($pass checks passed)"
