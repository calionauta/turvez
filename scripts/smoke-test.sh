#!/usr/bin/env bash
# End-to-end smoke path: register → start run → watch → inspect.
#
# Side-effect steps (http.request, notify.slack) need real ACME_* secrets.
# Without them the run still proves the plumbing: the engine dispatches,
# the worker executes build-fetch (transform.json), and the failure is
# visible via run status/events/DLQ — which is exactly what this script
# prints. With real secrets, the same run goes fully green.
#
# Usage: ./scripts/smoke-test.sh
set -euo pipefail

WF="example-client/hubspot-lead-to-slack"

./scripts/register-workflows.sh workflows/example-client

echo "== starting run: $WF"
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
echo
echo "Done. Green path needs real ACME_* secrets in .env (see .env.example)."
echo "Failure inspection: dagnats run inspect $RUN_ID ; dagnats dlq list --run=$RUN_ID"
