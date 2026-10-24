# Consultancy Automation Runtime

Production-oriented template for running durable customer automations on
**dagnats** (DAG workflows on NATS JetStream) with a single Go worker.
The operator edits **workflow JSON recipes**; Go handlers are shared
platform code.

- Engine: dagnats `v0.0.30` (`dagnats serve`: embedded NATS + API +
  console). Compatibility notes in `ARCHITECTURE.md`.
- Worker: one binary, eight generic handlers (HTTP, jq transform,
  condition, map, delay, Slack, approval, LLM).
- Recipes: `workflows/<client>/*.json`, namespaced `<client>/…`.
- Secrets: per-client env files (`env/<client>.env`) + optional
  age-encrypted files (`secrets/<client>/`). Never in JSON.
- Tenancy: prefix conventions + extract runbook; Windmill is a
  documented escape hatch, not a dependency.

## Quickstart

```bash
# 1. Engine + worker stack
make up            # dagnats :4222/:8080, worker pulls tasks

# …or locally (needs dagnats serve in another terminal):
go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30
dagnats serve &
cp .env.example .env   # fill real values
make worker

# 2. Register the example recipes
make register

# 3. Smoke: start a run, print status/events
make smoke
```

Console: http://127.0.0.1:8080/console/

## Operator loop

1. **Edit JSON** — copy the closest file from `workflows/_templates/`
   into `workflows/<client>/`, keep the `<client>/` name prefix.
2. **Validate** — `dagnats workflow validate workflows/<client>/x.json`
   (or `make validate`; `go test ./workflows/` executes every jq
   expression against samples).
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

Human approval from Slack/email: point the button at the ops shim
(`internal/opshttp`, `OPS_HTTP_ADDR=127.0.0.1:8090`):

```bash
curl -X POST localhost:8090/ops/approvals/<run-id> \
  -H "Authorization: Bearer $OPS_APPROVAL_TOKEN" -d '{"approved":true}'
```

## Operator alerts

Failures page the operator three ways (see `docs/alerting.md`):
`on_failure` → notify in every production recipe (example:
`approval-refund.json`), the `ops/dlq-watchdog.json` cron watching
`DEAD_LETTERS` depth, and an external uptime probe on `GET /health`
(dagnats cannot alert when itself is down).

## Docs

- `docs/writing-recipes.md` — recipe anatomy, engine-vs-handler table,
  what the engine allows here, v0.0.30 traps, custom-handler checklist.
- `docs/handlers-reference.md` — task inputs/outputs, metadata keys.
- `docs/agent-tools.md` — exact CLI/API for an authoring agent.
- `docs/secrets.md` — per-client age encryption, JIT runtime, sops evaluation.
- `docs/alerting.md` — operator notifications: on_failure pages, DLQ watchdog, external uptime.
- `docs/tenancy-evolution.md` — stages 1→3 without redesign.
- `docs/isolating-a-client.md` — extract one client to its own host.
- `docs/migrate-to-windmill.md` — Windmill escape hatch (docs only).
- `docs/sandbox-policy.md` — when you need a sandbox (usually you don't) + `scripts/sandbox-run.sh`.
- `docs/script-execution.md` — where to execute: dagnats vs one-off vs Dagu vs Windmill.
- `workflows/_templates/FIELDBOOK.md` — per-client fieldbook template (copy to `workflows/<client>/README.md`).
- `skills/dagnats-automation/SKILL.md` — automate-anything skill (direct → confirm → build). Symlink into your harness skills dir.
- `workflows/personal/` — your own automations as client `personal/` (same pipeline, you approve).
- `ARCHITECTURE.md` — diagram, data flow, decisions.
- `AGENTS.md` — agent quick rules (this file's essentials).

## Conventions

- Automation code = workflow JSON. New Go handler = last resort + tests
  + human review. No arbitrary script execution from JSON.
- Secrets by name in JSON (`"auth_secret": "ACME_HUBSPOT_TOKEN"`),
  values in env/age files only.
- Durations in JSON accept **integer nanoseconds** (`30000000000` = 30s)
  or Go duration strings (`"30s"`); both parse.
