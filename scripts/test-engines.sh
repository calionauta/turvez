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
# Retrieval consistency: this test also fails when the main retrieval docs and
# the skill disagree about the rewrite/fan-out default. The current default is:
# 0 rewrites by default, or 1 cached rewrite with a fallback to the original query;
# hybrid candidate cap 25-50; never a multi-rewrite fan-out as the default.
# The check below reads docs/retrieval.md, skills/turvez/references/retrieval.md
# and skills/turvez/references/decision-tree.md — if any of them contradicts the
# default, this script exits 1.
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

  # Every engine must also produce a coherent extraction plan via extract-client.py.
  # Previously dagu fell through to windmill commands (wmill flow preview).
  if ! plan="$(TURVEZ_OPS_DIR="$tmp" "$HERE/extract-client.py" probe 2>&1)"; then
    echo "FAIL: extract-client.py probe failed for --engine=$engine:" >&2
    printf '%s\n' "$plan" | sed 's/^/    /' >&2
    failures=$((failures + 1))
  else
    case "$engine" in
      dagu)
        if echo "$plan" | grep -q "wmill"; then
          echo "FAIL: extract-client.py leaked wmill into dagu plan" >&2
          failures=$((failures + 1))
        fi
        ;;
      windmill)
        if echo "$plan" | grep -q "dagnats serve"; then
          echo "FAIL: extract-client.py leaked dagnats into windmill plan" >&2
          failures=$((failures + 1))
        fi
        ;;
      dagnats)
        if echo "$plan" | grep -q "wmill"; then
          echo "FAIL: extract-client.py leaked wmill into dagnats plan" >&2
          failures=$((failures + 1))
        fi
        ;;
    esac
  fi
  rm -rf "$tmp"
done

# check-dagnats-pin.sh must catch drift, not just exist: an ops repo builds the
# server from a tag, and a tag older than the linked libraries is a run-time
# failure, not a build error.
pin_tmp="$(mktemp -d)"
cp "$HERE/../go.mod" "$HERE/../Dockerfile.dagnats" "$pin_tmp/"
mkdir -p "$pin_tmp/scripts" "$pin_tmp/docs"
cp "$HERE/setup.sh" "$pin_tmp/scripts/"
cp "$HERE/../docs/ops-repo.md" "$pin_tmp/docs/"
if ! "$HERE/check-dagnats-pin.sh" "$pin_tmp" >/dev/null 2>&1; then
  echo "FAIL: check-dagnats-pin.sh rejects an aligned copy" >&2
  failures=$((failures + 1))
fi
sed -i.bak 's/^ARG DAGNATS_VERSION=.*/ARG DAGNATS_VERSION=v0.0.0/' "$pin_tmp/Dockerfile.dagnats"
if "$HERE/check-dagnats-pin.sh" "$pin_tmp" >/dev/null 2>&1; then
  echo "FAIL: check-dagnats-pin.sh missed a drifted Dockerfile" >&2
  failures=$((failures + 1))
fi
rm -rf "$pin_tmp"

# It must work with no Nango running (it reports that) and always print the two
# enforced names, or an agent silently falls back to asking for the token.
if ! nango_out="$("$HERE/nango-connect.sh" probe --provider hubspot 2>&1)"; then
  echo "FAIL: nango-connect.sh exited non-zero with no Nango running:" >&2
  printf '%s\n' "$nango_out" | sed 's/^/    /' >&2
  failures=$((failures + 1))
else
  if ! printf '%s\n' "$nango_out" | grep -qF "Provider-Config-Key) : hubspot"; then
    echo "FAIL: nango-connect.sh did not print the integration key" >&2
    failures=$((failures + 1))
  fi
  # The id is labelled "intended" because Connect UI, not us, assigns it.
  if ! printf '%s\n' "$nango_out" | grep -Eq 'connection id +: probe'; then
    echo "FAIL: nango-connect.sh did not print the intended connection id" >&2
    failures=$((failures + 1))
  fi
fi

if [ "$failures" -ne 0 ]; then
  echo "test-engines: $failures failure(s)" >&2
  exit 1
fi
echo "test-engines: every declared engine works"
