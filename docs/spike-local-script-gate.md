# Spike: a local gate that catches what `wmill lint` passes

Status: proposal, not started.
Evidence: measured 2026-11-01 against the live `ai-agents` workspace.

## The gap

`wmill lint` validates the **flow schema**, not the **script body**. A flow whose
script raises `NameError` on the first line lints clean:

```
$ wmill lint q.flow --fail-on-warn
✅ Lint passed (1 file(s) validated, 1 valid)

$ python3 -m py_compile q.flow/a.py
(exit 0 — syntax is fine)

$ cat q.flow/a.py
import os
def main():
    cfg = CONFIG_FOLDER + "/x"    # NameError
    return missing_fn(cfg)        # NameError

$ ruff check --select F821 q.flow/a.py
F821 Undefined name `CONFIG_FOLDER`
F821 Undefined name `missing_fn`
```

The same thing happened in a real delivery: the agent shipped `on_failure.py`
with `CONFIG_FOLDER` undefined, and that handler *masked the original error*.
It only surfaced because the agent pushed and ran it. In the second delivery it
shipped `import os` unused — caught by `ruff` instantly, invisible to `lint`.

In Windmill, a broken script is discovered at **run time**, after a push, often
after a schedule fires at 9am. That is the most expensive place to find it.

## Why this is the next bottleneck, not docs

The previous optimisation measured 340 → 212 tool calls. The remainder is
dominated by push-verify-fix cycles. Every cycle that a local gate could have
prevented costs: push (1 call) + trigger/run (1) + read job output (1) + edit
(1) + push again (1). Roughly 4-5 calls per avoidable bug.

The `ruff F821` check above takes **~50 ms local**, no network, no push.

## Proposed spike (timeboxed, one sitting)

1. Add `scripts/lint-scripts.sh` that runs, on every `*.py` in
   `clients/*/automation/windmill/**`:
   - `ruff check --select F821,F401,F811,E9` (undefined names, unused/duplicate
     imports, syntax) — the classes that `lint` and `py_compile` both miss
   - a **names-only** scan for the variables the flow declares in
     `input_transforms`, so a script referencing an undeclared step input fails
     locally
2. Wire it into `scripts/validate.sh` next to `check-windmill.sh`, so it runs in
   CI and in the fast path.
3. Measure: does a delivery agent's push-verify-fix count drop?

## Acceptance

- A flow with an undefined name fails `validate.sh` with a message naming the
  file and the symbol.
- A clean flow still passes (no false positives on Windmill's injected
  `wmill`/`context` globals — verify, they are not imports).
- Expected effect: fewer server round-trips per delivery, measurable the same
  way the last change was (tool calls before/after, same briefing).

## Risk

`ruff` may flag legitimate patterns: Windmill injects names, and scripts that
read secrets at runtime look like undefined locals to a static checker. If the
false-positive rate is high, the gate gets ignored — that is worse than no gate.

**Measured against real output**: `ruff --select F821,F811,E9` on the nine
Python scripts of the successful delivery produced **zero findings**. The gate
would not have cried wolf. Broaden the rule set only after that is still true.

## Open question for the operator

Is `ruff` acceptable as a new dev dependency, given the repo rule "never add
dependencies without asking"? It is not a runtime dependency (gate only), but it
is a new tool in the loop.

**Partially answered during this mapping**: a ~20-line `ast` walker with no
dependency detects the same `F821` cases. Same file, both agree:

```
$ python3 astgate.py q.flow/a.py
q.flow/a.py: undefined at lines [4, 5]

$ ruff check --select F821 q.flow/a.py
F821 Undefined name `CONFIG_FOLDER`
F821 Undefined name `missing_fn`
```

The walker is weaker on unused imports (`F401`) and much weaker on real name
resolution (it will accept a typo as long as anything with that name was ever
bound). `ruff` is the better tool if a dev dependency is fine; the walker is the
zero-dependency floor. Decide before the spike starts, not during it.

---

# Candidates considered, and why this one wins

Three bottlenecks were mapped with `ripwire --for` / `ripwire --recall`, `cymbal`,
and direct measurement. `ripwire` returned `confidence="low" margin="0"` on the
query, which is itself the finding: the bottleneck is **not a symbol in the code**,
so no code-navigation tool can rank it. It had to be measured.

## Table

| # | Candidate | Evidence | Cost to fix | Verdict |
|---|---|---|---|---|
| 1 | **Local script gate** (this spike) | `wmill lint` ✅ + `py_compile` ✅ on a file with 2 `NameError`s; `ruff` finds both in ~50 ms. A real delivery shipped exactly this bug and it masked the original error. | small | **next spike** |
| 2 | Windmill scaffold emits nothing | `new-client.sh` creates 0 files under `automation/windmill/`, while the dagnats path creates 2 recipes | medium | later |
| 3 | `smoke-test.sh` is dagnats-only | 44 lines, zero `wmill` references, yet Windmill is the declared default | medium | later |

## Why not #2

The second delivery created the correct Windmill structure unaided; the scaffold
would save typing, not thinking. It also cannot be validated without a live
server, so it inherits #1's problem.

## Why not #3

A smoke test requires a deployed client. It shortens the *end* of a delivery,
while #1 shortens every *iteration* inside it. #1 is upstream of #3: a correct
local gate means the smoke test has fewer surprises to catch.

## What #1 unlocks

Once scripts are statically verified before push, the push-verify-fix loop shrinks
from 4-5 calls per bug to 0 for whole bug classes. That is the same lever the
previous change pulled (dead-ends, not document volume), applied one layer deeper.
