# Agent tools

CLI/API surface an automation-authoring agent should use. The agent edits
**workflow JSON only**; Go handlers are platform code and require human
review (see `docs/writing-recipes.md` checklist). All commands support
`--json` for machine-readable output — prefer it.

## Workflows

```bash
dagnats workflow validate <file>        # offline check, no NATS needed
dagnats workflow register <file>        # registers def + embedded triggers
dagnats workflow list
dagnats workflow show <name>            # (if available) print stored def
```

Validate locally after every edit; register only when the operator asks.
Recipe pitfalls (dagnats v0.0.30, verified): durations accept **integer
nanoseconds** (`30000000000` = 30s) or Go duration strings (`"30s"`);
`map` steps need exactly one dep emitting a JSON array; `on_failure`
targets must also be listed in top-level `aux_steps`; `sleep`/`approval`
steps take **no** `task`; cron defaults to UTC (`--tz` / `cron_timezone`
to change).

## Runs

```bash
dagnats run start <workflow> '<json-input>' --watch   # start + tail events
dagnats run start <workflow> '<json-input>' --json    # capture run id
dagnats run status <run-id> [--json] [--last]
dagnats run events <run-id> [--full] [--type=T] [--step=S]
dagnats run output <run-id> [--last]                  # final output
dagnats run inspect <run-id>                          # status + errors + DLQ
dagnats run list [--workflow=PREFIX] [--status=running]
dagnats run cancel <run-id>
```

## Signals (approvals, resumes)

```bash
dagnats run signal <run-id> approval '{"approved":true}'
```

REST: `POST /runs/{id}/signal/{name}` with the JSON body.
Native approval steps instead use the tokenized endpoint
(`POST /runs/{id}/approval/{step}?action=approve|reject&token=…`) —
route humans through the ops shim, never hand out dagnats API directly:

```bash
curl -X POST $OPS_BASE/ops/approvals/<run-id> \
  -H "Authorization: Bearer $OPS_APPROVAL_TOKEN" \
  -d '{"approved":true}'
```

## Triggers

```bash
dagnats trigger create <workflow> --cron="0 2 * * *" --tz=UTC
dagnats trigger create <workflow> --webhook=/hooks/acme/event
dagnats trigger create <workflow> --subject="orders.created"
dagnats trigger test "<expr>" [--tz=TZ] [--count=N]  # validate cron, show fire times
dagnats trigger list | dagnats trigger enable|disable|delete <id>
```

Exactly one of `--cron`, `--subject`, `--webhook` per trigger. Cron is
5-field, minute precision. Trigger-fired runs receive a TriggerEnvelope
(`{trigger, source, workflow_id, timestamp, data}`) — recipes must read
payload under `.data` (use the `(.data // .)` pattern).

## DLQ and recovery

```bash
dagnats dlq list [--run=<id>] [--limit=N]
dagnats dlq replay <seq> | dagnats dlq replay --run=<id>
```

4xx/config failures are permanent by design (fix the recipe or secret,
then replay); 5xx/timeouts retry with backoff first.

## Observability

- Console: `http://127.0.0.1:8080/console/` (workflows, runs, triggers,
  DLQ, audit, metrics) when `dagnats serve` runs.
- `dagnats trace <run-id>` for OTLP-free trace inspection.
- `dagnats status` for engine health.

## Agent workflow

0. Read the client's fieldbook (`workflows/<client>/README.md`; template at
   `workflows/_templates/FIELDBOOK.md`; personal work uses client `personal/`)
   and decide where the work belongs FIRST (`docs/script-execution.md` Q1–Q4 —
   dagnats recipe vs sandbox one-off vs Dagu ad-hoc vs Windmill migration).
   If it is not a dagnats recipe, stop here and say which level + why.
1. Read the client's folder + the closest `_templates/` recipe.
2. Copy → rename with `<client>/` prefix → edit → `workflow validate`.
3. `go test ./workflows/` if expressions changed (add sample coverage).
4. Propose `register` + trigger commands; do not run them unasked.
   Same for `run start` / `run cancel` / `trigger disable`: side effects and
   secret-adjacent — always show the exact command and wait for approval.
5. Never invent handler task names (registry is in
   `internal/handlers/handlers.go`) or protocol fields; when the engine
   lacks a feature, say so and mark the workaround.
