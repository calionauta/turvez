# AGENTS.md — consultancy automation runtime (Go + dagnats v0.0.24)

## Layout

- `cmd/worker/` — single worker binary (registers handlers, pulls NATS tasks).
- `internal/handlers/` — 12 generic tasks (stable names recipes depend on).
- `internal/secret/` — env + age loader. `internal/opshttp/` — approval shim.
- `workflows/<client>/*.json` — THE automation code. `workflows/_templates/` — skeletons.
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
- dagnats v0.0.24 traps: durations are INTEGER ns (`30000000000` = 30s);
  NO `$schema` key (CLI rejects it); `on_failure` targets need `aux_steps`
  + no `depends_on`; `sleep`/`approval` steps take no `task`; `map` = 1 dep
  emitting an array; trigger runs arrive as `{…, "data": …}` — open recipes
  with `((.data // .) | …)`.
- **dagnats v0.0.24 console cannot create the FIRST trigger.** The console
  calls `ListTriggers` before writing, to check for a duplicate id, and on an
  empty `triggers` bucket that returns `jetstream.ErrNoKeysFound` raw from
  `listTriggersInner` (`internal/api/service_triggers.go`). The console turns
  it into `500 lookup failed: nats: no keys found` and never writes — so the
  bucket can never become non-empty through the UI. **This project's normal
  path is not affected:** every trigger is declared inline in a recipe and
  applied with `dagnats workflow register`, which writes through
  `CreateTrigger` or straight to the KV and never calls the raising function.
  The CLI (`dagnats trigger create`) is also unaffected.
  Workaround: `internal/bootstrap/triggers.go` seeds one disabled placeholder
  trigger when the bucket is empty, so the console is usable on a fresh
  engine. Set `DAGNATS_TRIGGER_BOOTSTRAP=false` to disable. Upstream fix:
  [danmestas/dagnats#745](https://github.com/danmestas/dagnats/pull/745).
  **Re-check on every dagnats bump** — when the fix ships, delete
  `internal/bootstrap/triggers.go` and its call in `cmd/worker/main.go` in the
  SAME commit as the bump, and remove the paragraph above.
- Windmill: no dependency; keep recipes in portable step types; flag any new
  Go handler as "Windmill migration cost" (see `docs/migrate-to-windmill.md`).

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
