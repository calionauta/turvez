#!/usr/bin/env bash
# doctor.sh — one-shot health check for a running stack.
#
# Catches the failures that look like "the worker is broken" but are really
# wiring: wrong NATS target, no worker, console disabled, stuck runs.
#
# Read-only. Exits 1 on a real failure, 0 with warnings otherwise.
#
#   DAGNATS_NATS_URL=nats://127.0.0.1:4222 ./scripts/doctor.sh
#   DAGNATS_HTTP_PORT=31080 DAGNATS_CONSOLE_PASSWORD=... ./scripts/doctor.sh

set -uo pipefail

HTTP_PORT="${DAGNATS_HTTP_PORT:-8080}"
NATS_PORT="${DAGNATS_NATS_PORT:-4222}"
NATS_URL="${DAGNATS_NATS_URL:-nats://127.0.0.1:$NATS_PORT}"
CONSOLE_URL="http://127.0.0.1:$HTTP_PORT/console/"

pass=0; warn=0; fail=0
ok()   { echo "  ok    $1"; pass=$((pass+1)); }
note() { echo "  warn  $1"; warn=$((warn+1)); }
bad()  { echo "  FAIL  $1"; fail=$((fail+1)); }

echo "== ports =="
for p in "$HTTP_PORT" "$NATS_PORT"; do
  if command -v lsof >/dev/null 2>&1; then
    holder="$(lsof -nP -iTCP:"$p" -sTCP:LISTEN 2>/dev/null | awk 'NR==2{print $1}')"
  elif command -v ss >/dev/null 2>&1; then
    holder="$(ss -ltnp 2>/dev/null | awk -v p=":$p" '$4 ~ p {print $6; exit}')"
  else
    holder=""
  fi
  if [ -n "$holder" ]; then ok ":$p listener ($holder)"; else note ":$p has no listener"; fi
done

echo "== CLI target (NATS) =="
if status_json="$(dagnats status --json 2>/dev/null)"; then
  runs="$(echo "$status_json" | jq -c '.runs // "?"' 2>/dev/null)"
  ok "NATS reachable at $NATS_URL (runs=$runs)"
else
  bad "dagnats status failed against $NATS_URL — is DAGNATS_NATS_URL right?"
fi

echo "== workers =="
workers="$(dagnats workers list --json 2>/dev/null | jq 'length' 2>/dev/null || echo "?")"
if [ "$workers" = "0" ]; then
  bad "no workers registered — tasks will stay queued"
elif [ "$workers" = "?" ]; then
  note "could not read worker list"
else
  ok "$workers worker(s) registered"
fi

echo "== runs =="
stuck="$(dagnats run list --json 2>/dev/null | jq '[.runs[]? | select(.status=="running" or .status=="pending")] | length' 2>/dev/null || echo "?")"
if [ "$stuck" = "?" ]; then
  note "could not read run list"
elif [ "$stuck" -gt 0 ]; then
  note "$stuck run(s) not terminal — check 'dagnats run inspect <id>'"
else
  ok "no stuck runs"
fi

echo "== console =="
if [ -n "${DAGNATS_CONSOLE_PASSWORD:-}" ]; then
  code="$(curl -s -o /dev/null -m 3 -w '%{http_code}' -u "admin:$DAGNATS_CONSOLE_PASSWORD" "$CONSOLE_URL" 2>/dev/null)"
else
  code="$(curl -s -o /dev/null -m 3 -w '%{http_code}' "$CONSOLE_URL" 2>/dev/null)"
fi
case "$code" in
  200) ok "console at $CONSOLE_URL" ;;
  401) note "console requires auth (set DAGNATS_CONSOLE_PASSWORD and retry)" ;;
  503) bad "console disabled: set DAGNATS_CONSOLE_PASSWORD or DAGNATS_CONSOLE_TRUST_FORWARDED_AUTH=true" ;;
  *)   note "console returned HTTP $code at $CONSOLE_URL" ;;
esac

echo "== auth layer (optional) =="
if [ -n "${NANGO_URL:-}" ]; then
  code="$(curl -s -o /dev/null -m 3 -w '%{http_code}' "${NANGO_URL%/}/health" 2>/dev/null)"
  if [ "$code" = "200" ]; then ok "Nango reachable at $NANGO_URL"; else note "Nango configured but /health returned ${code:-no response}"; fi
  if [ -n "${NANGO_SECRET_KEY:-}" ]; then ok "NANGO_SECRET_KEY present"; else note "NANGO_SECRET_KEY not set"; fi
else
  note "Nango not configured (optional auth layer; see docs/script-execution.md)"
fi

echo "== eval tooling =="
if command -v npx >/dev/null 2>&1; then ok "npx $(npx --version 2>/dev/null)"; else note "npx missing — eval loop unavailable"; fi

echo
if [ "$fail" -gt 0 ]; then
  echo "doctor: $fail failure(s), $warn warning(s)"; exit 1
fi
echo "doctor: ok ($pass checks, $warn warning(s))"
