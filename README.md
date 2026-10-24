# turvez — the solo consultancy runtime for durable client automations

Turvez is built for forward-deployed engineers: client work that survives
failures, waits for humans, and keeps receipts. Recipes live in git — your
agent drafts them, tests gate them, you approve. One folder per client,
extractable to dedicated infra when they grow. Your own automations run
free on the same engine.

## What it solves, and how

Client automation breaks in predictable ways: a SaaS webhook changes shape
and nobody notices, an approval sits in someone's inbox for days, secrets leak
into a JSON file, one client's failure pages you at 3am with no run id. Turvez
answers each with a mechanism, not a policy doc:

| Breaks when | Turvez mechanism |
|---|---|
| Webhook payload changes shape | Envelope-tolerant first step + `go test` executes every jq expression |
| Human gate takes days | Native `approval`/`sleep` steps (durable in NATS, no worker slot held) |
| Secret leaks into a recipe | Names-only JSON; `go test` fails the build on embedded values; age-encrypted files |
| Silent failures | `on_failure` → notify per recipe + `ops/dlq-watchdog` + external `GET /health` probe |
| One client outgrows the shared box | Same worker binary, different env — extract runbook in `docs/isolating-a-client.md` |

## Architecture

```
dagnats serve (engine: NATS + API + console)
   ▲ tasks (JetStream)            ▲ events/signals
   │                              │
worker (this repo: 12 handlers, secrets by name at runtime)
   │                              │
   ▼                              ▼
external SaaS ◄──────── human (Slack link / curl approve)
```

- **Engine**: dagnats `v0.0.30` (embedded NATS JetStream + HTTP API + console).
  Full diagram and verified behaviors in `ARCHITECTURE.md`.
- **Worker**: one Go binary, twelve generic task handlers (HTTP, jq transform,
  condition, map, delay, Slack/Telegram/Discord/webhook, approval, LLM, DLQ check).
- **Recipes**: `workflows/<client>/*.json` — the automation code. Git is source
  of truth, dagnats KV is a deployment target.
- **Secrets**: names in JSON, values in `env/<client>.env` or age-encrypted
  `secrets/<client>/env.age`, decrypted to memory at boot.

## Stacks and when to use each

Turvez decides *where* work belongs before writing code (`docs/script-execution.md`):

| # | Stack | When | Cost |
|---|---|---|---|
| 0 | **turvez recipes** (this repo) | SaaS glue, cron, human approvals, retries, DLQ | 1 stack, already operated |
| 1 | `scripts/sandbox-run.sh` | One-off LLM-generated script — run isolated, throw away | zero (local docker) |
| 2 | **Dagu** (ad-hoc CLI, not integrated) | Recurrent script/SSH/Docker that stays script-shaped | 1 binary, no DB |
| 3 | **Windmill** (per-client migration) | Team needs generated UI, forms, integration catalog | Postgres + workers |
| D | **degustia** (sister project, optional; PT-BR + en-US) | Interview → tool prescription with global free-tool coverage; free MCP (recommend/render/validate); `scripts/tool-pick.sh` ranks the same catalog locally with no network. PT-BR is a bonus, not a gate | free MCP + local ranker |
| M | **FDEOps** ([fdeops.io](https://www.fdeops.io/), upstream methodology, MIT) | Discovery, scope, and architecture options before building; runbook/handoff after | harness-level skills |

Levels 2 and 3 replace each other per client — never operate both. Degustia and
FDEOps are *inputs* (diagnose, decide), turvez is the *executor* (run, operate).

## Per-client structure and fieldbooks

> Public template keeps only `_templates/`, `example-client/`, `ops/`,
> `personal/` (fixtures). Real clients live in a private `turvez-ops` repo —
> see `docs/ops-repo.md` (scaffold: `./scripts/new-ops-repo.sh ../turvez-ops`).

Everything a client needs lives in one folder plus one env template:

```text
workflows/<client>/
  README.md        # fieldbook: context, contacts, systems, triggers, runbook, decisions
  *.json           # recipes, named <client>/…, triggers <client>-…
env/<client>.env.example   # secret NAMES (commit); env/<client>.env holds values (never)
secrets/<client>/env.age   # same env, age-encrypted (ciphertext OK in git)
```

Conventions (enforced by `go test ./workflows/`): workflow names start with
`<client>/`, trigger ids with `<client>-`, webhook paths contain `<client>`.
Template: `workflows/_templates/FIELDBOOK.md`. Worked example: `example-client/`,
platform automations: `ops/`, your own: `personal/` (same pipeline, you approve).

## Scaling: solo → team, one client → many

**Solo → team.** Add a second operator by appending their age public key to
`secrets/<client>/.recipients` — they decrypt only the clients they support
(`docs/secrets.md`). Migrate to `sops` when per-client recipients become routine
or editing turns weekly (concrete triggers documented, not before).

**One client → many.** Three stages, no redesign (`docs/tenancy-evolution.md`):
1. Single server, isolation by folder + name prefix + env (default).
2. Logical multi-tenant: prefix-checked registration, optional per-client approval tokens.
3. Physical: copy folder + env file to a new host, re-register, repoint webhooks,
   disable on the old box (`docs/isolating-a-client.md`, rollback = two enables).

## Quickstart

```bash
# 0. Skill first (one command, works from any directory):
npx skills add calionauta/turvez --skill turvez -g   # global; drop -g for project-level

# 1. Check prereqs (go, dagnats CLI, docker, env files, degustia ranker)
./scripts/setup.sh
./scripts/setup.sh --init personal   # scaffold gitignored env files

# 2. Engine + worker stack
make up            # dagnats :4222/:8080, worker pulls tasks

# …or locally (needs dagnats serve in another terminal):
go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30
dagnats serve &
cp .env.example .env   # fill real values
make worker

# 3. Register the example recipes
make register

# 4. Smoke: start a run, print status/events
make smoke
```

Console: http://127.0.0.1:8080/console/

## Operator loop

0. **Direct first** — read `workflows/<client>/README.md` (fieldbook) and
   decide where the work belongs (`docs/script-execution.md` Q1–Q4).
   Personal work uses client `personal/`.
1. **Edit JSON** — copy the closest file from `workflows/_templates/`
   into `workflows/<client>/`, keep the `<client>/` name prefix.
2. **Validate** — `dagnats workflow validate workflows/<client>/x.json`
   (or `make validate`; `go test ./workflows/` compiles every jq
   expression and executes samples).
3. **Register** — `dagnats workflow register …` (embedded `triggers`
   register with it) or attach a schedule:
   `dagnats trigger create '<client>/nightly-sync' --cron='0 2 * * *'`.
4. **Run & monitor** — `dagnats run start <name> '<input>' --watch`,
   then `run status|events|output|inspect`, `dlq list|replay`.

## Examples

| Recipe | Flow |
|---|---|
| `hubspot-lead-to-slack` | webhook → HTTP GET → transform → Slack |
| `invoice-paid-route` | fetch → transform → `skip_if` VIP/standard split |
| `nightly-sync` | embedded cron trigger, fully static first step |
| `approval-refund` | native `approval` gate (48h) → refund POST → receipt |
| `personal/ping` | no-op smoke for your own namespace |

Human approval from Slack/email: point the button at the ops shim
(`internal/opshttp`, `OPS_HTTP_ADDR=127.0.0.1:8090`):

```bash
curl -X POST localhost:8090/ops/approvals/<run-id> \
  -H "Authorization: Bearer $OPS_APPROVAL_TOKEN" -d '{"approved":true}'
```

## Docs

- `docs/writing-recipes.md` — recipe anatomy, engine-vs-handler table,
  what the engine allows here, v0.0.30 behaviors, custom-handler checklist.
- `docs/handlers-reference.md` — task inputs/outputs, metadata keys.
- `docs/agent-tools.md` — exact CLI/API for an authoring agent.
- `docs/ops-repo.md` — template (public) vs ops (private `turvez-ops`) split, lifecycle, tenancy.
- `docs/secrets.md` — per-client age encryption, JIT runtime.
- `docs/alerting.md` — operator notifications: on_failure pages, DLQ watchdog, external uptime.
- `docs/tenancy-evolution.md` — stages 1→3 without redesign.
- `docs/isolating-a-client.md` — extract one client to its own host.
- `docs/migrate-to-windmill.md` — Windmill escape hatch (docs only).
- `docs/sandbox-policy.md` — when you need a sandbox (usually you don't) + `scripts/sandbox-run.sh`.
- `docs/script-execution.md` — where to execute: dagnats vs one-off vs Dagu vs Windmill.
- `workflows/_templates/FIELDBOOK.md` — per-client fieldbook template (copy to `workflows/<client>/README.md`).
- `skills/turvez/SKILL.md` — automate-anything skill (direct → confirm → build). Symlink into your harness skills dir.
- `workflows/personal/` — your own automations as client `personal/` (same pipeline, you approve).
- `ARCHITECTURE.md` — diagram, data flow, decisions.
- `CHANGELOG.md` — release history (generated by release-please).
- `AGENTS.md` — agent quick rules (this file's essentials).

## Conventions

- Automation code = workflow JSON. New Go handler = last resort + tests
  + human review. No arbitrary script execution from JSON.
- Secrets by name in JSON (`"auth_secret": "ACME_HUBSPOT_TOKEN"`),
  values in env/age files only.
- Durations in JSON are integer nanoseconds (`30000000000` = 30s) or Go
  duration strings (`"30s"`).
- Secrets are age-encrypted from day one (`secrets/<client>/env.age`,
  `scripts/edit-secrets.sh`); plain `.env` files are local scratch only.
- Local hooks (pre-commit recipe scan + commit-msg Conventional Commits):
  `./scripts/install-hooks.sh`.
- Versioned releases: Conventional Commits → release-please → tag +
  changelog. Ops repos pin `TURVEZ_REF=vX.Y.Z` (tag), not a moving branch.
