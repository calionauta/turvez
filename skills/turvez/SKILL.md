---
name: turvez
description: Decide where an automation belongs (turvez recipe vs sandbox one-off vs Dagu vs Windmill), then build it with fieldbook, validate, tests, and approval-gated register. Use when the user asks to automate anything — personal or client.
---

# turvez

Repo: consultancy automation runtime (dagnats + 1 Go worker, 12 generic tasks).
You direct, then build. Read `AGENTS.md` first (normative), route via `llms.txt`.

## Q0 — repo type (before anything else)

- `go.mod` only = template (public). Real clients forbidden — refuse, point to
  `turvez-ops` (`docs/ops-repo.md`) + `scripts/new-client.sh`.
- `TURVEZ_PIN` only = ops repo (private). Validate from the template checkout:
  `OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/workflows go test ./workflows/`.

## Q0b — split: ensuring the ops repo exists (skill-driven)

The skill drives repo creation; scripts never run unasked. Order:

1. **Detect.** In this order: `TURVEZ_OPS_DIR` set and valid? cwd has
   `TURVEZ_PIN`? sibling `../turvez-ops` (or another `*-ops`) exists?
   If ambiguous (several candidates), ask the operator which one — never guess.
2. **Missing + task needs a real client (or real `personal/`) → propose scaffold:**
   `./scripts/new-ops-repo.sh ../turvez-ops` (exact command, wait).
   Explain: local-only by default; `--github-private` pushes to GitHub.
3. **Client missing inside an existing ops → propose:**
   `TURVEZ_OPS_DIR=<ops> ./scripts/new-client.sh <client>` (exact command, wait).
4. **Existence is enforced by the scripts** (`[ -e ]` → abort, no overwrite):
   re-check with `ls`/`test -e` before proposing, reuse what exists.
5. **`gh` rules:** only ever via explicit `--github-private`; without the flag
   nothing touches network. Require `command -v gh` (see `setup.sh` §6),
   push `--private` only (never public), and only after the operator says
   "pode dar push" (or equivalent). `gh` auth failure → stop, hand the operator
   the printed manual command.

## When to use

Any "automate X" request — personal (`personal/`) or client (`<client>/`).
Direct FIRST via `references/decision-tree.md`; build here only if level 0.
Methodology questions (discover, scope, runbook): see `references/methodology.md`.

## Confirm policy (no exceptions)

- `register`, `run start/cancel`, `trigger disable/delete`, sandbox `--secret-env`: show exact command, wait.
- Repo-creating ops: `new-ops-repo.sh`, `new-client.sh`, `gh repo create`
  (any push): show exact command, wait. Recipe edits inside an existing
  client stay in the build loop below — no per-write confirm.
- New Go handler: propose only, human lands (table tests required).
- Secrets by NAME only. Never print values.

## Build loop (level 0)

1. Read `workflows/<client>/README.md` + closest `workflows/_templates/*.json`.
   Fieldbook thin or tool choice open? → `references/client-intake.md`
   (≤2 fact questions; local degustia ranker for tool picks).
2. Copy → `<client>/` prefix → envelope-tolerant first step `((.data // .) | …)`.
3. `dagnats workflow validate` + `go test ./workflows/`.
4. Propose `register` + triggers. Never run unasked.
5. Traps: durations ns or Go strings; `$schema` accepted (optional); `on_failure`→`aux_steps`, no `depends_on` (`docs/writing-recipes.md`).

## Examples

### Route a HubSpot request

**Input:** "deal qualified → notify #sales, create Notion page; >10k needs approval"
**Output:** level 0 — recipe with native `approval` step; propose validate + register.

### Route a backup script

**Input:** "backup via SSH toda noite nos 3 servidores do cliente"
**Output:** level 2 Dagu ad-hoc (`run: ssh …` + retry), NOT a dagnats handler; no repo changes.

## Edge cases

- Engine lacks the feature (e.g. SSH exec): say so, mark workaround, never invent `task:` names.
- Agent loop / autonomous LLM with tools → not a recipe (Q1.5 in `references/decision-tree.md`).
- `llm.complete` output is DATA: validate schema via `transform.json`, never exec it.
- Secret value spotted in recipe/fieldbook: stop, fix to names only, rotate the value.

## Test cases

Should activate: "automatiza X pra mim", "cria recipe pro cliente acme", "onde rodo esse script?".
Should NOT activate: "optimize this SQL query" (general coding), "what is dagnats?" (just answer, point to docs).
