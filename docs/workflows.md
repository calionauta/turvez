# Workflows

The paths through the repo, as diagrams. GitHub renders Mermaid.

## 1. Which repo am I in, and what am I doing?

```mermaid
flowchart TD
    A["Fresh checkout"] --> B{"go.mod or TURVEZ_PIN?"}
    B -->|"go.mod"| C["Template: code, tests, docs"]
    B -->|"TURVEZ_PIN"| D["Ops repo: client data only"]
    C --> E["Client work?"]
    E -->|"new client"| F["new-client.sh in the ops repo"]
    E -->|"automation"| G["workflows/client/*.json"]
    E -->|"eval"| H["evals/client/"]
    D --> F
```

`new-client.sh` scaffolds `workflows/<client>/`, `env/<client>.env.example`,
`secrets/<client>/` and `evals/<client>/` in one pass.

## 2. Automation: build, run, observe

```mermaid
flowchart LR
    T["Trigger: cron / webhook / manual"] --> W["dagnats engine"]
    W --> K["Worker: 12 handlers"]
    K --> S["External SaaS"]
    K --> H["Human approval"]
    K --> R["Run history: KV + events"]
    R --> A["Alerts: on_failure, DLQ, health"]
```

A recipe is JSON in `workflows/<client>/`. Git is the source; `workflow
register` puts it in the engine. Every run keeps its input and each step's
output, which is what the eval reads.

## 3. Eval loop: from runs or logs to a measured dataset

```mermaid
flowchart TD
    R["dagnats runs"] --> S["select-cases.py"]
    L["logs: JSONL / CSV / text"] --> I["import-logs.py"]
    S --> C["cases/cases.json"]
    I --> CSV["cases/logs.csv"]
    C --> L1["label-cases.py"]
    CSV --> HUM["human fills labels"]
    L1 --> LB["labels.json"]
    LB --> C2["cases.json + expected + JS"]
    HUM --> CFG["promptfooconfig.logs.yaml"]
    C2 --> E["promptfoo eval"]
    CFG --> E
    E --> RES["results.json + UI at :15500"]
    RES --> D{"deterministic check possible?"}
    D -->|"yes"| DET["schema + label: free, stable"]
    D -->|"no"| J["judge: LLM, or JEV with escalation"]
```

Use the run path when the automation already calls a model: production output
becomes a *suggestion*, not a label. Use the logs path when the client has only
inputs: labels are human from the start.

## 4. Deployment profiles

```mermaid
flowchart LR
    A["A. Embedded"] --> A1["client server, client keys"]
    B["B. Managed"] --> B1["your server, one stack per client"]
    C["C. Multi-tenant"] --> C1["one stack, virtual keys per tenant"]
```

Details and gateway placement: `docs/deployment-profiles.md`.

## Formats: pick by role, not by taste

| Role | Format | Why |
|---|---|---|
| Raw logs to import | **JSONL**, CSV, plain text | JSONL is the common structured-log shape; one record per line, streams without loading everything |
| Generated cases | **JSON** (`cases/cases.json`) | machine output from `select-cases.py`; never hand-edited |
| Hand-written cases | **YAML** (`curated.yaml`) | comments, multi-line text, per-case assertions |
| Label worksheet | **CSV** (`logs.csv`) | spreadsheets; one row per input, empty `expected_category` |
| Labels for generated cases | **JSON** (`labels.json`) | script-owned map keyed by input hash |
| Promptfoo config | **YAML** | native format, supports comments |
| Per-client eval config | **JSON** (`eval.config.json`) | small, parsed by the scripts without extra dependencies |
| Results | **JSON** + HTML | machine-readable and human-readable |

In short: **JSONL for machine logs, CSV for human tables, YAML for authored
text, JSON for generated artifacts.** promptfoo accepts all of them, so the
choice is about who edits the file and how it diffs, not compatibility.
