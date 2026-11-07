#!/usr/bin/env bash
# setup-nango.sh — health check and bootstrap for the Nango auth layer.
#
# Nango is the credential broker in front of SaaS APIs: Windmill (or dagnats)
# calls its proxy and Nango refreshes the OAuth token (docs/nango.md). The
# runners only ever hold NANGO_HOST and NANGO_SECRET_KEY.
#
# One broker per client is the default (docs/access-plane.md §3): NANGO_CLIENT_ID
# scopes the Compose project, the containers and the Postgres volume
# (nango-<client>). The shared broker is explicit and pilots-only.
#
# Usage:
#   NANGO_CLIENT_ID=acme ./scripts/setup-nango.sh --up  # this client's broker
#   ./scripts/setup-nango.sh --up --shared              # shared (data_class: normal)
#   ./scripts/setup-nango.sh                            # health check
#   ./scripts/setup-nango.sh --down                     # stop this client's stack
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

# One broker per client is the default (docs/access-plane.md §3): the project
# name, containers and the Postgres volume all carry the client suffix, so two
# stacks on one host cannot share a database or an encryption key by accident.
action="--check"
shared=0
for arg in "$@"; do
  case "$arg" in
    --up|up|--down|down|--check|check) action="$arg" ;;
    --shared) shared=1 ;;
    -h|--help)
      sed -n '3,24p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "setup-nango: unknown argument '$arg' (want --up | --down | --check | --shared)" >&2
      exit 2
      ;;
  esac
done

CLIENT="${NANGO_CLIENT_ID:-${CLIENT_ID:-}}"

if [ -n "$CLIENT" ] && [ "$shared" -eq 1 ]; then
  echo "setup-nango: --shared contradicts NANGO_CLIENT_ID=$CLIENT" >&2
  echo "  a shared broker belongs to no single client; drop one (docs/access-plane.md §3)" >&2
  exit 2
fi

case "$action" in
  --up|up)
    if [ -z "$CLIENT" ] && [ "$shared" -eq 0 ]; then
      echo "setup-nango: NANGO_CLIENT_ID is required" >&2
      echo "" >&2
      echo "  one broker per client is the default (docs/access-plane.md §3):" >&2
      echo "    NANGO_CLIENT_ID=acme NANGO_PORT=8188 NANGO_CONNECT_PORT=8189 $0 --up" >&2
      echo "  a shared broker is only legal for a data_class: normal pilot:" >&2
      echo "    $0 --up --shared" >&2
      exit 1
    fi
    if [ "$shared" -eq 1 ]; then
      echo "setup-nango: starting the SHARED broker (pilots, data_class: normal only)" >&2
      echo "  a sensitive client on this stack fails 'go test ./clients/' (invariant 7)" >&2
    elif [ -z "${NANGO_PORT:-}" ]; then
      echo "setup-nango: NANGO_PORT unset, using 8088 — that collides with any other" >&2
      echo "  stack on this host; give each client its own NANGO_PORT/NANGO_CONNECT_PORT" >&2
    fi
    ;;
esac

PROJECT="${NANGO_PROJECT:-nango-shared}"
if [ -z "${NANGO_PROJECT:-}" ] && [ -n "$CLIENT" ]; then
  PROJECT="nango-$CLIENT"
fi
export NANGO_CLIENT_ID="${CLIENT:-shared}"
COMPOSE=(docker compose -f "$COMPOSE_FILE" -p "$PROJECT")

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
    echo "== starting Nango auth layer for '$NANGO_CLIENT_ID' (project $PROJECT)"
    "${COMPOSE[@]}" up -d
    echo "waiting for Nango at $NANGO_HOST/health ..."
    for _ in $(seq 1 30); do
      if curl -s -f -m 2 "$NANGO_HOST/health" >/dev/null 2>&1; then
        echo "nango: healthy at $NANGO_HOST (project $PROJECT, client $NANGO_CLIENT_ID)"
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
    echo "== stopping Nango stack '$PROJECT' (volume ${PROJECT}_nango-db-data is kept)"
    "${COMPOSE[@]}" down
    exit 0
    ;;

  --check|check|*)
    echo "== checking Nango auth layer ($NANGO_HOST, project $PROJECT)"
    if curl -s -f -m 3 "$NANGO_HOST/health" >/dev/null 2>&1; then
      echo "nango: running at $NANGO_HOST"
      exit 0
    fi
    echo "nango: NOT running at $NANGO_HOST"
    echo "start it with: NANGO_CLIENT_ID=<client> ./scripts/setup-nango.sh --up"
    echo "or shared:     ./scripts/setup-nango.sh --up --shared   (data_class: normal only)"
    exit 1
    ;;
esac
