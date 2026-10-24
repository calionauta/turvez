# AGENTS.md — consultancy automation runtime (Go + dagnats v0.0.30)

## Layout

- `cmd/worker/` — single worker binary (registers handlers, pulls NATS tasks).
- `internal/handlers/` — 12 generic tasks (stable names recipes depend on).
- `internal/secret/` — env + age loader. `internal/opshttp/` — approval shim.
- `workflows/<client>/*.json` — THE automation code. `workflows/_templates/` — skeletons.
- `workflows/<client>/README.md` — per-client fieldbook (copy from `workflows/_templates/FIELDBOOK.md`).
- `env/<client>.env.example` — secret names (commit); `env/<client>.env` — values (never).
- `secrets/<client>/*.env.age` — ciphertext OK in git. `docs/` — expanded refs.

## Rules

- Automation code = workflow JSON. New Go handler = last resort: only if
  unexpressible via recipe + generic tasks; needs typed I/O, env secrets,
  timeouts, table tests, human review. Never arbitrary script exec from JSON.
- Secrets by NAME in JSON (`"auth_secret": "ACME_X"`); values only in env/age.
- Tenancy: names `<client>/…`, trigger ids `<client>-…`, webhook paths contain
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
- Windmill: no dependency; keep recipes in portable step types; flag any new
  Go handler as "Windmill migration cost" (see `docs/migrate-to-windmill.md`).
- Sandbox/where-to-run: recipes need no external sandbox (`validate` + `go test`);
  one-off scripts via `scripts/sandbox-run.sh`; recurrent script-shaped jobs via
  Dagu ad-hoc, Windmill by migration (see `docs/sandbox-policy.md`, `docs/script-execution.md`).

## Commands

```bash
dagnats workflow validate <file> && dagnats workflow register <file>
dagnats run start <client>/<wf> '<json>' --watch | --json
dagnats run status|events|output|inspect <run-id> [--last]
dagnats run list --workflow=<client>/   dagnats run cancel <run-id>
dagnats run signal <run-id> approval '{"approved":true}'
dagnats trigger create <wf> --cron="0 2 * * *" [--tz=UTC] | --webhook=/hooks/<client>/x | --subject="s"
dagnats trigger test "<expr>" --count=3   dagnats trigger list|enable|disable|delete <id>
dagnats dlq list --run=<id>   dagnats dlq replay --run=<id>   dagnats trace <run-id>
curl -X POST $OPS/ops/approvals/<run> -H "Authorization: Bearer $TOK" -d '{"approved":true}'
```

REST: `POST /workflows|/runs|/runs/{id}/cancel|/runs/{id}/signal/{n}|/runs/{id}/approval/{s}?action=approve|reject&token=…`.
Full reference: `docs/agent-tools.md`. After edits: `go test ./...`, `dagnats workflow validate`.

## Stack

- Go 1.27 (`go.mod`), dagnats v0.0.24 (pinned), NATS JetStream embedded, worker `distroless/static`.
- Skill: `skills/turvez/SKILL.md` (+ `references/` detail files) — direct → confirm → build.

## Don'ts

- Never execute arbitrary scripts from workflow JSON — recipes use only the 12 registered tasks.
- Never paste secret values into JSON, docs, chat, or prompts — names only (`go test ./workflows/` guards this).
- Never edit definitions only in dagnats KV/console without committing to git first.
- Never invent handler `task` names or protocol fields — registry is `internal/handlers/handlers.go`.
- Never run `register`, `run start/cancel`, or `trigger disable/delete` unasked — propose the exact command and wait.
