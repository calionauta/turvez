#!/usr/bin/env bash
# test-decisions.sh — the decision tree's red-proof.
#
# The tree (skills/turvez/references/decision-tree.md) is prose, so there is
# nothing to mutate and nothing a unit test can call. What CAN be proven is the
# contract around it: every branch names an artifact that exists, the artifact's
# gate exists, and the docs and the repo agree on the same facts. That is what
# fails in practice — an engine added without a scaffold, a script renamed, the
# tree pointing at a capability with no code, a doc contradicting the unit it
# says to index.
#
# One scenario per decision, each deterministic and cheap (no network, no CLIs):
#
#   engine:<name>      every engines.json entry is in the tree, the scaffold
#                      and the README, and its declared gate path exists
#   capability-genkit  not an engine, has a scaffold, a compile gate, one pin
#   library-goai       the only model path is a go.mod dependency
#   library-nango      the credential layer has a script and a doc
#   level-1-sandbox    the one-off path has a runner
#   level-2-dagu       the script-shaped path has a doc
#   retrieval-unit     chunk in / parent out is stated the same way everywhere
#   doc-links          no skill/doc names a path that does not exist
#   skill-cache        the always-loaded skill holds no dated facts
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
TREE="$ROOT/skills/turvez/references/decision-tree.md"
SKILL="$ROOT/skills/turvez/SKILL.md"

checks=0
fails=0
ok() { checks=$((checks + 1)); }
bad() { echo "FAIL: $*" >&2; fails=$((fails + 1)); }
need_file() { # path, why
  if [ -e "$ROOT/$1" ]; then ok; else bad "missing $1 ($2)"; fi
}
need_text() { # file, pattern, why
  if grep -qE "$2" "$ROOT/$1" 2>/dev/null; then ok; else bad "$1 does not mention '$2' ($3)"; fi
}

json() { python3 -c "import json,sys;d=json.load(open(sys.argv[1]));$2" "$ROOT/engines.json"; }

default_engine="$(json 1 'print(d["default"])')"
engines="$(json 1 'print(" ".join(d["engines"]))')"

# --- engine:<name> ---------------------------------------------------------
# The default is a policy fact, so it must agree across the manifest and the docs.
need_text engines.json "\"default\": \"$default_engine\"" "the manifest declares its own default"
need_text skills/turvez/references/decision-tree.md "\`$default_engine\`" "the tree states the default engine"
need_text README.md "$default_engine" "the README states the default engine"

for e in $engines; do
  need_text engines.json "\"$e\"" "engine listed in the manifest"
  need_text skills/turvez/references/decision-tree.md "$e" "the tree routes to this engine"
  need_text scripts/new-client.sh "$e" "the scaffold supports this engine"
  need_text README.md "$e" "the stack table lists this engine"
  # validate is either a repo script or a documented external command.
  gate="$(json 1 "print(d['engines']['$e']['validate'])")"
  case "$gate" in
    scripts/*) need_file "$gate" "gate declared by engines.json for $e" ;;
    *) need_text docs/script-execution.md "$e" "external gate for $e is documented" ;;
  esac
done

# --- capability-genkit -----------------------------------------------------
# Never an engine; a scaffold, a compile gate, and one pin the docs agree on.
if grep -q '"genkit"' "$ROOT/engines.json"; then
  bad "genkit is an engine in engines.json (it is a capability, docs/genkit.md)"
else
  ok
fi
need_file scripts/new-agent-service.sh "Genkit scaffold (Q1.5)"
need_file scripts/templates/agent-service/main.go.tmpl "scaffold source"
need_file scripts/check-agent-template.sh "scaffold compile gate"
need_text docs/genkit.md "Install and scaffold" "the install path is documented"
need_text skills/turvez/references/decision-tree.md "new-agent-service.sh" "the tree names the scaffold"
need_text README.md '```mermaid' "the README renders the tree"
for q in Q0 Q0.8 Q1.5 Q1.6 Q2 Q4; do
  need_text README.md "$q" "the rendered tree keeps question $q"
done
# The README diagram once kept the old Q3/Q5 numbering after the tree moved on.
for old in 'Q3\.' 'Q5\.'; do
  if grep -qE "$old" "$ROOT/README.md"; then
    bad "README numbers a decision $old (canonical ids are Q0.8 and Q1.6)"
  else ok; fi
done
# A floating @latest would install a Genkit the pinned gate never compiled.
if grep -rq "firebase/genkit/go@latest" "$ROOT/docs" "$ROOT/skills" 2>/dev/null; then
  bad "a Genkit install still says @latest (it must be the pin the gate compiles)"
else
  ok
fi
pinned="$(sed -n 's/.*GENKIT_VERSION:-\(v[0-9][0-9.]*\).*/\1/p' "$ROOT/scripts/check-agent-template.sh" | head -1)"
if [ -n "$pinned" ]; then ok; else bad "check-agent-template.sh has no pinned GENKIT_VERSION"; fi
need_text docs/genkit.md "genkit $pinned" "docs name the version the gate compiles"
need_text docs/genkit.md "go@$pinned" "the documented install uses the gated pin, not @latest"
if grep -qF 'go@${GENKIT_VERSION}' "$ROOT/scripts/new-agent-service.sh"; then
  ok
else
  bad "new-agent-service.sh does not print the pin it reads from the gate"
fi
# The always-loaded skill must not carry a version: it would rot and break the prefix.
if grep -q "$pinned" "$SKILL"; then
  bad "SKILL.md hardcodes $pinned (versions belong in the scaffold/gate, not the loaded prefix)"
else
  ok
fi

# --- library-goai ----------------------------------------------------------
# One model path: goai is a dependency; Genkit stays out of the build.
need_text go.mod "zendev-sh/goai" "goai is the only model path"
if grep -q "firebase/genkit" "$ROOT/go.mod"; then
  bad "genkit leaked into go.mod (docs/genkit.md: capability, not a dependency)"
else
  ok
fi
need_text skills/turvez/references/decision-tree.md "goai" "the library gate names goai"

# --- library-nango ---------------------------------------------------------
need_file scripts/nango-connect.sh "operator connects a provider in Nango"
need_file docs/nango.md "credential layer doc"
need_text skills/turvez/references/decision-tree.md "Nango" "the tree routes credentials to Nango"

# --- level-1-sandbox -------------------------------------------------------
need_file scripts/sandbox-run.sh "level 1 one-off runner"
need_file docs/sandbox-policy.md "level 1 policy doc"

# --- level-2-dagu ----------------------------------------------------------
need_file docs/script-execution.md "level 2 and engine competition doc"

# --- retrieval-unit --------------------------------------------------------
# The one fact that already drifted once: what is indexed vs what is read.
for f in docs/retrieval.md skills/turvez/references/retrieval.md docs/workflows.md README.md ARCHITECTURE.md; do
  need_text "$f" "reading unit" "the indexing/reading split is stated"
done
for f in docs/retrieval.md skills/turvez/references/retrieval.md docs/workflows.md README.md ARCHITECTURE.md; do
  if grep -qE "(retrieve|returns|Read) whole units" "$ROOT/$f"; then
    bad "$f says whole units are returned/read (the chunk is the routing unit, the parent is read)"
  else ok; fi
done

# --- doc-links -------------------------------------------------------------
# Every path a reader is told to open must exist. Placeholders are skipped.
for f in skills/turvez/SKILL.md skills/turvez/references/decision-tree.md llms.txt README.md ARCHITECTURE.md; do
  while read -r p; do
    p="${p%%#*}"; p="${p%/}"
    # Placeholders, and `acme`, the example client that lives in the ops repo.
    case "$p" in ''|*'<'*|*'*'*|*'$'*|*'{'*|clients/acme/*) continue ;; esac
    if [ ! -e "$ROOT/$p" ]; then bad "$f points at a missing path: $p"; else ok; fi
  done < <(grep -oE '(scripts|docs|skills|clients)/[A-Za-z0-9_./-]+' "$ROOT/$f" 2>/dev/null | sort -u || true)
done

# No orphan reference: a file the skill never routes to is dead weight.
for ref in "$ROOT"/skills/turvez/references/*.md; do
  base="$(basename "$ref")"
  if grep -q "references/$base" "$SKILL" "$TREE"; then ok; else bad "orphan reference: references/$base is never referenced"; fi
done

# --- skill-cache -----------------------------------------------------------
# SKILL.md is re-read every request: a dated fact there breaks the prefix.
if grep -qE '20[0-9]{2}' "$SKILL"; then
  bad "SKILL.md contains a 4-digit year (dated facts break the stable prefix)"
else
  ok
fi
need_text skills/turvez/SKILL.md "Reading order" "the cache rule is written down"

if [ "$fails" -gt 0 ]; then
  echo "test-decisions: $fails failure(s) across $checks checks" >&2
  exit 1
fi
echo "test-decisions: ok ($checks checks, $(echo $engines | wc -w | tr -d ' ') engine(s), default $default_engine)"
