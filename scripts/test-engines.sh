#!/usr/bin/env bash
# test-engines.sh — every engine in engines.json must scaffold AND validate.
#
# engines.json declares three engines. Before this test, patch: dagu was declared
# everywhere (engines.json, new-client.sh, the contract test) while the gate that
# decides whether a client is valid did not know it — so a dagu client fell
# through to "no automation source" and failed, and the scaffold produced an
# empty automation/dagu/ that could never pass.
#
# A declared engine that cannot be used is worse than an absent one: it looks
# supported. This test makes that state impossible to ship.
#
# Usage: ./scripts/test-engines.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
ENGINES_JSON="$ROOT/engines.json"

[ -f "$ENGINES_JSON" ] || { echo "FAIL: engines.json missing" >&2; exit 1; }

ENGINES="$(python3 -c 'import json,sys;print(" ".join(sorted(json.load(open(sys.argv[1]))["engines"])))' "$ENGINES_JSON" 2>/dev/null)"
[ -n "$ENGINES" ] || { echo "FAIL: could not read the engine list from engines.json" >&2; exit 1; }

failures=0
for engine in $ENGINES; do
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  if ! TURVEZ_OPS_DIR="$tmp" "$HERE/new-client.sh" probe --engine="$engine" >/dev/null 2>&1; then
    echo "FAIL: new-client.sh could not scaffold --engine=$engine" >&2
    failures=$((failures + 1))
    rm -rf "$tmp"
    continue
  fi

  cdir="$tmp/clients/probe/automation/$engine"
  if [ ! -d "$cdir" ]; then
    echo "FAIL: --engine=$engine created no automation/$engine/ directory" >&2
    failures=$((failures + 1))
    rm -rf "$tmp"
    continue
  fi

  # An empty engine dir means the scaffold produced a client that can never pass
  # validation; that is the exact dagu bug this test exists for.
  if [ -z "$(ls -A "$cdir" 2>/dev/null)" ]; then
    echo "FAIL: --engine=$engine scaffolded an EMPTY automation/$engine/ (no example to start from)" >&2
    failures=$((failures + 1))
    rm -rf "$tmp"
    continue
  fi

  if ! out="$(TURVEZ_OPS_DIR="$tmp" "$HERE/validate.sh" 2>&1)"; then
    echo "FAIL: a freshly scaffolded --engine=$engine client does not validate:" >&2
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    failures=$((failures + 1))
  else
    echo "ok: --engine=$engine scaffolds and validates"
  fi
  rm -rf "$tmp"
done

if [ "$failures" -ne 0 ]; then
  echo "test-engines: $failures failure(s)" >&2
  exit 1
fi
echo "test-engines: every declared engine works"
