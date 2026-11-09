#!/usr/bin/env bash
# test-eval-strategies.sh — the eval-strategies doc's red-proof.
#
# docs/eval-strategies.md is prose, so the check is the contract around it:
# every script, flag and section it names must exist, and the strategies it
# lists must match the code. A renamed script, a removed flag, or a section
# moved without updating the tree fails here instead of in a user's session.
#
#   doc-links     every (scripts|docs|skills|clients)/path in the doc exists
#   doc-flags     every --flag the doc names exists in the named script
#   doc-sections  every §N ref in the doc resolves to a ## N. heading
#   doc-strategies the sampling table lists exactly the STRATEGIES the code has
#   doc-wired     llms.txt routes agents to the doc
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
DOC="$ROOT/docs/eval-strategies.md"

checks=0
fails=0
ok() { checks=$((checks + 1)); }
bad() { echo "FAIL: $*" >&2; fails=$((fails + 1)); }

# --- doc-links -------------------------------------------------------------
# Every path the doc tells a reader to open must exist (same rule as
# test-decisions.sh). Placeholders are skipped.
while read -r p; do
  p="${p%%#*}"; p="${p%/}"
  case "$p" in ''|*'<'*|*'*'*|*'$'*|*'{'*|clients/acme/*|scripts/gen-cases.py) continue ;; esac
  # scripts/gen-cases.py is a roadmap proposal (§6, "not implemented yet") — the
  # doc marks it as such, so a missing file is honest, not a broken link.
  if [ ! -e "$ROOT/$p" ]; then bad "docs/eval-strategies.md points at a missing path: $p"; else ok; fi
done < <(grep -oE '(scripts|docs|skills|clients)/[A-Za-z0-9_./-]+' "$DOC" 2>/dev/null | sort -u || true)

# --- doc-flags -------------------------------------------------------------
# A flag the doc shows in a command must be a real argparse flag in the
# script next to it. Only commands of the form `scripts/<x>.py --flag` are
# checked; prose backtick flags are not attributed to a script.
while read -r line; do
  script=$(echo "$line" | grep -oE 'scripts/[A-Za-z0-9_.-]+\.py' | head -1)
  for flag in $(echo "$line" | grep -oE '\-\-[a-z][a-z-]*'); do
    case "$flag" in
      --help) continue ;;
    esac
    if grep -q -- "\"$flag\"" "$ROOT/$script"; then ok
    else bad "docs/eval-strategies.md shows $flag for $script but the script has no such flag"; fi
  done
done < <(grep -E '^(python3 )?scripts/' "$DOC" || true)

# --- doc-sections ----------------------------------------------------------
# Every §N reference must resolve to a "## N." heading.
for n in $(grep -oE '§[0-9]+' "$DOC" | sed 's/§//' | sort -u || true); do
  if grep -qE "^## $n\\. " "$DOC"; then ok
  else bad "docs/eval-strategies.md references §$n but no '## $n.' heading exists"; fi
done

# --- doc-strategies --------------------------------------------------------
# The sampling table must name exactly the strategies select-cases.py accepts.
if python3 - "$DOC" "$ROOT/scripts/select-cases.py" <<'PY'
import re, sys
doc = open(sys.argv[1]).read()
code = open(sys.argv[2]).read()
strategies = re.search(r'STRATEGIES = \(([^)]+)\)', code).group(1)
code_strats = set(re.findall(r'"(\w+)"', strategies))
table = doc[doc.index("## Sampling strategy selection"):]
table_strats = set(re.findall(r'\| `(\w+)`', table))
for s in sorted(code_strats - table_strats):
    print(f"FAIL: sampling table omits strategy {s!r} that select-cases.py supports", file=sys.stderr)
for s in sorted(table_strats - code_strats):
    print(f"FAIL: sampling table lists {s!r} but select-cases.py does not support it", file=sys.stderr)
sys.exit(1 if code_strats - table_strats or table_strats - code_strats else 0)
PY
then ok; else bad "docs/eval-strategies.md sampling table disagrees with select-cases.py STRATEGIES"; fi

# --- doc-wired -------------------------------------------------------------
if grep -q "docs/eval-strategies.md" "$ROOT/llms.txt"; then ok
else bad "llms.txt does not route agents to docs/eval-strategies.md"; fi

if [ "$fails" -gt 0 ]; then
  echo "test-eval-strategies: $fails failure(s) across $checks checks" >&2
  exit 1
fi
echo "test-eval-strategies: ok ($checks checks)"
