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
    E -->|"automation"| G["clients/client/automation/dagnats/*.json"]
    E -->|"eval"| H["clients/client/evals/"]
    D --> F
```

`new-client.sh` scaffolds `clients/<client>/`, `env/<client>.env.example`,
`secrets/<client>/` and `clients/<client>/evals/` in one pass.

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

A recipe is JSON in `clients/<client>/automation/dagnats/`. Git is the source; `workflow
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

## 5. Operator flow: from request to production

What the operator does, what the agent does, and where to check.

```mermaid
sequenceDiagram
    actor O as Operator
    participant A as Agent + turvez skill
    participant R as Ops repo
    participant D as dagnats
    participant U as Console / CLI

    O->>A: "automate X for client acme"
    A->>A: Q0 repo type + decision tree
    A->>O: intake questions: systems, trigger, approval, secret names
    O->>A: answers
    A->>R: recipe + fieldbook + env example
    A->>O: "validate + register?"
    O->>A: approve
    A->>D: register + trigger
    D-->>U: runs, events, DLQ
    O->>U: check run, alert, DLQ
    A->>O: "want the eval loop for this client?"
    O->>A: approve cases and eval
```

| Step | Operator | Agent | Where to check |
|---|---|---|---|
| 1. Ask | describes the integration in natural language | loads `turvez` skill, Q0, decision tree | the agent's answer |
| 2. Intake | answers systems, trigger, approval rule, secret **names**, alert channel | asks, then writes the fieldbook | `clients/<client>/README.md` |
| 3. Build | reviews the recipe | writes JSON, runs `validate` + `go test` | repo diff |
| 4. Secrets | puts values in `env/<client>.env` or age | never writes values | `env/`, `secrets/` |
| 5. Go live | approves `register` + `trigger` | runs them | `/console/workflows/<client>.<wf>` |
| 6. Operate | checks runs, DLQ, alerts | explains failures | console, `dagnats run list`, Slack/Telegram |
| 7. Eval | labels cases, approves the eval | generates cases, runs promptfoo | `results.json`, `:15500` |

The operator never has to memorize commands. They ask for the outcome; the
skill drives the sequence. The two hard rules are **approve side effects** and
**provide secret values only through env/age**.

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
