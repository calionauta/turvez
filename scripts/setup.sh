#!/usr/bin/env bash
# Setup check (+ optional init) for a fresh clone.
#
# Usage:
#   ./scripts/setup.sh                # check only — no side effects
#   ./scripts/setup.sh --init [client]# also copies *.env.example → *.env when missing
#                                     # (gitignored, never overwrites existing)
#
# What a new user needs, in order:
#   0. Repo type (template public vs ops private)
#   1. Go toolchain (worker builds + go test)
#   2. dagnats CLI pinned at v0.0.30 (validate/register/run)
#   3. Docker daemon (compose stack: engine + worker)
#   4. env files, required (names only in git; values are yours)
#   5. degustia ranker, OPTIONAL (tool-pick.sh degrades without it)
#   6. Ops tooling: age (required, secrets flow), python3 (required, register
#      namespace check), gh (optional, only for new-ops-repo.sh --github-private)
set -euo pipefail

INIT=0
CLIENT="${2:-personal}"
if [ "${1:-}" = "--init" ]; then INIT=1; fi
[ "${1:-}" = "--init" ] && [ -n "${2:-}" ] && CLIENT="$2"

pass=0; fail=0; notes=0
ok()   { echo "  ✅ $1"; pass=$((pass+1)); }
warn() { echo "  ⚠️  $1"; fail=$((fail+1)); }
note() { echo "  ℹ️  $1 (optional — setup still passes)"; notes=$((notes+1)); }

echo "== 0. repo type (template vs ops) =="
if [ -f "TURVEZ_PIN" ] && [ ! -f "go.mod" ]; then ok "ops repo (data-only; engine pinned via TURVEZ_PIN)";
elif [ -f "go.mod" ] && [ ! -f "TURVEZ_PIN" ]; then ok "template repo (public; real clients live in turvez-ops — docs/ops-repo.md)";
elif [ -f "TURVEZ_PIN" ] && [ -f "go.mod" ]; then warn "both TURVEZ_PIN and go.mod present — ambiguous checkout";
else warn "unknown checkout (neither go.mod nor TURVEZ_PIN)"; fi
if [ -n "${WORKFLOWS_DIR:-${TURVEZ_OPS_DIR:+$(cd "$TURVEZ_OPS_DIR" 2>/dev/null && "$(dirname "$0")/clients-root.sh")}}" ]; then ok "WORKFLOWS_DIR override: ${WORKFLOWS_DIR:-$("$(dirname "$0")/clients-root.sh")}"; fi

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
if [ -f "TURVEZ_PIN" ] && [ ! -f "go.mod" ]; then
  : # ops repo: no root .env.example by design (per-client env/<client>.env only)
else
  check_env ".env.example" ".env"
fi
check_env "env/$CLIENT.env.example" "env/$CLIENT.env"

echo "== 5. degustia ranker (optional, for scripts/tool-pick.sh) =="
RANKER="${DEGUSTIA_SKILL_DIR:-}/scripts/rank_candidates.py"
if [ -n "${DEGUSTIA_SKILL_DIR:-}" ] && [ -f "$RANKER" ]; then ok "ranker at $RANKER";
else note "DEGUSTIA_SKILL_DIR unset or missing ranker — tool-pick.sh degrades to manual hierarchy"; fi

echo "== 6. eval tooling (optional) =="
if command -v npx >/dev/null; then ok "npx $(npx --version 2>/dev/null) — promptfoo via npx (make eval-loop / eval-client-view)"; else note "npx not found — eval loop (make eval-loop) will not run"; fi

echo "== 7. ops tooling =="
if command -v age >/dev/null && command -v age-keygen >/dev/null; then ok "age + age-keygen ($(command -v age))";
else warn "age not found — edit-secrets.sh needs it (brew install age)"; fi
if command -v python3 >/dev/null; then ok "python3 ($(command -v python3))";
else warn "python3 not found — register-workflows.sh needs it for the namespace check"; fi
if command -v gh >/dev/null; then ok "gh ($(command -v gh)) — for new-ops-repo.sh --github-private";
else note "gh not found — only needed to push a new ops repo (https://cli.github.com/)"; fi

echo
echo "== next =="
if [ -f "TURVEZ_PIN" ] && [ ! -f "go.mod" ]; then
  echo "  # ops repo: validate/register from the pinned template checkout"
  echo "  OPS=\$PWD; cd \$TURVEZ_CHECKOUT && WORKFLOWS_DIR=\$OPS/clients go test ./clients/"
  echo "  # or: make validate-ops TURVEZ_OPS_DIR=\$PWD   (from \$TURVEZ_CHECKOUT)"
  echo "  OPS=\$PWD; cd \$TURVEZ_CHECKOUT && WORKFLOWS_DIR=\$OPS/clients ./scripts/register-workflows.sh $CLIENT"
else
  echo "  make up                      # engine :8080/:4222 + worker (docker)"
  echo "  # …or locally: dagnats serve &  make worker"
  echo "  ./scripts/setup.sh --init $CLIENT   # scaffold env files"
  echo "  ./scripts/install-hooks.sh       # pre-commit secret scan (opt-in, local)"
  echo "  ./scripts/new-ops-repo.sh ../turvez-ops  # private data repo (real clients live there)"
  echo "  make register CLIENT=$CLIENT && make smoke"
  echo "  dagnats run start \"$CLIENT/ping\" '{\"hello\":\"world\"}' --watch"
fi
echo
echo "== UI =="
echo "  Console:   http://127.0.0.1:\${DAGNATS_HTTP_PORT:-8080}/console/  (needs DAGNATS_CONSOLE_PASSWORD in a container)"
echo "  Eval UI:   make eval-view  ->  http://localhost:15500"
echo "  Trap:      if 8080/4222 are already taken, the CLI reads another server and runs stay queued."
echo "             Export DAGNATS_HTTP_PORT / DAGNATS_NATS_PORT / DAGNATS_NATS_URL before running."
echo
echo "== optional companions (informed, never installed automatically) =="
echo "  FDEOps methodology (discover/scope/runbook): npx skills add suboss87/fdeops --skill fde"
echo "  degustia MCP, free (recommend/render/validate): https://degustia-mcp.calionauta.com/mcp"
echo "  turvez skill for your harness: npx skills add calionauta/turvez --skill turvez -g"
[ "$fail" -eq 0 ] || { echo; echo "$fail required check(s) need attention (see ⚠️  above)"; exit 1; }
echo; echo "setup OK ($pass checks passed${notes:+, $notes optional note(s)})"
