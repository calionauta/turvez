# Architecture

Turvez is a **spine** plus **two engines** plus **layers**. The spine owns the
contract and the source of truth; the engine runs the work; the layers add
credentials, models and retrieval without becoming a second orchestrator.

```
                    ┌──────────────────────────────────────────────┐
   git (source of   │ SPINE — this repo                            │
    truth, review)  │ clients/<client>/client.yaml  engine+runtime │
                    │ README.md fieldbook · evals/ · env/ + age    │
                    │ gates: go test · make validate · lint-scripts│
                    └───────────────┬───────────────┬──────────────┘
                                    │               │
                   engine: windmill │               │ engine: dagnats
                                    ▼               ▼
              ┌─────────────────────────┐   ┌──────────────────────────┐
              │ Windmill (default)      │   │ dagnats (exception)      │
              │ one instance, workspace │   │ embedded NATS JetStream  │
              │ per client; flow.yaml + │   │ + HTTP API + console     │
              │ Go script nodes; UI,    │   │ 12 Go tasks; one binary  │
              │ forms, step replay      │   │ native approval/sleep    │
              └───────────┬─────────────┘   └────────────┬─────────────┘
                          │                              │
                          └──────────────┬───────────────┘
                                         │
          ┌──────────────────────────────┼──────────────────────────────┐
          ▼                              ▼                              ▼
 ┌─────────────────┐          ┌──────────────────┐          ┌─────────────────┐
 │ Nango           │          │ LLM gateway      │          │ Retrieval       │
 │ OAuth + proxy   │          │ llm.complete →   │          │ long context or │
 │ Agent Sessions  │          │ one auditable    │          │ Go + Bleve      │
 │ (MCP)           │          │ model surface    │          │ (per client)    │
 └────────┬────────┘          └────────┬─────────┘          └────────┬────────┘
          ▼                            ▼                             ▼
   external SaaS              provider / gateway            client documents
```

Deployment is orthogonal: profiles A (embedded), B (managed, the SMB default)
and C (multi-tenant), and `runtime: shared|dedicated` per client
(`docs/deployment-profiles.md`). The engine choice and the deployment profile
never change the repo shape.

## Components

- **Spine** — `clients/<client>/` holds `client.yaml` (engine, runtime, target,
  data class, owner), the fieldbook, `automation/<engine>/` and `evals/`.
  `clients/recipes_test.go` enforces the manifest, one-engine rule, tenancy,
  fieldbooks, jq expressions and the secrets guard. `scripts/validate.sh` runs
  every engine gate that has source in the checkout.
- **Windmill — default engine** (`engines.json` → `"default": "windmill"`).
  One instance with a workspace per client. The graph is `flow.yaml`; the work
  is **Go script nodes** (`func main(...) (interface{}, error)`), so the
  compiler checks the contract and Windmill derives the input schema/UI.
  TypeScript/Python is legal only for a dispatch-only WAC orchestrator
  (`docs/language-policy.md`). Costs Postgres + workers; AGPL core, EE for
  SSO/audit/PR gates. Deploy one flow at a time — never `wmill sync push`.
- **dagnats — exception engine** (pinned `v0.0.32`). One binary: embedded NATS
  JetStream + engine + HTTP API + console. The worker (`cmd/worker`) registers
  twelve generic Go tasks and **pulls**; it is stateless and horizontally
  repeatable. Recipes are JSON in `clients/<client>/automation/dagnats/`.
  Chosen for typed contracts, native approvals, no arbitrary code, one binary,
  or an AGPL/Postgres constraint.
- **Dagu — level 2 only.** A separate GPLv3 binary that runs YAML DAGs over
  commands the client already has (SSH, backup, deploy). No repo integration
  and never a second orchestrator for a client with dagnats/Windmill source.
- **Nango — credential and tool layer.** OAuth/token store plus an HTTP proxy;
  scripts and recipes call `${NANGO_HOST}/proxy/...` with `Connection-Id: <client>`
  and `Provider-Config-Key: <provider>`, so no raw token reaches git, Windmill
  or the run. Agent Sessions add a per-session MCP server (`mcp_url` +
  `session_token`) with a scoped toolset; that is for an agent choosing tools,
  not for a deterministic recipe. `docs/nango.md`, `docs/retrieval.md`.
- **LLM gateway.** Every model call is an `llm.complete` step with a required
  `base_url` (`LLM_BASE_URL` fallback) — never a silent direct provider. One
  surface for cost, latency and errors; per-client virtual keys per deployment
  profile. Prompt caching, when present, is the gateway's behaviour
  (`docs/retrieval.md`).
- **Genkit — optional capability, not an engine.** A bounded, human-watched
  agent loop (review/rewrite, triage with read-only tools, extraction with a
  critic) runs in a Go service (`cmd/<client>-agent`) inside one request; goai
  stays the only model path, Nango the tool layer, and anything durable is
  emitted as a run in the client's engine (`docs/genkit.md`). Scaffolded by
  `scripts/new-agent-service.sh` and compiled in CI by
  `scripts/check-agent-template.sh`. Never a second orchestrator, never an
  `engines.json` entry.
- **Retrieval.** A per-client capability, not a runtime: **route-then-read**.
  The 2026 default is lexical for identifiers and hybrid (BM25 + a small dense
  model over the same indexing units, RRF, optional cross-encoder rerank of
  units) for natural-language queries, not vectors-first. Bleve returns whole
  units (chapter, or section when a chapter is too long) within a token budget,
  and the LLM reads the parent of the matched chunk, not the chunk alone.
  Chunking is the workaround for a small reader window; when the model can read
  a section or chapter, that is the unit. A curated glossary expands the query
  before routing; a query rewrite is an optional step inside the hybrid route,
  not a default rung. Where a rewrite step exists at all in this runtime, keep it
  to **one cached rewrite with a fallback to the original query**, not a
  multi-rewrite fan-out.
  The funnel is bounded by a hard token budget, a candidate cap and a calibrated
  refusal threshold; the reranker scores units, never snippets. The eval gates
  the whole shape before it is used with clients. Vectors, if the eval ever
  demands them, live in Bleve itself.
  Systems of record are called at the source through Nango; documents get an
  index and an eval (`docs/retrieval.md`).
- **Evals.** `evals/` is engine-agnostic. `select-cases.py` reads dagnats runs;
  `windmill-job-to-run.py` normalizes a Windmill job into the same shape; then
  label + promptfoo. Cases are deterministic first, rubric only when necessary.
- **Sandbox.** `scripts/sandbox-run.sh` runs a one-off script in a throwaway
  container with only the repo mounted read-only. It never becomes automation.

## Data flow

### dagnats: refund with a human gate

1. `run start example-client.approval-refund '{order…}'` → `build-review`
   shapes the review object.
2. `approve` (native step, no worker) publishes `approval.requested` + token and
   waits up to 48h. The grant payload becomes the step output; reject goes to
   `on_failure: notify-rejected`.
3. `build-refund` merges `{approve, build-review}` into an `http.request`
   payload; `refund` POSTs; `build-receipt` + `notify-done` close the loop.

### Windmill: the shape of a flow

1. A schedule, webhook or manual run starts `flow.yaml` at its first step; the
   scaffold's `example-flow` is one Go node (`func main() (interface{}, error)`).
2. Each step's output is persisted per node; a failed step can be replayed from
   that node without re-running the earlier ones.
3. Production flows carry an error handler that notifies the operator; the
   external dead-man switch covers the case where the flow itself cannot run.

### Documents: question to answer (route-then-read)

1. Batch indexer (Go node/service) segments by structure and writes Bleve with
   one record per **chunk** (~256-512 tokens, 50-100 overlap): `doc_id`,
   `unit_id`, heading path, `parent_section_id`, offset, tokens, `acl`.
2. The router expands the question with the client's glossary, applies the
   `acl` filter, and returns candidate chunks (25-50, hybrid for natural
   language).
3. The reader expands each chunk to its **parent section/chapter** and fills a
   **token budget** with those units — not a fixed top-k of chunks; a parent
   that exceeds the budget is read as its subsections.
4. The prompt is built in a prior step; `llm.complete` reads the units in long
   context and answers as JSON with `citations`. Citations outside the selected
   set are rejected.
5. A 30-50 question eval gates the change; tier 0 (whole corpus) and a chunked
   baseline are what it must beat. If lexical routing misses, add vectors to
   Bleve rather than a second database.

## Decisions

- **The repo is the spine, not the runtime.** The engine is per client in
  `client.yaml`; changing it does not change the folder, the fieldbook, the
  evals or the tenancy rules.
- **Windmill by default, dagnats by exception.** The default covers UI, replay
  and non-trivial code; dagnats is chosen only for the contract criteria in
  `docs/client-spine.md#choosing-the-engine`.
- **Recipes over handlers** (dagnats). Conditionals, loops and extraction cover
  almost all client work; a new Go handler needs typed I/O, table tests and
  human review.
- **Go for the nodes** in both engines. TS/Python exists only where Windmill
  implements the graph API, and only as dispatch (`docs/language-policy.md`).
- **Credentials in Nango, not in the repo.** Provider tokens are connections;
  the client slug is the connection id. `clients/credentials_test.go` enforces
  the naming and follows `WORKFLOWS_DIR` into an ops repo.
- **Retrieval is lexical-first and reads parents.** Bleve indexes and routes
  chunks, the LLM reads their parent section/chapter; glossary before WordNet, before FastText,
  before vectors; each step needs an eval that justifies it. The funnel is
  bounded by a hard token budget, a candidate cap and a calibrated refusal —
  and "bounded" is not "immune".
- **Env + age, no vault** (solo). Process env in prod, age files for versioned
  backups; revisit with a second operator or compliance pressure.
- **Engine sleeps, not worker sleeps.** Multi-day waits are `sleep`/`approval`
  in the DAG; the `delay` handler is capped at 4h so a stuck run is loud.
- **One server, then extract.** Client identity = folder + name prefix + env.
  A dedicated host is a deployment move (`runtime: dedicated`), not a repo fork.

## Verified upstream behaviors (against the pinned dagnats source)

Recorded so upgrades re-check them: durations parse as **integer nanoseconds or
Go duration strings** (`"30s"`); `$schema` is accepted (optional); `config`
exists on map/sleep/approval steps though the JSON schema file omits it;
`aux_steps` must be set manually in JSON; `sleep`/`approval` steps take no
`task`; `map` needs exactly one array-emitting dep; trigger runs get a
TriggerEnvelope (`data` holds the payload); `WaitForSignal` ≤1h per call;
approval grant payload = step output, reject = failure path; cron schedules
default to UTC (`cron_timezone` / `cron_timezone_input_path` override it);
`output_schema`, when set, is enforced against the run output.
