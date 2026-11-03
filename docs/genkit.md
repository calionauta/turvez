# Genkit: an agent-loop capability, not an engine

`engines.json` names the durable engines a client runs on (windmill,
dagnats, dagu). Genkit is **not** one of them and must never be added
there. It is a **capability**: a Go library that runs a bounded multi-step
loop **inside one request**, in a service we own.

The invariant does not move: **one engine per client**. Genkit sits in
front of that engine, never beside it as a second orchestrator.

```
request ──> Genkit flow (bounded loop, in-process)
              │  model calls ──> goai client (the only LLM path)
              │  tools       ──> Nango proxy (never a raw token)
              └─ durable tail ──> dagnats run / Windmill flow (the client's engine)
```

## What it buys (read from `calionauta/gogogo`, genkit v1.13.1)

- **Typed flows.** Input/output are Go structs; Genkit derives the schema.
  A flow is a function, not a script node.
- **Streaming and per-step progress.** `genkit.DefineStreamingFlow` emits
  typed chunks; gogogo forwards them to SSE so the human watches the
  writer/critic exchange instead of a spinner.
- **Dev UI and evals per phase.** `genkit.Init` plus a registered flow gives a
  local trace UI and an eval harness — the reason to prefer it over a
  hand-rolled `for` loop.
- **The model action is the seam.** gogogo registers one model action whose
  body calls its existing `llm.Client`. **No provider plugins**: a plugin SDK
  would bypass the retry/metering choke point.

Genkit orchestrates, **goai generates**. In Turvez `llm.complete` is already
a goai call, so that same client becomes the model action and both surfaces
share one retry layer and one meter.

## Three contracts (non-negotiable)

1. **One LLM path.** The Genkit model action calls our goai client. Never add
   a Genkit retry middleware or a Genkit billing middleware: retry lives in
   goai, metering in the client (`docs/observability.md`). Two retry layers
   means double reserve in the ledger, which is what gogogo's comment warns
   about at the call site.
2. **Observability does not become a hole.** A recipe persists the prompt as
   a step output *before* the call. A Genkit flow must do the same: write the
   composed prompt (client, workflow, run id, model) before generating and the
   completion after, in the same store the recipes use. A flow whose prompts
   exist only in process memory is not allowed in a client.
3. **Credentials never enter the flow.** Tools that touch a SaaS API call the
   Nango proxy with `Provider-Config-Key` + `Connection-Id`; the flow holds
   names, never tokens (`docs/nango.md`). The tool allowlist is the blast
   radius: an agent may call what we listed, nothing else.

## Where the line runs: flow vs recipe

| The work is… | Home |
|---|---|
| A loop whose value is a human watching it finish in seconds | Genkit flow in a service (`cmd/<client>-agent` or a feature package) |
| One model call whose output is data for the next step | `llm.complete` + `transform.json`/`condition` in a recipe |
| A durable consequence of the loop (save the record, notify, invoice) | Emit a **run** to the client's engine: dagnats `run start` / Windmill flow, with an idempotency key taken from the request |
| A retry that must survive a restart, or a wait of hours/days | Recipe (retry/backoff, `sleep`, native `approval`). Never a Genkit flow |
| An unattended loop that runs on a schedule | Not a recipe today: `StepTypeAgentLoop` is upstream-only here (`docs/writing-recipes.md`). That is a platform decision (`docs/script-execution.md`), not a Genkit default |

The rule in one sentence: **the loop is the product, so it may live in the
request; the record of what the loop decided is durable, so it becomes a run
in the engine the client already has.**

## When Genkit is the best proposal

1. **Review / rewrite loops** — writer → critic → judge with a human reading
   the exchange (gogogo's `/review`: 3 rounds, 90 s bound, `needs-human` on a
   no-op). The value is the visible argument, not a stored artifact.
2. **Triage assistant with read-only tools** — "which of these tickets is
   about billing?" where the model picks 2–4 calls from a short Nango-backed
   toolset and a human approves the output.
3. **Structured extraction with a critic** — parse a PDF or an e-mail into a
   typed record; a second pass checks the record against the source; below a
   confidence threshold the run escalates instead of writing.
4. **Bounded RAG answers** — route-then-read (`docs/retrieval.md`) plus a
   refusal path, streamed to a person who is waiting; the eval set gates it.
5. **Prompt-chain development** — you need the Dev UI/eval harness to iterate
   on roles and prompts before the shape is worth making durable.

## When it is the wrong tool

- **Nightly sync, webhook intake, per-lead pipeline** — no human in the loop,
  must survive restarts: recipe.
- **Approvals that take days** — native dagnats `approval` (timeout ≤168 h).
  A request-scoped flow dies with the connection.
- **High-volume batch** — a recipe with `map`, or a queue; a per-request loop
  costs a model call per item per round with no replay.
- **A single prompt** — `llm.complete` is the whole job.
- **A loop that writes to production systems without a gate** — the failure is
  unbounded and unattributable; put the write behind a recipe step with
  `on_failure` and an approval.

## Shape of a client integration

- **One Genkit instance per process**, created at boot (`genkit.Init`), all
  flows defined at startup, shared by handlers. `Init` registers a
  `signal.NotifyContext` it never stops — one per process is acceptable,
  documented where it is created.
- **Bounds are explicit**, like the recipes: `MaxIterations` (default 3),
  `MaxRunDuration` (default 90 s), a rune cap on input, and a terminal judge
  that runs once. The handler enforces the duration on the request context.
- **Synchronous by design**: cancel on disconnect, stream progress over SSE,
  persist nothing except the prompt/completion record and the emitted run.
- **Human escalation is a first-class outcome** (`needs-human` with a reason),
  not an exception. A confidence threshold below which the flow refuses to
  decide is the difference between a helper and a liability.
- **Language policy**: Genkit Go is Go, so it does not weaken
  `docs/language-policy.md`. It lives in `cmd/` or a feature package — never
  in `clients/<client>/automation/`, which is recipe/flow source only.

## Cost, and the reason to say no

Genkit is a plugin with a real dependency delta and its own dev surface.
It earns that weight only when a loop is the product. For everything else the
engine already has the primitives, with replay, approvals and `on_failure`.

Removal is a feature removal, not a refactor: delete the feature package and
its wiring, then `go mod tidy` drops Genkit from the build (gogogo keeps this
true with a flat-dependency check).

## Decision checklist

Answer in order; the first `yes` decides:

1. Does a human watch the loop finish, in this request? → **Genkit**.
2. Must it survive a restart, a queue, or a wait? → **recipe** (`on_failure`
   included).
3. Is it one call whose output the recipe validates? → **`llm.complete`**.
4. Is it an unattended loop on a schedule? → platform decision
   (`docs/script-execution.md`), not Genkit and not a recipe.
5. None of the above? → you probably need a report, not a loop.
