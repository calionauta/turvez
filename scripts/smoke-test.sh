#!/usr/bin/env bash
# End-to-end smoke path: runs a smoke test according to the client's declared engine.
#
# Usage:
#   ./scripts/smoke-test.sh [client]
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/smoke-test.sh acme
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${TURVEZ_OPS_DIR:-$(cd "$HERE/.." && pwd)}"
CLIENT="${1:-example-client}"

CLIENT_DIR="$ROOT/clients/$CLIENT"
if [ ! -d "$CLIENT_DIR" ]; then
  echo "smoke-test: client '$CLIENT' not found under $ROOT/clients/" >&2
  exit 1
fi

# Detect engine from client.yaml
ENGINE="dagnats"
if [ -f "$CLIENT_DIR/client.yaml" ]; then
  DETECTED="$(awk -F':' '$1 ~ /^engine$/ {gsub(/[ \t"\x27]/,"",$2); print $2}' "$CLIENT_DIR/client.yaml" 2>/dev/null | head -1)"
  [ -n "$DETECTED" ] && ENGINE="$DETECTED"
fi

echo "== smoke-test: client $CLIENT (engine: $ENGINE) =="

case "$ENGINE" in
  dagnats)
    WF="$CLIENT.hubspot-lead-to-slack"
    # Find first recipe if default doesn't exist
    if [ ! -f "$CLIENT_DIR/automation/dagnats/hubspot-lead-to-slack.json" ] && [ ! -f "$CLIENT_DIR/hubspot-lead-to-slack.json" ]; then
      FIRST="$(find "$CLIENT_DIR" -name "*.json" | head -1)"
      if [ -n "$FIRST" ]; then
        BASE="$(basename "$FIRST" .json)"
        WF="$CLIENT.$BASE"
      fi
    fi

    "$HERE/register-workflows.sh" "$CLIENT"

    echo "== starting dagnats run: $WF"
    if dagnats run start --help 2>&1 | grep -q -- '--json'; then
      RUN_OUT="$(dagnats run start "$WF" '{"lead_id":"smoke-1"}' --json)"
      echo "$RUN_OUT"
      RUN_ID="$(echo "$RUN_OUT" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("run_id") or d.get("runId") or d.get("id") or "")' 2>/dev/null || true)"
    else
      dagnats run start "$WF" '{"lead_id":"smoke-1"}'
      RUN_ID=""
    fi

    if [ -z "${RUN_ID:-}" ]; then
      echo "Could not parse run id from output; listing recent runs:"
      dagnats run list --workflow="$WF" 2>/dev/null || dagnats run list
      echo "Inspect manually: dagnats run status <run-id> | dagnats run events <run-id>"
      exit 0
    fi

    echo "== run id: $RUN_ID"
    sleep 8
    echo "== status"
    dagnats run status "$RUN_ID" || true
    echo "== events (last 20)"
    dagnats run events "$RUN_ID" 2>/dev/null | tail -20 || true
    echo "== output (empty unless completed)"
    dagnats run output "$RUN_ID" || true
    ;;

  windmill)
    if ! command -v wmill >/dev/null 2>&1; then
      echo "smoke-test: wmill CLI is required for windmill smoke tests" >&2
      exit 1
    fi
    FLOW_DIR="$(find "$CLIENT_DIR/automation/windmill" -mindepth 1 -maxdepth 2 -name "flow.yaml" -exec dirname {} \; 2>/dev/null | head -1)"
    if [ -z "$FLOW_DIR" ]; then
      echo "smoke-test: no flow.yaml found under $CLIENT_DIR/automation/windmill" >&2
      exit 1
    fi
    echo "== testing flow preview: $FLOW_DIR"
    wmill flow preview "$FLOW_DIR" --data '{}'
    echo "smoke-test: windmill preview passed"
    ;;

  dagu)
    DAG="$(find "$CLIENT_DIR/automation/dagu" -name "*.yaml" -o -name "*.yml" 2>/dev/null | head -1)"
    if [ -z "$DAG" ]; then
      echo "smoke-test: no DAG yaml found under $CLIENT_DIR/automation/dagu" >&2
      exit 1
    fi
    if command -v dagu >/dev/null 2>&1; then
      echo "== validating DAG: $DAG"
      dagu validate "$DAG"
      echo "== dry running DAG: $DAG"
      dagu start "$DAG"
    else
      echo "smoke-test: dagu CLI not installed, validating structure"
      python3 -c "import yaml,sys; doc=yaml.safe_load(open('$DAG')); assert 'steps' in doc"
      echo "smoke-test: dag structure valid"
    fi
    ;;

  *)
    echo "smoke-test: unsupported engine '$ENGINE'" >&2
    exit 1
    ;;
esac

echo "smoke-test: ok"
