# AGENTS.md — consultancy automation runtime (client spine; Windmill default,
# dagnats legacy)

## Repo type (Q0 — check before anything else)

- `go.mod` only = **template** (this repo, public). Real clients forbidden here
  (`go test` fails them) — they live in private `turvez-ops` (`docs/ops-repo.md`).
- `TURVEZ_PIN` only = **ops repo** (private, data-only). Validate from the
  template checkout: `OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/clients go test ./clients/`.
- Env: `TURVEZ_CHECKOUT` (template clone), `TURVEZ_OPS_DIR` (ops root),
  `WORKFLOWS_DIR` (override for tests/register).

## Engines (Q0.1 — before touching automation code)

- The repo is the **spine**; the engine is per client, in
  `clients/<client>/client.yaml` (`docs/client-spine.md`).
- `engine: windmill` — **default for new clients**. Source in
  `clients/<client>/automation/windmill/`; gate `./scripts/check-windmill.sh`.
- `engine: dagnats` — **legacy**: supported, still tested, no new client starts
  here (`new-client.sh` needs `--engine=dagnats` for it). Source in
  `clients/<client>/automation/dagnats/*.json`.
- One engine per client. Never migrate one unasked.

## Layout

- `cmd/worker/` — single worker binary (registers handlers, pulls NATS tasks) —
  **dagnats engine only**.
- `internal/handlers/` — 12 generic tasks (stable names recipes depend on).
- `internal/secret/` — env + age loader. `internal/opshttp/` — approval shim.
- `clients/<client>/automation/dagnats/*.json` — THE automation code. `clients/_template/` — skeletons.
- `clients/<client>/README.md` — per-client fieldbook (copy from `clients/_template/FIELDBOOK.md`).
- `env/<client>.env.example` — secret names (commit); `env/<client>.env` — values (never).
- `secrets/<client>/*.env.age` — ciphertext OK in git **in ops repos only**. `docs/` — expanded refs.
- Template keeps only `_template/ example-client/ ops/ personal/` (see `docs/ops-repo.md`).

## Rules

- Automation code = workflow JSON. New Go handler = last resort: only if
  unexpressible via recipe + generic tasks; needs typed I/O, env secrets,
  timeouts, table tests, human review. Never arbitrary script exec from JSON.
- Secrets by NAME in JSON (`"auth_secret": "ACME_X"`); values only in env/age.
- Tenancy: names `<client>.<wf>` (separator in `naming.json`), trigger ids `<client>-…`, webhook paths contain
  `<client>` (`ops/` reserved for platform). No shared names. Register per client.
- Alerts: every production recipe gets `on_failure` → notify; `ops/dlq-watchdog`
  covers silent DLQ buildup; external probe on `GET /health` covers total death.
  Never rely solely on the console (see `docs/alerting.md`).
- dagnats v0.0.30 traps: `on_failure` targets need `aux_steps` + no
  `depends_on`; `sleep`/`approval` steps take no `task`; `map` = 1 dep
  emitting an array; trigger runs arrive as `{…, "data": …}` — open recipes
  with `((.data // .) | …)`; cron schedules default to UTC (`cron_timezone`
  overrides). Durations accept integer ns (`30000000000`) **or** Go strings
  (`"30s"`), and `$schema` is accepted (optional). Allowed/excluded engine
  surfaces: `docs/writing-recipes.md#what-the-engine-allows-in-this-runtime`.
- Windmill: default engine, still no dependency of this repo. Never `wmill sync
  push` (deletes remote items); push one flow at a time after `wmill flow
  preview`. Keep logic in Go/TS scripts, not in YAML expressions
  (`skills/turvez/references/windmill.md`).
- Sandbox/where-to-run: recipes need no external sandbox (`validate` + `go test`);
  one-off scripts via `scripts/sandbox-run.sh`; recurrent script-shaped jobs via
  Dagu ad-hoc, Windmill by migration (see `docs/sandbox-policy.md`, `docs/script-execution.md`).

## Commands

```bash
# every checkout, both engines
make validate            # dagnats recipes, every client
go test ./...            # structure, tenancy, jq expressions, secrets guard
./scripts/check-windmill.sh   # windmill lint gate (no worker needed)

# dagnats engine (legacy)
dagnats workflow validate <file> && dagnats workflow register <file>
dagnats run start <client>.<wf> '<json>' --watch | --json
dagnats run status|events|output|inspect <run-id> [--last]
dagnats run list [--workflow=<client>.<wf>]   dagnats run cancel <run-id>
dagnats run signal <run-id> approval '{"approved":true}'
dagnats trigger create <wf> --cron="0 2 * * *" [--tz=UTC] | --webhook=/hooks/<client>/x | --subject="s"
dagnats trigger test "<expr>" --count=3   dagnats trigger list|enable|disable|delete <id>
dagnats dlq list --run=<id>   dagnats dlq replay --run=<id>   dagnats trace <run-id>
curl -X POST $OPS/ops/approvals/<run> -H "Authorization: Bearer $TOK" -d '{"approved":true}'
```

REST: `POST /workflows|/runs|/runs/{id}/cancel|/runs/{id}/signal/{n}|/runs/{id}/approval/{s}?action=approve|reject&token=…`.
First run + traps: `docs/first-run.md`. Flows and formats: `docs/workflows.md`. Client deployment model (embedded vs
managed vs multi-tenant, gateway placement): `docs/deployment-profiles.md`.
Full CLI/API reference: `docs/agent-tools.md`. After edits: `go test ./...`, `dagnats workflow validate`.

**CLI target:** the CLI is NATS-first — it reads `DAGNATS_NATS_URL`
(default `nats://127.0.0.1:4222`). `DAGNATS_API_URL` is only the worker's
approval shim; setting it does not move the CLI. Compose publishes
`${DAGNATS_HTTP_PORT:-8080}` / `${DAGNATS_NATS_PORT:-4222}`; if a port is
already forwarded by something else, the CLI silently reads a different
server (symptom: runs stay `queued`, `workers list` empty).

## Stack

- Go 1.27 (`go.mod`), dagnats v0.0.30 (pinned), NATS JetStream embedded, worker `distroless/static`.
- Skill: `skills/turvez/SKILL.md` (+ `references/` detail files) — direct → confirm → build.

## Observability

Every model call is an `llm.complete` step (GoAI is the handler's transport,
not a second entry point); the prompt is built in a prior step so it is
persisted; quality gates are `condition` steps over the output schema; the
run KV has no TTL. Full rules and the storage map: `docs/observability.md`.

## Commits and releases

- Conventional Commits: `feat:`, `fix:`, `docs:`, `chore:`. Breaking changes
  use `!` or a `BREAKING CHANGE:` footer; release-please derives the version.
- Merging the release PR tags and publishes the GitHub Release. Do not tag by
  hand after the `v0.1.0` bootstrap.
- Run `./scripts/install-hooks.sh` once per clone: pre-commit recipe scan and
  the commit-msg Conventional Commits check.
- Never move the version to 1.x: the project stays 0.x until a human explicitly
  asks for 1.0.0. `bump-minor-pre-major` keeps breaking changes in 0.x and
  `scripts/guard-version.sh` (CI) blocks a 1.x manifest.
- Ops repos pin `TURVEZ_REF` to a release tag (`v0.1.0`); SHA only for an
  unreleased commit under test.

## Don'ts

- Never execute arbitrary scripts from workflow JSON — recipes use only the 12 registered tasks.
- Never paste secret values into JSON, docs, chat, or prompts — names only (`go test ./clients/` guards this).
- Never edit definitions only in dagnats KV/console without committing to git first.
- Never invent handler `task` names or protocol fields — registry is `internal/handlers/handlers.go`.
- Never run `register`, `run start/cancel`, or `trigger disable/delete` unasked — propose the exact command and wait.
