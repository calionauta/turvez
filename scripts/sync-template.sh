#!/usr/bin/env bash
# Check-only drift report: ops repo vs pinned template.
#
# Usage (from the template checkout):
#   TURVEZ_CHECKOUT=$PWD TURVEZ_OPS_DIR=../turvez-ops ./scripts/sync-template.sh
#   cd $TURVEZ_CHECKOUT && TURVEZ_OPS_DIR=$PWD ./scripts/sync-template.sh
#
# Never auto-merges. Prints: pin vs HEAD, new _templates files, new handler
# task names the ops recipes don't know yet. Human decides what to adopt.
set -euo pipefail

TEMPLATE_DIR="${TURVEZ_CHECKOUT:?set TURVEZ_CHECKOUT to the turvez template clone}"
OPS_DIR="${TURVEZ_OPS_DIR:-$PWD}"
PIN_FILE="$OPS_DIR/TURVEZ_PIN"

PINNED="$(awk -F= '/^TURVEZ_REF=/{print $2}' "$PIN_FILE" 2>/dev/null || echo "unknown")"
HEAD="$(git -C "$TEMPLATE_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
echo "== pin =="
echo "  TURVEZ_PIN: $PINNED"
echo "  template HEAD: $HEAD"
[ "$PINNED" = "$HEAD" ] || [ "$PINNED" = "$(git -C "$TEMPLATE_DIR" rev-parse HEAD 2>/dev/null)" ] \
  && echo "  ✅ in sync" || echo "  ⚠️  drift — review changelog, test, then bump TURVEZ_PIN manually"

echo
echo "== templates (new skeletons you may want) =="
ls "$TEMPLATE_DIR/workflows/_templates/"

echo
echo "== handlers (registry in template) =="
grep -o '"http.request"\|"transform.json"\|"condition"\|"map"\|"delay"\|"notify[^"]*"\|"request.approval"\|"llm.complete"\|"dlq.check"' \
  "$TEMPLATE_DIR/internal/handlers/handlers.go" 2>/dev/null | sort -u || echo "  (registry unreadable — check internal/handlers/handlers.go)"

echo
echo "no action taken. To adopt: cd $TEMPLATE_DIR && git pull, re-run go test with WORKFLOWS_DIR=$OPS_DIR/workflows, then update TURVEZ_PIN."
