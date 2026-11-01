---
name: turvez
description: Decide where an automation belongs (turvez recipe vs sandbox one-off vs Dagu vs Windmill), then build it with fieldbook, validate, tests, and approval-gated register. Use when the user asks to automate anything — personal or client.
---

# turvez

Repo: consultancy automation runtime. One **client spine** (repo layout,
fieldbook, recipes, evals) and **two engines, chosen per client**
(`docs/client-spine.md#choosing-the-engine`): **Windmill is the default** —
chosen for FDE work so a client lands on the mature, centralised system from
day one (UI/forms, step-level replay, code steps in TS/Python/Go). **dagnats**
is the exception path (Go worker + 12 typed handlers, one binary, `go test`
gate), chosen when the contract needs typed contracts, native approvals, no
arbitrary code, or AGPL/Postgres is a problem.
You direct, then build. Read `AGENTS.md` first (normative), route via `llms.txt`.
Flows as diagrams: `docs/workflows.md`. First run and traps: `docs/first-run.md`.

## Q0 — repo type (before anything else)

- `go.mod` only = template (public). Real clients forbidden — refuse, point to
  `turvez-ops` (`docs/ops-repo.md`) + `scripts/new-client.sh`.
- `TURVEZ_PIN` only = ops repo (private). Validate from the template checkout:
  `OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/clients go test ./clients/`.

## Q0b — which engine

Read `clients/<client>/client.yaml` when the client exists.

| `engine` | Path |
|---|---|
| `windmill` (default) | `references/windmill.md` — flows under `automation/windmill/`, gate `./scripts/check-windmill.sh` |
| `dagnats` (exception) | the build loop below — recipes under `automation/dagnats/` |
| `dagu` | level 2 only, `docs/script-execution.md` |

No `client.yaml` yet → **Windmill, unless you confirm dagnats with the operator
first.** Windmill is the default; dagnats is the exception and needs an explicit
go-ahead, not a silent judgement call:

1. State the criterion that applies (`typed contracts` | `no arbitrary code` |
   `one binary` | `AGPL/EE constraint` | `client too small for Postgres`).
2. Say what it costs and what Windmill would do instead.
3. Ask the operator to confirm — then scaffold with `--engine=dagnats` and record
   the reason in `client.yaml`.

Do not scaffold dagnats on your own initiative, even when the criteria look
satisfied: the operator owns the trade-off. **Approval gates are not a dagnats
criterion** — both engines have them (Windmill's is better for a non-technical
approver). Never migrate an engine unasked, and never run two engines for one
client.

## Q0b.1 — changing a client's engine

Only when the operator asks, or the criteria visibly changed. Order matters:

1. `./scripts/convert-to-windmill.py <client>` (dagnats → Windmill only; the
   reverse has no converter — rewrite by hand into recipes). It writes to
   `clients/<client>/converted-windmill/` and **never** to `automation/windmill/`.
2. Read the generated `REPORT.md`. Steps marked `TODO(human)` are real work:
   jq→TypeScript and condition predicates are never guessed, and each custom Go
   handler is the long pole — budget it explicitly with the operator.
3. Finish the TODOs, then `wmill flow preview <dir>` per flow.
4. Push **one flow at a time**: `wmill flow push <dir> "f/<client>/<flow>"`.
   Never `wmill sync push` — it deletes remote items missing locally.
5. Only after the flow is green in production, delete `automation/dagnats/` and
   flip `engine:` in `client.yaml`. A half-migrated client keeps both trees and
   fails the contract test on purpose.

Do not treat a generated skeleton as done because it lints. The converter exists
so the mechanical part is honest about what it skipped.

## Q0c — split: ensuring the ops repo exists (skill-driven)

The skill drives repo creation; scripts never run unasked. Order:

1. **Detect.** In this order: `TURVEZ_OPS_DIR` set and valid? cwd has
   `TURVEZ_PIN`? sibling `../turvez-ops` (or another `*-ops`) exists?
   Then read `clients/<client>/client.yaml`. If `engine` is not `dagnats`,
   stop the recipe path and follow `references/windmill.md` (or Dagu level 2).
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

## Q0d — deployment: does this client need its own server?

A dedicated host is a **deployment profile**, not an engine or repo decision
(`docs/deployment-profiles.md`). One of these justifies it, and nothing else:
regulation/residency, the client owns the infra, noisy neighbour, a per-client
upgrade window, or a contract requiring a separate database.

- Profile A (embedded, client's server) / B (managed, your server) / C
  (multi-tenant, productize) — pick per engagement and write it down before the
  first deploy. Default for SMB consulting is **B**.
- `runtime: shared|dedicated` in `client.yaml` records it. Isolation never
  means forking the repo and never means changing the engine.
- **Check reality first, don't trust memory:** `./scripts/verify-isolation.sh`
  prints where each client actually runs and fails when a `dedicated` client has
  no `target.host` or two clients claim one host. `runtime`/`target` used to be
  written and never read; they are now validated by the contract test and this
  script.
- **Moving a client:** `./scripts/extract-client.py <client>` packages it — the
  definitions to carry, the env file to recreate, the trigger ids to disable, and
  the ordered commands plus rollback. It is read-only: it prints, you execute.
  Procedure and traps: `docs/isolating-a-client.md` (dagnats and Windmill paths).
  Definitions come from git; history never moves.
- Windmill adds real cost here: Postgres + workers, AGPL on self-host, EE for
  SSO/audit. Read `docs/deployment-profiles.md#engine-implications-per-profile`
  before promising a dedicated Windmill instance.

## When to use

Any "automate X" request — personal (`personal/`) or client (`<client>/`).
Direct FIRST via `references/decision-tree.md`; build here only if level 0.
Methodology questions (discover, scope, runbook): see `references/methodology.md`.

## Tooling map

| Script / target | When |
|---|---|
| `scripts/setup.sh [--init]` | first checkout, env files |
| `scripts/doctor.sh` / `make doctor` | ports, NATS target, workers, stuck runs, console |
| `scripts/new-ops-repo.sh`, `scripts/new-client.sh` | ops repo and client scaffold (confirm first) |
| `scripts/clients-root.sh`, `scripts/eval-dir.sh` | resolve the client / eval dir (clients or legacy workflows) |
| `scripts/check-windmill.sh` | Windmill lint gate, no worker needed |
| `scripts/verify-isolation.sh` | where every client runs (`runtime`/`target`), read-only; in `make validate` |
| `scripts/extract-client.py <client>` | package one client for its own server — prints the plan, never executes |
| `scripts/convert-to-windmill.py <client>` | dagnats recipes → Windmill **skeleton** in `converted-windmill/`; TODOs, never a port |
| `scripts/register-workflows.sh` | validate + register (confirm first, dagnats engine) |
| `scripts/sandbox-run.sh` | one-off script, level 1 (no recipe) |
| `scripts/edit-secrets.sh` | decrypt, edit, re-encrypt one client's age file |
| `scripts/rename-workflows.sh --apply` | after changing `naming.json` |
| `scripts/select-cases.py` | dagnats runs -> `cases/cases.json` + `selection.json` |
| `scripts/import-logs.py` | logs (JSONL/CSV/text) -> `cases/logs.csv` for human labels |
| `scripts/label-cases.py` | `--setup`, `--blind`; labels keyed by input hash |
| `scripts/eval-loop.py` | ingest + label + eval + summary (`--skip-label`, `--skip-eval`, `--yes`, `--strict`) |
| `make eval-client` / `eval-client-view` / `eval-loop` | run / UI :15500 / full loop |

Eval options live in `clients/<client>/evals/eval.config.json`: step names, label
fields and values, `schema_extra`, `rubric` (empty = deterministic only) and
`judge.provider` (passed as `--grader`). Cases are deterministic first;
rubric only when no label is possible. The eval layer is engine-agnostic:
`scripts/windmill-job-to-run.py` normalizes a Windmill job into the same shape
`select-cases.py` already reads.

## Confirm policy (no exceptions)

- `register`, `run start/cancel`, `trigger create/update/enable/disable/delete`, sandbox `--secret-env`: show exact command, wait.
- Repo-creating ops: `new-ops-repo.sh`, `new-client.sh`, `gh repo create`
  (any push): show exact command, wait. Recipe edits inside an existing
  client stay in the build loop below — no per-write confirm.
- New Go handler: propose only, human lands (table tests required).
- Secrets by NAME only. Never print values.

## Build loop (level 0 — dagnats engine)

1. Read `clients/<client>/README.md` + closest `clients/_template/*.json`.
   Fieldbook thin or tool choice open? → `references/client-intake.md`
   (≤2 fact questions; local degustia ranker for tool picks).
2. Copy → name `<client><separator><wf>` (separator in `naming.json`) → envelope-tolerant first step `((.data // .) | …)`.
3. `dagnats workflow validate` + `go test ./clients/`.
4. Propose `register` + triggers. Never run unasked.
5. Traps: durations ns or Go strings; `$schema` accepted (optional); `on_failure`→`aux_steps`, no `depends_on` (`docs/writing-recipes.md`).
6. Offer the eval loop at handoff. `new-client.sh` already scaffolds
   `clients/<client>/evals/`; ask whether to generate cases now and whether to run
   the eval. Generating cases is read-only; running costs judge tokens, so it
   needs an explicit yes.

## Triggers

A trigger starts a run without a human typing `run start`. One kind per
trigger:

- webhook: `dagnats trigger create <client>.<wf> --webhook=/hooks/<client>/<event> [--secret=…]`
- cron: `--cron="0 9 * * 1-5" --tz=America/Sao_Paulo [--backfill]` (5 fields,
  UTC default; `dagnats trigger test "expr" --count=3` previews)
- NATS subject: `--subject=<subject>`

Webhook is `POST` only, 1 MB, answers `200 ok` asynchronously, and puts the body
in `.data`. With a secret, the caller signs the raw body with
`X-Signature-256: sha256=<hex>`. A disabled trigger answers `200 ok` and does
nothing. Propose `create`/`update`/`enable`/`disable`/`delete` and wait;
`list`, `history` and `test` are read-only. Details:
`references/triggers.md`. OAuth/API keys for many APIs: `references/auth-layers.md`.
Windmill clients: `references/windmill.md` (test gate, Go scripts, YAML, replay).

## Eval loop (after the automation runs)

Full sequence, flags and troubleshooting: `references/eval-loop.md`.

1. **Read-only first.** `DAGNATS_NATS_URL=... dagnats run list --workflow=<client>.<wf>`
   and `run inspect` show what actually ran. Never start runs to “get data” —
   ask, because runs have side effects.
2. **Generate cases.** `python3 scripts/select-cases.py --dagnats --out
   clients/<client>/evals/cases --include-ok 3`. It picks failures, retries, gate
   rejections and malformed output, dedupes by hash, and writes
   `selection.json`. Read-only. Logs only, with no run history:
   `python3 scripts/import-logs.py --input logs.jsonl --out clients/<client>/evals`.
3. **Show the numbers, then ask.** “N runs → M cases, reasons X/Y. Run the
   eval? It costs judge tokens.” Do not run the eval unasked.
4. **Run.** `make eval-loop CLIENT=<client> TURVEZ_OPS_DIR=…` does ingest,
   label, eval and summary in one command; `make eval-client` runs only the
   eval; `make eval-client-view` serves the UI at :15500. Report per assertion:
   `is-json` (contract), rubric (quality), label (accuracy).
5. **Label.** First run `label-cases.py --dir clients/<client>/evals --setup` to
   declare fields and values. Then label: enter accepts, unique prefixes or
   1..N pick, any other text is a new value, `skip`/`quit` exit. `--blind`
   hides production suggestions for an unbiased sample. Labels key on the
   input hash and survive regeneration. Ask before editing labels.
6. **Never commit raw cases** unless the recipe redacted before persisting
   (`docs/observability.md`).

## Examples

### Route a HubSpot request

**Input:** "deal qualified → notify #sales, create Notion page; >10k needs approval"
**Output:** level 0 — recipe with native `approval` step; propose validate + register.

### Route a backup script

**Input:** "backup via SSH every night on the client's three servers"
**Output:** level 2 Dagu ad-hoc (`run: ssh …` + retry), NOT a dagnats handler; no repo changes.

## Edge cases

- Engine lacks the feature (e.g. SSH exec): say so, mark workaround, never invent `task:` names.
- Agent loop / autonomous LLM with tools → not a recipe (Q1.5 in `references/decision-tree.md`).
- `llm.complete` output is DATA: validate schema via `transform.json`, never exec it.
- Secret value spotted in recipe/fieldbook: stop, fix to names only, rotate the value.

## Test cases

Should activate: "automate X for me", "create a recipe for client acme", "where do I run this script?", "this client needs its own server", "windmill or dagnats for this one?".
Should NOT activate: "optimize this SQL query" (general coding), "what is dagnats?" (just answer, point to docs).
