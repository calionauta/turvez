# Handlers reference

Generic task handlers registered by `cmd/worker` (see `RegisterAll` in
`internal/handlers/handlers.go`). Task names are stable — recipes depend
on them. All handlers:

- parse typed JSON input (envelope or raw — see below),
- resolve secrets **by name** via `internal/secret` at runtime,
- return structured output via `ctx.Complete`,
- use context timeouts; 5xx/429/timeouts are retryable, 4xx/config errors
  are permanent (`FailPermanent`, no DLQ churn).

## Input convention: envelope or raw

Most handlers accept the full envelope object **or** raw data:

- `transform.json` — `{"input":…, "expression":…}` or any raw JSON value
  (expression then comes from metadata).
- `condition` — envelope or raw data (rule then comes from metadata).
- `map` — `{"items":…, "expression":…}` or a bare JSON array.
- `delay` — `{"duration":"…"}` or a bare JSON string.
- `notify.*` — envelope, bare string (message), or empty input with
  metadata only (the `on_failure`-safe shape: failure steps always get
  `{"failed_step":…, "error":…}`, which becomes the webhook payload).
- `request.approval` — envelope, `{}` or empty (metadata/defaults apply).
- `dlq.check` — `{"stream":…, "threshold":…}`, `{}` or empty (defaults apply).

Static metadata keys are listed per handler. Task input wins on conflict.

## http.request

Call any HTTP API. Auth header value comes from a secret name.

```json
{"method": "GET", "url": "https://…", "headers": {"Accept": "application/json"},
 "body": {"k": "v"}, "timeout_ms": 15000,
 "auth_secret": "ACME_HUBSPOT_TOKEN", "auth_scheme": "Bearer",
 "response_json": true}
```

- `auth_header` defaults to `Authorization`; `auth_scheme` defaults to
  `Bearer` for that header, raw value otherwise.
- Output: `{"status":…, "headers":…, "body": "…", "json": …}` (`json`
  only with `response_json`).
- Metadata keys: `method, url, auth_secret, auth_header, auth_scheme,
  response_json ("true"), timeout_ms`.

## transform.json

jq reshape/extract (gojq). `expression` required (input or metadata).
Output is the **raw jq value** (not wrapped), so the next step receives
it directly. Caveat: a data payload containing top-level `input` or
`expression` keys is read as the envelope — avoid those key names in
shaped data, or rename them in the expression.

```json
{"webhook_secret": "ACME_SLACK_WEBHOOK_URL",
 "message": ("New lead: " + (.json.properties.firstname // "?"))}
```

Single output unwraps; multiple outputs become an array. Metadata:
`expression`.

## condition

Boolean rule for complex branching. Prefer engine `skip_if` for a single
field comparison (no task execution spent); use this for nested paths,
jq expressions, or explicit branch names.

Field form: `{"input":…, "field": "json.total", "op": ">", "value": 10000,
"on_true": "vip", "on_false": "std"}`. Or `{"input":…, "expression":
".json.total > 10000"}`. Ops mirror `skip_if`: `==,!=,<,>,<=,>=`
(numbers float64; strings orderable; bools equality-only).
Output: `{"matched":…, "branch": "vip"|""}`.
Metadata keys: `field, op, value (JSON or raw string), expression,
on_true, on_false`.

## map

In-worker fan-out fallback. **Prefer engine `type: map`** (parallel
across workers, durable). Use this task for small reshapes (≤10 000
items, concurrency cap 32, default 8): applies one jq expression per
item, order-preserving. `{"items":[…], "expression": ". * 2",
"concurrency": 4}` → `{"results":[…], "count":…}`.
Metadata keys: `expression, concurrency`.

## delay

Worker-side wait. **Prefer engine `type: sleep`** (durable to 365d, no
worker slot). This handler caps at **4h** and holds a worker slot —
for minutes, not days. `{"duration": "90s"}` (Go duration string) →
`{"waited_ms":…, "duration":…}`. Metadata key: `duration`.

## notify.slack

POST `{"text":…}` to an incoming-webhook URL held in a secret.
`{"webhook_secret": "ACME_SLACK_WEBHOOK_URL", "message": "…"}` →
`{"ok": true}`. Metadata keys: `webhook_secret, message`.

## notify.webhook

POST any JSON payload to a URL held in a secret — the generic operator
path. All chat wrappers funnel through it.

`{"webhook_secret": "OPS_SLACK_WEBHOOK_URL", "payload": {"text": "…"}}`
→ `{"ok": true, "status": 200, "run_id": "…"}`. Shorthand:
`{"webhook_secret": "…", "message": "…"}` wraps as `{"text": …}`.
Raw data (or the `on_failure` input `{"failed_step":…, "error":…}`)
becomes the payload; `client` / `workflow` merge from metadata;
`run_id` / `step_id` stamp automatically. Empty input with metadata
only is valid (the `on_failure`-safe shape).
Metadata keys: `webhook_secret, message, client, workflow`.

## notify.telegram / notify.discord

Thin wrappers over the same HTTP path.
`notify.telegram`: `webhook_secret` holds the full Bot API sendMessage
URL (`https://api.telegram.org/bot<token>/sendMessage`), `chat_id` in
input or metadata. `notify.discord`: `webhook_secret` holds the Discord
webhook URL, posts `{"content": …}`.
Metadata keys: `webhook_secret, message` (+ `chat_id` for telegram).

## dlq.check

Read the dead-letter stream depth for the ops watchdog. There is no
dagnats HTTP route for the DLQ (CLI/NATS only), so this handler dials
NATS directly (short-lived connection, `DAGNATS_NATS_URL`) and returns
`StreamInfo.State.Msgs`.

`{"stream": "DEAD_LETTERS", "threshold": 0}` (both optional; metadata
keys `stream, threshold`) → `{"stream":…, "depth":…, "threshold":…,
"alert": depth>threshold}`. Backend errors are retryable; pair with a
`skip_if` on `alert` (see `clients/ops/automation/dagnats/dlq-watchdog.json`).
## request.approval

Signal-based human gate with optional pre-notify. Blocks in ≤55m chunks
(engine `WaitForSignal` ceiling is 1h) until the total `timeout`
(default 72h). The vote arrives as a run signal
`{"approved": bool}` (or `{"rejected": bool}`).

```json
{"signal": "approval", "timeout": "72h",
 "notify": {"webhook_secret": "ACME_SLACK_WEBHOOK_URL", "message": "Please approve"}}
```

Output: `{"approved":…, "data": {…raw vote…}}`. Pair with
`POST /ops/approvals/{runID} {"approved":true}` or
`dagnats run signal <runID> approval '{"approved":true}'`.
For tokenized multi-day gates with approve/reject links, prefer engine
`type: approval` instead. Metadata keys: `signal, timeout, message`.

## llm.complete

POST to an OpenAI-compatible `/chat/completions`. Key from secret name
(default `LLM_API_KEY`). Transport is GoAI (`provider/openai`), pinned to
the chat-completions dialect — GoAI v0.10 defaults to `/v1/responses`,
which most gateways do not implement. Internal retries are disabled: the
engine owns retry/backoff. `4xx` (except 429) and context overflow fail
non-retryably; `429`/`5xx` and transport errors stay retryable.

`base_url` is **required** — there is no silent fallback to
`api.openai.com`. Resolution order: task input → step metadata `base_url`
→ env `LLM_BASE_URL`; if none is set the task fails non-retryably. A
direct provider call bypasses the client's gateway, and with it the
cost/latency/error log: point every deployment at a gateway (Bifrost or
equivalent) and let the recipe name only the model.

```json
{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "…"}],
 "api_key_secret": "LLM_API_KEY", "base_url": "http://bifrost:8081/v1",
 "max_tokens": 1024, "temperature": 0.2, "timeout_ms": 120000}
```

Output: `{"content": "…", "usage": {...}, "model": "…"}`.
Metadata keys: `model, api_key_secret, base_url`.

**Reasoning models:** `max_tokens` covers reasoning *and* the answer. Too
small a budget returns `content: ""` with a successful status (measured
with `opencode-go/iceman-contributor-free`: 100 tokens → empty, 400 →
complete JSON). Give classifier/reasoning models headroom, and assert on
the output schema rather than trusting "status = success".

**Observability:** the run snapshot persists step *outputs*. Build the
prompt in a `transform.json` step (as `messages`) instead of inlining it
in the metadata — the prompt then becomes that step's output, so a
failed call is still replayable and every run records prompt + response.
