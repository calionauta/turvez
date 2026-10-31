#!/usr/bin/env bash
# rename-workflows.sh — migrate workflow names to the current naming.json.
#
# Use after changing `client_workflow_separator` (and the regex) in
# naming.json: this rewrites each recipe's "name" to <client><sep><basename>
# and every literal reference to the old name inside that file (trigger
# "workflow" fields, payload strings).
#
# Dry-run by default. Re-register afterwards: the KV key changes, so old
# definitions stay behind until `dagnats workflow delete <old>`.
#
# Usage:
#   ./scripts/rename-workflows.sh                     # preview, ./clients
#   ./scripts/rename-workflows.sh --apply
#   WORKFLOWS_DIR=../turvez-ops/clients ./scripts/rename-workflows.sh --apply
#
# Both layouts are supported: <client>/automation/dagnats/*.json (phase 3) and
# the legacy <client>/*.json (phase 1).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SEP="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["client_workflow_separator"])' "$ROOT/naming.json")"
if [ -z "${WORKFLOWS_DIR:-}" ]; then
  WORKFLOWS_DIR="${TURVEZ_OPS_DIR:+$(cd "${TURVEZ_OPS_DIR}" && "$ROOT/scripts/clients-root.sh")}"
  WORKFLOWS_DIR="${WORKFLOWS_DIR:-$("$ROOT/scripts/clients-root.sh")}"
fi

APPLY=0
[ "${1:-}" = "--apply" ] && APPLY=1

python3 - "$WORKFLOWS_DIR" "$SEP" "$APPLY" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
sep = sys.argv[2]
apply = sys.argv[3] == "1"

changed = 0
for client_dir in sorted(p for p in root.iterdir() if p.is_dir()):
    if client_dir.name in ("_templates", "_template"):
        continue
    # phase 3 keeps recipes one level down; the legacy layout has them flat.
    recipe_dirs = [d for d in (client_dir / "automation" / "dagnats", client_dir) if d.is_dir()]
    for f in sorted(p for d in recipe_dirs for p in d.glob("*.json")):
        try:
            wf = json.loads(f.read_text())
        except json.JSONDecodeError:
            print(f"skip {f}: invalid JSON")
            continue
        old = wf.get("name", "")
        if not old:
            continue
        desired = f"{client_dir.name}{sep}{f.stem}"
        if old == desired:
            continue
        changed += 1
        if apply:
            text = f.read_text()
            text = text.replace(json.dumps(old), json.dumps(desired))
            f.write_text(text)
            print(f"renamed {f}: {old} -> {desired}")
        else:
            print(f"would rename {f}: {old} -> {desired}")

print(f"{changed} file(s) {'renamed' if apply else 'to rename'}")
if changed and not apply:
    print("re-run with --apply, then re-register and delete the old names")
PY
