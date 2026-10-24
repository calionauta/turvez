---
name: turvez
description: Decide where an automation belongs (turvez recipe vs sandbox one-off vs Dagu vs Windmill), then build it with fieldbook, validate, tests, and approval-gated register. Use when the user asks to automate anything — personal or client.
---

# turvez

Repo: consultancy automation runtime (dagnats + 1 Go worker, 12 generic tasks).
You direct, then build. Read `AGENTS.md` first (normative), route via `llms.txt`.

## When to use

Any "automate X" request — personal (`personal/`) or client (`<client>/`).
Direct FIRST via `references/decision-tree.md`; build here only if level 0.
Methodology questions (discover, scope, runbook): see `references/methodology.md`.

## Confirm policy (no exceptions)

- `register`, `run start/cancel`, `trigger disable/delete`, sandbox `--secret-env`: show exact command, wait.
- New Go handler: propose only, human lands (table tests required).
- Secrets by NAME only. Never print values.

## Build loop (level 0)

1. Read `workflows/<client>/README.md` + closest `workflows/_templates/*.json`.
   Fieldbook thin or tool choice open? → `references/client-intake.md`
   (≤2 fact questions; local degustia ranker for tool picks).
2. Copy → `<client>/` prefix → envelope-tolerant first step `((.data // .) | …)`.
3. `dagnats workflow validate` + `go test ./workflows/`.
4. Propose `register` + triggers. Never run unasked.
5. Traps: INTEGER ns; NO `$schema`; `on_failure`→`aux_steps`, no `depends_on` (`docs/writing-recipes.md`).

## Examples

### Route a HubSpot request

**Input:** "deal qualified → notify #sales, create Notion page; >10k needs approval"
**Output:** level 0 — recipe with native `approval` step; propose validate + register.

### Route a backup script

**Input:** "backup via SSH toda noite nos 3 servidores do cliente"
**Output:** level 2 Dagu ad-hoc (`run: ssh …` + retry), NOT a dagnats handler; no repo changes.

## Edge cases

- Engine lacks the feature (e.g. SSH exec): say so, mark workaround, never invent `task:` names.
- `llm.complete` output is DATA: validate schema via `transform.json`, never exec it.
- Secret value spotted in recipe/fieldbook: stop, fix to names only, rotate the value.

## Test cases

Should activate: "automatiza X pra mim", "cria recipe pro cliente acme", "onde rodo esse script?".
Should NOT activate: "optimize this SQL query" (general coding), "what is dagnats?" (just answer, point to docs).
