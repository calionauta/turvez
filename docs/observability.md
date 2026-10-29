# Observability and data rules (recipe level)

What the runtime already stores, and what a recipe must do so the answer is not
"nowhere". Read this before adding an LLM step or a tool call.

## What is stored, where

| Data | Store | Retention |
|---|---|---|
| Run input, per-step output, status, attempts, timings | dagnats KV `workflow_runs` | **no TTL** — purge is a deployment task |
| Step lifecycle events | `WORKFLOW_HISTORY` stream | 30d (`EVENTS` 14d, `DEAD_LETTERS` 30d) |
| Prompt + response | KV — **only if emitted by a step** (see rule 2) | same as the run |
| Tool/RAG HTTP request+response | the step's output | same as the run |
| OTel traces/metrics/logs | `dagnats sidecar` → Parquet (DuckDB MCP) | disk |
| Eval results | `promptfoo` local DB + `results.json` | local |

Step **inputs** are not persisted: they travel in the task message, which is
deleted on ack.

## Rules

1. **Every model call is an `llm.complete` step.** The handler's transport is
   GoAI (`provider/openai`); that is an implementation detail, not a second
   entry point. A script calling a model directly is a permanent blind spot.
2. **Build the prompt in a step.** Emit `messages` from `transform.json`
   (`build-prompt`) instead of inlining them in `llm.complete` metadata: the
   prompt then becomes a step output and a failed call stays replayable.
3. **Redact before persisting.** The build step is where PII is removed. Once
   the prompt is an output, it is in the run record — redacting afterwards is
   always worse.
4. **Gate on schema, not status.** A `condition` step asserting the output
   shape is the quality gate; `status = completed` proves nothing (a reasoning
   model that spent `max_tokens` thinking returns empty content with 200).
5. **`max_tokens` covers reasoning *and* the answer.** Use ≥512 for reasoning
   models; the handler fails empty completions non-retryably.
6. **No model traffic outside dagnats.** One audit surface, one place to
   purge, one place to observe.
7. **Retention is a deployment decision, not a config default.** The KV has no
   TTL: agree a window (e.g. 30–90d), run the purge, and put it in the
   contract.
8. **Gateway content logging stays off by default.** Metadata answers cost,
   latency, errors, fallback and routing. Prompts/responses in a gateway log
   are a DPA decision, not a default.
9. **Export cases only with approval, redacted, into the client's ops repo.**
   Aggregates (cost, latency, error rate) are shareable; raw prompts are not.

## Seeing a run end to end

```bash
export DAGNATS_NATS_URL=nats://127.0.0.1:4222     # the CLI is NATS-first
dagnats run list --workflow=acme/                 # ids + status
dagnats run inspect <run-id> --json               # steps, attempts, outputs (base64)
dagnats run events <run-id>                       # lifecycle + step payloads
dagnats trace <run-id>                            # OTel span tree
dagnats logs                                      # telemetry log stream (tails)
```

Decode one step output:

```bash
jq -r '.run.steps["build-prompt"].output' run.json | base64 -d | jq .
```

Console UI: `http://127.0.0.1:8080/console/` (or `${DAGNATS_HTTP_PORT}`).

## Eval loop

`scripts/select-cases.py` turns run history into frozen cases; each client's
`evals/<client>/promptfooconfig.yaml` replays them through its gateway
(`make eval-client CLIENT=<id>`). Two rules: generation params must mirror
production (`max_tokens`), and the rubric must carry `{{text}}` or it grades
blind.
