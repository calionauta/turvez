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
   `dagnats run start example-client/hubspot-lead-to-slack '{"lead_id":"123"}'`.
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

## Engine primitives — prefer these over worker handlers

| Need | Engine feature | Worker fallback |
|---|---|---|
| Fan-out over a list | `type: map` step, `config: {"max_items": N}`. Exactly **one** `depends_on`; its output must be a **JSON array**; the step's `task` handler runs **per item**; downstream receives an **array** of outputs | `map` task (in-worker reshape, ≤10k items) |
| Wait | `type: sleep` step, `config` = exactly one of `{"duration": NS}`, `{"cron": "…"}`, `{"until_input_path": "…"}`, max 365d. Durable, holds no worker slot | `delay` task (≤4h, holds a worker slot) |
| Branch | `skip_if: {"step_id": "…", "field": "…", "op": "…", "value": …}` — `step_id` must be in `depends_on`; `field` is a **top-level** key of that parent's output; ops `==,!=,<,>,<=,>=` (numbers as float64) | `condition` task (nested paths, jq, branch names) |
| Human gate, days | `type: approval` step (no `task`), `config: {"timeout": NS, "subject": "…", "description": "…"}`. Timeout ≤168h. Decision via `POST /runs/{id}/approval/{step}?action=approve\|reject&token=…`; grant payload becomes the step output; reject takes the `on_failure` path | `request.approval` task + run signal (custom notify, chunked waits) |
| Failure path | `on_failure: "other-step"` + **`"aux_steps": {"other-step": true}`** at top level (see trap below). The target must be a **single terminal step** (no `depends_on`, no downstream chain — aux steps never resolve normally) and receives `{"failed_step":…, "error":…}`. Point it at a notify step (see `docs/alerting.md`) | — |

## What the engine allows in this runtime

dagnats v0.0.24 is larger than what this repo exposes. Stay inside the
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

Bounds enforced in this repo: in-worker `map` ≤10 000 items
(`internal/handlers/map_items.go`); `delay` ≤4h (`internal/handlers/delay.go`);
signal chunks ≤55m against the 1h engine ceiling
(`internal/handlers/request_approval.go`). Upstream position is "bounded
dynamism": every dynamic surface carries a numeric bound + validation.

## dagnats v0.0.24 traps (verified against upstream source)

1. **Durations are integer nanoseconds.** The docs mention Go strings
   (`"5m"`), but `workflow register` fails to parse them — use numbers:
   `30s = 30000000000`, `1m = 60000000000`, `1h = 3600000000000`,
   `48h = 172800000000000`. This applies to `timeout`, retry
   `initial_delay`/`max_delay`, and `sleep`/`approval` config timeouts.
   An editor schema (`docs/workflow-schema.json` upstream) still shows the
   string form — it is autocomplete only; the CLI parses Go structs.
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
4. **No `$schema` key (for now).** Upstream docs recommend a `$schema`
   pointer and the JSON schema file allows it, but the CLI's strict
   top-level check rejects it — upstream's own `examples/*` fail
   `workflow validate` for this reason at v0.0.24. This repo omits it
   (editor autocomplete is the loss); `recipes_test.go` guards the
   absence so the trap stays visible. Re-check on upgrade.
4. **`sleep`/`approval` steps take no `task`.** They are engine-executed;
   omit `task` (validation enforces this split).
5. **Cron is 5-field, minute precision.** Sub-minute schedules are rejected.
6. **Approval tokens** arrive in the step's `subject` (NATS) as an
   `approval.requested` event; the ops shim (`internal/opshttp`) turns a
   human click into the tokenized API call.

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
