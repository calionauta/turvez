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
- `notify.slack` — `{"webhook_secret":…, "message":…}` or a bare string
  (secret name from metadata).
- `request.approval` — envelope, `{}` or empty (metadata/defaults apply).

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
(default `LLM_API_KEY`).

```json
{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "…"}],
 "api_key_secret": "LLM_API_KEY", "base_url": "https://api.openai.com/v1",
 "max_tokens": 1024, "temperature": 0.2, "timeout_ms": 120000}
```

Output: `{"content": "…", "usage": {...}, "model": "…"}`.
Metadata keys: `model, api_key_secret, base_url`.
