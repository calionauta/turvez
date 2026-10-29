# Writing recipes

A recipe is a **workflow JSON file**: the automation code. Go handlers are
shared platform code — you touch them only when a recipe cannot express
the logic (see the checklist at the bottom).

## Change flow: git is the source of truth

Every recipe change is a commit in `workflows/<client-id>/`. Never edit a
definition only inside dagnats KV (console/API) without updating git —
KV is a deployment target, git is the source of truth. The extract and
disaster-recovery paths re-register from git; anything that lives only
in KV is lost on cutover.

## Anatomy of a run

1. A run starts with an **input**. Manual starts pass it directly:
   `dagnats run start example-client.hubspot-lead-to-slack '{"lead_id":"123"}'`.
   Trigger-fired runs (webhook, cron, NATS subject) receive a
   **TriggerEnvelope**: `{"trigger":"…","source":"…","workflow_id":"…",
   "timestamp":"…","data":{…}}` — your payload lives under `data`.
2. Entry steps (no `depends_on`) receive the run input.
3. A step with **one** dependency receives that parent's output unchanged.
4. A step with **several** dependencies receives an object keyed by parent
   step id: `{"build-review": {...}, "approve": {...}}`.
5. Step outputs flow downstream; the last steps' outputs are the run output.

**Envelope-tolerant first step.** Because the same recipe may start manually
or from a trigger, open webhook/cron recipes with:

```jq
((.data // .) | {lead_id: .lead_id}) as $r | { … use $r.lead_id … }
```

(`.data // .` = trigger envelope or raw manual input.) All example recipes
do this; `workflows/recipes_test.go` executes both shapes.

## Idempotency: what re-runs, and what must not duplicate

Delivery is **at-least-once**. A step that finished is never re-executed
(its `step.completed` event is in the log), but a step that was **in
flight** when the worker died is redelivered after `AckWait`. The engine
cannot tell whether your side effect landed, so it runs the step again.

| Surface | Duplicate is… | Rule |
|---|---|---|
| GET / read | harmless | nothing to do |
| PUT/upsert with a natural key | harmless | prefer over POST |
| POST that creates a resource | dangerous | send an idempotency key |
| Charge / payment | critical | provider idempotency key, always |
| Slack / email notification | noisy but acceptable | document it; include `run_id` |
| Ledger append | dangerous | unique constraint at the destination |

### Layer 1 — one run per event (`idempotency_key`)

```json
{
  "name": "acme/invoice-paid",
  "idempotency_key": "data.event_id",
  "steps": [ … ]
}
```

Dot-path into the **raw** input; the engine hashes `{workflow}.{value}`
(SHA-256) into the `idempotency_keys` KV bucket (24h TTL) and returns the
existing run instead of starting a second one.

- Trigger envelope → `data.<field>`; manual start → `<field>`. One path does
  not cover both shapes.
- A missing path is a **soft failure**: the run starts without dedup and
  logs a warning. `go test ./workflows/` cannot catch it — check the field
  exists in every producer.
- It is check-then-start, not a lock: two concurrent deliveries can both
  pass. Treat it as duplicate *reduction*, not a guarantee.

### Layer 2 — HTTP triggers (`idempotency_header`)

Durable Endpoints (`/api/...`) accept an `idempotency_header` (e.g.
`Idempotency-Key`), backed by the `http_idempotency` KV (1h TTL) which also
stores the response payload. A sender retry gets the same response instead
of a new run. Fire-and-forget webhooks (`/hooks/...`) do not have this —
use layer 1 there.

### Layer 3 — the effect (`Idempotency-Key` at the destination)

This is the only layer that gives a real guarantee. Build the request in a
`transform.json` step and carry a deterministic key:

```json
{
  "id": "build-charge",
  "task": "transform.json",
  "metadata": {
    "expression": "{method: \"POST\", url: \"https://api.stripe.com/v1/charges\", headers: {\"Idempotency-Key\": .payment_id}, body: {amount: .amount}}"
  }
}
```

- Prefer PUT/upsert + natural key (`lead_id`, `invoice_id`) over blind POST.
- Use the provider's idempotency key where it exists (Stripe, Shopify, many
  CRMs).
- Slack has no idempotency key. Duplicates are inherent — include `run_id`
  in the message so an operator can tell them apart, and record the
  tolerance in the client fieldbook.
- `singleton` prevents overlapping runs; it does **not** deduplicate.

**Exactly-once does not exist end to end.** What exists is at-least-once
delivery plus a deterministic key plus a destination that enforces
uniqueness — the same outbox/unique-key pattern every durable engine ends
up with.

**Review rule:** any recipe whose final write is a non-idempotent POST must
carry a key from the source event. If it cannot, the recipe needs a
destination-side unique constraint or it does not ship.

## Static config in metadata, dynamic data in input

Handlers read **static recipe config from step `metadata`** (strings) and
**dynamic data from task input** (parent outputs). Input wins on conflict.

```json
{
  "id": "build-fetch",
  "task": "transform.json",
  "timeout": 30000000000,
  "metadata": { "expression": "{method: \"GET\", url: …}" }
}
```

The handler reference (`docs/handlers-reference.md`) lists the metadata
keys per task. Secret *names* go in metadata or input
(`"auth_secret": "ACME_HUBSPOT_TOKEN"`); values never appear in JSON.

**Every metadata value is a string.** `metadata` is `map[string]string`:
numbers become `"timeout_ms": "15000"` and objects become JSON-encoded
strings, e.g. `"headers": "{\"Accept\":\"application/json\"}"`. A real
object or number fails validation (`cannot unmarshal object into ...`).

**Persist what you call.** Build the LLM prompt in a `transform.json` step
and pass `messages` downstream (see `docs/observability.md`): the prompt
becomes a step output, so a failed call is still replayable and the run
records prompt + response.

## Engine primitives — prefer these over worker handlers

| Need | Engine feature | Worker fallback |
|---|---|---|
| Fan-out over a list | `type: map` step, `config: {"max_items": N}`. Exactly **one** `depends_on`; its output must be a **JSON array**; the step's `task` handler runs **per item**; downstream receives an **array** of outputs | `map` task (in-worker reshape, ≤10k items) |
| Wait | `type: sleep` step, `config` = exactly one of `{"duration": NS}`, `{"cron": "…"}`, `{"until_input_path": "…"}`, max 365d. Durable, holds no worker slot | `delay` task (≤4h, holds a worker slot) |
| Branch | `skip_if: {"step_id": "…", "field": "…", "op": "…", "value": …}` — `step_id` must be in `depends_on`; `field` is a **top-level** key of that parent's output; ops `==,!=,<,>,<=,>=` (numbers as float64) | `condition` task (nested paths, jq, branch names) |
| Human gate, days | `type: approval` step (no `task`), `config: {"timeout": NS, "subject": "…", "description": "…"}`. Timeout ≤168h. Decision via `POST /runs/{id}/approval/{step}?action=approve\|reject&token=…`; grant payload becomes the step output; reject takes the `on_failure` path | `request.approval` task + run signal (custom notify, chunked waits) |
| Failure path | `on_failure: "other-step"` + **`"aux_steps": {"other-step": true}`** at top level (see trap below). The target must be a **single terminal step** (no `depends_on`, no downstream chain — aux steps never resolve normally) and receives `{"failed_step":…, "error":…}`. Point it at a notify step (see `docs/alerting.md`) | — |

## What the engine allows in this runtime

dagnats v0.0.30 is larger than what this repo exposes. Stay inside the
left column; the right column exists upstream but has no handler or
recipe pattern here — do not assume it.

| Allowed here | Upstream-only (do not use in recipes) |
|---|---|
| Linear durable steps (`depends_on`, output persisted) | Bounded agent loops (`StepTypeAgentLoop` + `MaxIterations`/`MaxDuration`) |
| Branch via `skip_if` (top-level parent key, `==,!=,<,>,<=,>=`, numbers as float64) | Runtime-planned sub-DAGs (planner fragments, `MaxSteps`/`MaxDepth`/`AllowedTasks`) |
| Fan-out via native `type: map` (one array-emitting dep) | Agent-spawned runs (`ControlPlane` + audit KV) |
| Durable waits via `type: sleep` (max 365d, no worker slot) | Subworkflows (child runs from KV defs) |
| Human gates via native `type: approval` (timeout ≤168h) or `request.approval` task (≤55m `WaitForSignal` chunks, engine ceiling 1h) | `waitforevent` gates with match/dotpaths |
| Failure path via `on_failure` + `aux_steps` (single terminal step) | Compensation (`compensate`) targets |
| Per-step retry/backoff (`timeout`, `initial_delay`/`max_delay`) + cron/webhook/NATS triggers | Free-form expressions (engine has none — comparisons + dotpaths only) |
| Input/output contracts via `input_schema` / `output_schema` (JSON Schema; input checked at run start, output at completion) | — |

Bounds enforced in this repo: in-worker `map` ≤10 000 items
(`internal/handlers/map_items.go`); `delay` ≤4h (`internal/handlers/delay.go`);
signal chunks ≤55m against the 1h engine ceiling
(`internal/handlers/request_approval.go`). Upstream position is "bounded
dynamism": every dynamic surface carries a numeric bound + validation.

### Why AgentLoop/planner stay upstream-only here

`agent_loop` is real and well built upstream (ADR-002): each iteration is a
`Continue()` with a conversation checkpoint, so a crash resumes at the last
turn instead of the first. It is not enabled in this runtime on purpose:

1. **No handler can drive it.** `llm.complete` calls `Complete()`. An agent
   loop needs a handler that calls `ctx.Continue()` and owns
   `Checkpoint()`/`LoadCheckpoint()` — and that handler is the tool runner.
2. **Tools are arbitrary code.** The loop's value is letting the model call
   shell/files/HTTP. That is exactly the surface `AGENTS.md` forbids
   ("never execute arbitrary scripts from workflow JSON").
3. **It does not pass our gate.** `go test ./workflows/` runs jq expressions
   against samples; a model loop is non-deterministic and has unbounded
   cost per run. A recipe you cannot validate before registering is not a
   recipe.
4. **It does not port to Windmill.** It becomes a script step in
   `docs/migrate-to-windmill.md` — migration cost with no recipe gain.

**If a client genuinely needs an agent loop**, that is a new platform
decision (`docs/script-execution.md` Q1.5) — Dagu 2.x `llm.*`/`harness.run`/
agent DAGs, or a dedicated harness. `llm.complete` stays: one bounded call
whose output is **data**, validated by `transform.json`/`condition`.

## dagnats v0.0.30 traps (verified against upstream source)

1. **Durations accept two forms.** Integer nanoseconds (`30000000000`)
   or a Go duration string (`"30s"`, `"48h"`); the engine rewrites the
   string to ns before decoding. Applies to `timeout`, retry
   `initial_delay`/`max_delay`, and `sleep`/`approval` config timeouts.
   The recipes here use integers; neither form is wrong.
2. **`config` is real but undocumented.** `map`/`sleep`/`approval` steps
   take a `config` object (see table). Verified in `dag/config.go`,
   `dag/approval.go`.
3. **`aux_steps` must be set by hand, and failure targets take no
   `depends_on`.** Steps referenced by `on_failure` (or `compensate`)
   are excluded from normal scheduling only when listed in top-level
   `"aux_steps"` — JSON registration does not derive this (only the Go
   builder does). Without it, your failure step also runs on the happy
   path. And validation rejects `depends_on` on such targets outright.
   `approval-refund.json` shows the pattern.
4. **`sleep`/`approval` steps take no `task`.** They are engine-executed;
   omit `task` (validation enforces this split).
5. **Cron is 5-field, minute precision, and UTC by default.** Trigger
   cron and `sleep` cron both default to UTC; `cron_timezone` (static)
   or `cron_timezone_input_path` (read from input) picks another zone,
   and at most one of the two may be set.
6. **Approval tokens** arrive in the step's `subject` (NATS) as an
   `approval.requested` event; the ops shim (`internal/opshttp`) turns a
   human click into the tokenized API call.
7. **`$schema` is accepted (optional).** It was rejected through
   v0.0.24; since v0.0.26 the CLI carries it as a known top-level key.
   These recipes omit it — adding it only buys editor autocomplete.

## Custom Go handler checklist (add one only if ALL apply)

- [ ] The logic cannot be expressed as recipe + `transform.json`/`condition`/`map`.
- [ ] It is stable business logic (pricing, tax matching, multi-API with
      state) or needs an official SDK — not a one-off script.
- [ ] It reads secrets by name from env, takes typed JSON input, returns
      structured output, uses context timeouts.
- [ ] You added it under `internal/handlers/`, registered it in
      `RegisterAll`, and wrote table-driven tests.
- [ ] The recipe change creating it passed human review — LLM agents may
      propose handlers but never land new ones unreviewed.

Never enable arbitrary script execution from workflow JSON.
