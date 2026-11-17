# docs/ — the index

The entry point for **both** readers. A human opens this folder in an editor; an
agent lands here after `llms.txt` points at a task. One table, three columns:
what the doc is for, when to read it, and the check that keeps it honest.

If a doc is not in this table, `scripts/test-decisions.sh` fails (`doc-index`).
That is the whole rule: a doc nobody routes to is dead weight, and a table
nobody checks is a lie. `llms.txt` (repo root) is the **same set, ordered by
task** — read it when you have a job, read this when you have a doc name.

## Start

| Doc | Read when | Gate |
|---|---|---|
| `ops-repo.md` | first: template (public) vs ops (private), lifecycle, tenancy map | `go test ./clients/` |
| `client-spine.md` | second: the spine, per-client engine, shared vs dedicated, migrations | `scripts/test-decisions.sh` |
| `first-run.md` | installing and running it for the first time, with the known traps | `scripts/setup.sh` |
| `deployment-profiles.md` | choosing embedded / managed / multi-tenant and where the gateway sits | `scripts/verify-isolation.sh` |
| `script-execution.md` | deciding **where** a job runs: recipe vs sandbox vs Dagu vs Windmill | `scripts/test-decisions.sh` |
| `sandbox-policy.md` | asking "do I need a sandbox?" — usually the answer is no | none |

## The ask

| Doc | Read when | Gate |
|---|---|---|
| `workflows.md` | wanting the diagrams: repo choice, automation, eval loop, formats | none |
| `writing-recipes.md` | writing or editing a recipe: anatomy, engine-vs-handler, engine traps | `dagnats workflow validate` |
| `handlers-reference.md` | needing a task's inputs/outputs — never invent a task name | `go test ./internal/handlers/` |
| `browser.md` | the data only exists on a page with no API (Q1.7) | `scripts/test-decisions.sh` (`capability-browser`) |
| `traffic-capture.md` | the target is an **app**, not a page — and where reverse engineering stops (Q1.7.2) | `scripts/test-decisions.sh` (`capability-traffic`) |
| `genkit.md` | a human watches an agent loop; the durable tail is still a run | `scripts/check-agent-template.sh` |
| `retrieval.md` | answering questions over documents: route-then-read, caps, refusal | `scripts/test-decisions.sh` (`retrieval-unit`) |
| `language-policy.md` | writing a script node (Go; TS/Python only for a dispatch-only WAC) | `scripts/lint-scripts.sh` |
| `eval-strategies.md` | choosing an eval method and calibrating a judge | `scripts/test-eval-strategies.sh` |
| `observability.md` | deciding what is stored, prompt persistence, retention | `go test ./internal/handlers/` |

## The layers

| Doc | Read when | Gate |
|---|---|---|
| `nango.md` | a client's OAuth/SaaS credentials: connection naming, proxy, Agent Sessions | `go test ./clients/` |
| `access-plane.md` | a client's **own** agents calling authenticated tools (the third plane) | `go test ./clients/` |
| `cloud-remotes.md` | recording Windmill/Nango cloud vs self-hosted endpoints per client | `scripts/connect-remotes.sh --dry-run` |
| `secrets.md` | anything touching a secret value: names in JSON, values in env/age only | `go test ./clients/` |
| `tooling.md` | considering a new tool (Nango, Corsair, ACI, agentgateway, Bifrost, OpenBao, Agent Substrate) | `scripts/tool-pick.sh` |

## Operating it

| Doc | Read when | Gate |
|---|---|---|
| `agent-tools.md` | authoring as an agent: exact CLI/API, plus `turvez` (read-only router) | `go build ./cmd/turvez` |
| `alerting.md` | a production recipe needs `on_failure`, the DLQ watchdog, an uptime probe | `go test ./clients/` |
| `tenancy-evolution.md` | growing from one shared stack to per-client isolation without redesign | `scripts/verify-isolation.sh` |
| `isolating-a-client.md` | extracting one client onto its own host | `scripts/extract-client.py` |
| `migrate-to-windmill.md` | moving a dagnats client to Windmill (mapping table, skeleton converter) | `scripts/test-convert-to-windmill.py` |
| `spike-local-script-gate.md` | understanding why `lint-scripts` exists beside `wmill lint` (measured 2026-11-01) | `scripts/lint-scripts.sh` |

## Traps a fresh clone hits

The six things that produce a confident, wrong answer. Full text in `llms.txt`
(`## Traps a fresh clone hits`); the short version:

| Trap | What looks true | What is true |
|---|---|---|
| `go test ./...` passing | the repo is consistent | only `make validate` checks tree ↔ docs ↔ engines |
| `engines.json` says the engine | editing it switches clients | `client.yaml` wins, per client |
| `DAGNATS_API_URL` moves the CLI | the CLI is talking to the worker | it is NATS-first; that var is the approval shim only |
| a provider token in `env/` | it works locally | `go test ./clients/` fails it; use a Nango connection |
| a task name you remember | it exists | the registry is `internal/handlers/handlers.go`, nothing else |
| `clients/personal/` is "quick" | it skips review | same gates, you are the approver |

## Not in this folder

| Where | What |
|---|---|
| `AGENTS.md` | normative rules for an agent working in this repo — read first |
| `ARCHITECTURE.md` | diagrams, components, data flow, decisions |
| `README.md` | quickstart, operator loop, who this is for |
| `llms.txt` | the same docs, ordered by **task** instead of by name |
| `skills/turvez/SKILL.md` | the automate-anything skill: direct → confirm → build |
| `skills/turvez/references/decision-tree.md` | the canonical tree, the interview protocol, the defaults table |
