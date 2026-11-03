#!/usr/bin/env bash
# setup-nango.sh — health check and bootstrap for the Nango auth layer.
#
# Nango is the credential broker in front of SaaS APIs: Windmill (or dagnats)
# calls its proxy and Nango refreshes the OAuth token (docs/nango.md). The
# runners only ever hold NANGO_HOST and NANGO_SECRET_KEY.
#
# Usage:
#   ./scripts/setup-nango.sh          # check if running; report status
#   ./scripts/setup-nango.sh --up     # validate env, start the stack, wait
#   ./scripts/setup-nango.sh --down   # stop the stack (data volume is kept)
#
# `--up` refuses to start without NANGO_ENCRYPTION_KEY and NANGO_DB_PASSWORD:
# a default encryption key would be shared by every deployment, which is a
# backdoor rather than a convenience.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

PORT="${NANGO_PORT:-8088}"
NANGO_HOST="${NANGO_HOST:-http://localhost:$PORT}"
COMPOSE_FILE="$ROOT/docker-compose.nango.yml"
COMPOSE=(docker compose -f "$COMPOSE_FILE")

action="${1:---check}"

missing_env() {
  local missing=()
  for v in NANGO_ENCRYPTION_KEY NANGO_DB_PASSWORD; do
    [ -n "${!v:-}" ] || missing+=("$v")
  done
  if [ "${#missing[@]}" -eq 0 ]; then
    return 0
  fi
  echo "setup-nango: missing ${missing[*]}" >&2
  echo "" >&2
  echo "Export them (or put them in $ROOT/.env, which compose reads):" >&2
  echo "  export NANGO_ENCRYPTION_KEY=\$(openssl rand -hex 16)   # 32 hex chars, keep it" >&2
  echo "  export NANGO_DB_PASSWORD=\$(openssl rand -base64 24)" >&2
  echo "" >&2
  echo "Store the encryption key with the deployment: without it the stored" >&2
  echo "OAuth tokens are unrecoverable." >&2
  return 1
}

case "$action" in
  --up|up)
    command -v docker >/dev/null 2>&1 || {
      echo "setup-nango: docker is required to spin up Nango" >&2
      exit 1
    }
    missing_env || exit 1
    echo "== starting Nango auth layer (${COMPOSE[*]} up -d)"
    "${COMPOSE[@]}" up -d
    echo "waiting for Nango at $NANGO_HOST/health ..."
    for _ in $(seq 1 30); do
      if curl -s -f -m 2 "$NANGO_HOST/health" >/dev/null 2>&1; then
        echo "nango: healthy at $NANGO_HOST"
        echo "  dashboard/API: $NANGO_HOST"
        echo "  connect UI  : http://localhost:${NANGO_CONNECT_PORT:-8089}"
        echo "next: create the integration in the dashboard, then run"
        echo "      ./scripts/nango-connect.sh <client> --provider <slug> --link --open"
        echo "      (the operator authorises; we never handle the token)"
        echo "      and set NANGO_HOST / NANGO_SECRET_KEY as Windmill variables"
        exit 0
      fi
      sleep 2
    done
    echo "nango: container started but /health is not answering yet." >&2
    echo "logs: ${COMPOSE[*]} logs nango" >&2
    exit 1
    ;;

  --down|down)
    echo "== stopping Nango stack (volume nango-db-data is kept)"
    "${COMPOSE[@]}" down
    exit 0
    ;;

  --check|check|*)
    echo "== checking Nango auth layer ($NANGO_HOST)"
    if curl -s -f -m 3 "$NANGO_HOST/health" >/dev/null 2>&1; then
      echo "nango: running at $NANGO_HOST"
      exit 0
    fi
    echo "nango: NOT running at $NANGO_HOST"
    echo "start it with: ./scripts/setup-nango.sh --up"
    echo "or:            docker compose -f docker-compose.nango.yml up -d"
    exit 1
    ;;
esac
