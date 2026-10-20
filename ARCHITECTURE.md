# Architecture

```
                    ┌────────────────────────────────────────────┐
                    │ dagnats serve (one process)                │
                    │  embedded NATS + JetStream                 │
                    │  engine (DAG advance, retries, cron)       │
                    │  HTTP API + triggers + console (:8080)     │
                    │  KV: workflow_defs, runs, triggers,        │
                    │      signals, approval_tokens, DLQ stream  │
                    └──────▲───────────────────────────▲─────────┘
                           │ tasks (JetStream)         │ events/signals
                           ▼                         │
                    ┌─────────────────────────────────┴──────────┐
                    │ worker (this repo, single binary)          │
                    │  handlers: http.request, transform.json,   │
                    │  condition, map, delay, notify.* (webhook, │
                    │  slack, telegram, discord),                │
                    │  request.approval, llm.complete, dlq.check │
                    │  secrets resolved from env at task time    │
                    │  optional ops HTTP (:8090)                 │
                    └──────┬────────────────────────┬────────────┘
                           │ HTTPS                  │ approve/reject
                           ▼                        ▼
                 external SaaS ◄────────── human (Slack link/curl)
                 (HubSpot, Slack, billing…)
```

## Components

- **dagnats** (pinned `v0.0.30`, built from upstream — no published
  image yet) is the only infrastructure: JetStream streams (`TASK_QUEUES`,
  `WORKFLOW_HISTORY`, `DEAD_LETTERS`, …) plus KV buckets. No Postgres,
  no Redis, no per-automation callback servers — workers **pull**.
- **Worker** (`cmd/worker`) registers the eight generic handlers and
  blocks on tasks. It is stateless and horizontally repeatable; the only
  per-host state is which secret files it loads.
- **Recipes** (`workflows/<client>/*.json`) are the automation code:
  DAG + static config in step `metadata` + trigger declarations.
  Registered into dagnats, versioned in git.
- **Secrets** (`internal/secret`): env-first, optional age-encrypted
  dotenv second (`secrets/<client>/env.age`, decrypted to memory at boot).
  JSON carries names, never values. Full flow in `docs/secrets.md`
  (per-client keys, JIT runtime, sops evaluation, LLM hygiene).
- **Ops HTTP** (`internal/opshttp`, optional): Bearer-token shim turning
  a human click into a dagnats signal (`request.approval` flows) or a
  tokenized approval call (native `approval` steps).

## Data flow (refund example)

1. `run start example-client/approval-refund '{order…}'` → run input to
   `build-review` → shaped review object.
2. `approve` (native step, no worker): engine publishes
   `approval.requested` + token to `approvals.example-client.refund`,
   waits up to 48h. Human approves via ops URL; grant payload becomes
   the step output. Reject → `on_failure: notify-rejected`.
3. `build-refund` merges `{approve, build-review}` fan-in into a
   `http.request` payload; `refund` POSTs; `build-receipt` + `notify-done`
   close the loop to Slack.

## Decisions

- **Recipes over handlers.** Conditionals (`skip_if`/`condition`),
  loops (native `map`), extraction (`transform.json`) cover ~everything
  in client work. New Go only for stable domain logic or SDK needs —
  checklist in `docs/writing-recipes.md`.
- **Env + age, no vault.** A solo operator needs decrypt-at-rest and
  zero moving parts. Process env in prod (Docker/systemd), age files
  for versioned backups. Revisit only with a second operator or
  compliance pressure.
- **No per-client processes.** Client identity = folder + name prefix +
  env file. Physical split later = copy those three to a new host
  (`docs/isolating-a-client.md`).
- **Engine sleeps, not worker sleeps.** Multi-day waits are `sleep` /
  `approval` / signal-resume in the DAG (durable in KV); the `delay`
  handler is capped at 4h so a stuck run is loud.

## Verified upstream behaviors (v0.0.30)

Recorded here so upgrades re-check them: durations parse as **integer
nanoseconds or Go duration strings** (`"30s"`); `$schema` is accepted
(optional); `config` exists on map/sleep/approval steps though the JSON
schema file omits it; `aux_steps` must be set manually in JSON;
`sleep`/`approval` steps take no `task`; `map` needs exactly one
array-emitting dep; trigger runs get a TriggerEnvelope (`data` holds the
payload); `WaitForSignal` ≤1h per call; approval grant payload = step
output, reject = failure path; cron schedules default to UTC
(`cron_timezone` / `cron_timezone_input_path` override it);
`output_schema`, when set, is enforced against the run output.
