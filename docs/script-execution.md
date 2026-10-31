# Where to execute? dagnats vs one-off vs Dagu vs Windmill

Short answer: **it depends on the client's engine, not on taste.** A client
already on dagnats stays there (level 0 below); a **new client starts on
Windmill** (level 3) unless the work is clearly dagnats-shaped and the operator
says so. dagnats is legacy (`docs/client-spine.md`, phase 4): it is supported,
documented and tested, but it is no longer where new work lands by default.
Everything below is still the day-to-day decision tree;
`docs/migrate-to-windmill.md` is the migration plan.

**One engine per client.** In steady state, levels 0, 2 and 3 are mutually
exclusive for the same client's orchestration (overlap is allowed only during
a cutover). Two engines = 2x monitoring, 2x secrets, 2x DLQ, 2x upgrade path.
Pick by the shape of the work, not by taste.

## The 4 levels (cheapest to most expensive to operate)

| # | When | Tool | Operating cost |
|---|---|---|---|
| 0 | SaaS glue, cron, human approval, retries, DLQ | **dagnats** (this repo) | 1 stack, already operated |
| 1 | One-off script / LLM-generated code test | `scripts/sandbox-run.sh` | zero — local docker, throw away after |
| 2 | **Recurrent** script/SSH/Docker that stays script-shaped | **Dagu** (ad-hoc, not integrated into the repo) | 1 binary, no DB/broker + a second control plane: UI, secrets, alerts, upgrades |
| 3 | Recurrent + needs generated UI, internal apps, integration catalog | **Windmill** (per-client migration) | Postgres + server + workers |

"Cheapest to operate" measures **surface you now own**, not RAM. Dagu's
footprint is small (measured on v2.22.2, macOS arm64: 166 MB binary, ~115 MB
RSS idle for `start-all` serving UI + scheduler + coordinator); the cost is
the second scheduler, the second secret store, and the second place a run can
get stuck. See "Dagu 2.x reality check" below.

Levels 2 and 3 **replace each other** for orchestration — and so does level 0
against either. Official Dagu-vs-Windmill comparison:
`https://dagu.sh/dagu-vs-windmill`.

## Decision tree (ask in order)

**Q1. Can it be expressed with the 12 handlers?**
`http.request`, `transform.json`, `condition`, `map`, `delay`,
`notify.*`, `request.approval`, `llm.complete`, `dlq.check`
(see `docs/handlers-reference.md`).
Yes → level 0, and only when the client's `engine` is `dagnats`. Write the
recipe, `validate` + `go test ./clients/`.
Ex.: HubSpot deal → Slack → Notion; invoice VIP split; refund with approval.
The same logic on a `engine: windmill` client is a flow, not a recipe: the 12
handlers have direct equivalents (`docs/migrate-to-windmill.md`, step mapping).

**Q1.5. Does it need an agent that loops with tools (shell, files, browser)
until it is done?**
Then it is **not** a recipe and **not** level 2 — it is a new platform
decision (Dagu 2.x `llm.*` / `harness.run` / agent DAGs, or a dedicated
harness). Do not smuggle an `agent_loop` step into a recipe: this runtime
has no handler for it and the tools are arbitrary code. Rationale:
`docs/writing-recipes.md` → "Why AgentLoop/planner stay upstream-only".

**Q2. Is it new code that runs once?** (LLM-generated parser, CSV cleanup,
client API probe)
Yes → level 1. Read the script, copy it inside the repo (the sandbox only
mounts the repo at `/work:ro`), run isolated, throw away:

```bash
cp /tmp/migra.py ./scratch-migra.py
./scripts/sandbox-run.sh --allow-net=off ./scratch-migra.py
```

Details in `docs/sandbox-policy.md`. If the script needs secrets, pass
`--secret-env` explicitly — never export them in the day-to-day shell.

**Q3. Is it a command the client already has and will repeat?** (SSH backup,
`deploy.sh`, `python report.py`, `ffmpeg`, `dbt`, `kubectl`)
Yes, and still script-shaped → level 2, ad-hoc Dagu:

```bash
# install once (single binary, no DB)
curl -fsSL https://raw.githubusercontent.com/dagucloud/dagu/main/scripts/installer.sh | bash

# one YAML per job; the dags dir is the deployment target, git is the source
DAGS=~/.config/dagu/dags
mkdir -p "$DAGS"
cat > "$DAGS/backup-acme.yaml" <<'EOF'
schedule: "0 3 * * *"
steps:
  - id: backup
    run: ssh deploy@acme.internal /opt/backup.sh
    retry_policy: {limit: 3, interval_sec: 30}
EOF

dagu validate "$DAGS/backup-acme.yaml"
dagu start "$DAGS/backup-acme.yaml"   # test: run once, no scheduler
dagu start-all --dags "$DAGS"         # scheduler + queue + UI :8080
```

`--dags` is a flag of `start-all` / `server` / `scheduler`, **not** of
`dagu start` (verified v2.22.2; older notes here had `dagu start --dags`,
which fails).

Rules: one YAML per job (source in the client's private repo, dags dir as the
deployment target), logs in the Dagu UI, secrets in host env (never in YAML),
no integration with this repo. Dagu is GPLv3 — separate binary only, never
copy code into this repo.

**Q4. Did it become the client's internal product?** (needs web form for
the team, integration catalog, approvals with UI, low-code apps)
Windmill is both the **default for new clients** and the migration target for
a client that outgrows dagnats. The client's `client.yaml` records the choice.

Yes → level 3, Windmill, **per client** (see
`docs/migrate-to-windmill.md` + `docs/isolating-a-client.md`):
repoint webhooks, run in parallel until green, turn off dagnats
for that client. For a brand-new client there is nothing to migrate: scaffold
with `new-client.sh` (engine defaults to `windmill`) and follow
`skills/turvez/references/windmill.md`.

## Dagu 2.x reality check (verified 2026-10-24, v2.22.2)

This page used to describe Dagu as "a nicer cron" — single binary, YAML,
retries, UI. That framing is no longer accurate, and the difference matters
when choosing.

**What it is:** local-first workflow engine, one Go binary (166 MB), no
external DB or broker, YAML DAGs, file-based run history/logs/queue, web UI,
optional distributed workers (coordinator over gRPC).

**What grew in 2.x:**

- Native **MCP server** at `/mcp`: agents inspect runs, edit DAGs, trigger runs.
- **Approval gates** and **human tasks** (durable wait, resume by API/UI).
- **Sub-DAGs** (`dag.run`) with fan-out and concurrency limits.
- **LLM steps**, **agent DAGs**, **`harness.run`** (Claude Code, Codex,
  OpenCode), browser/computer actions.
- Queues, overlap policies, catch-up windows, RBAC/SSO (paid features).

**Durability semantics — do not oversell it:**

- "Durable Execution" in Dagu's docs means **retries**: per step, defaults,
  and a DAG-level retry issued by the scheduler.
- Crash mid-run: the zombie sweep (default 45s) detects stalled/orphaned runs
  and closes them out — they do not continue on their own. Recovery is
  **manual** `dagu retry --run-id <id> --step <name>`, which preserves the
  other steps' statuses.
- There is **no automatic resume of an in-flight step**. It is not Temporal.
  Step-level retry is the checkpoint.

**License:** GPL-3.0-or-later. Using the CLI/server as a separate process is
fine. Never import the embedded Go API into our binaries, and note that
SSO/RBAC/audit and console monitoring beyond the free tier are paid.

**Competition signal:** if a client's Dagu YAML starts carrying `llm.*`,
`harness.run`, approval/human-task steps, or orchestration of work that
already lives in a dagnats recipe, that client now has **two orchestrators**.
Stop and choose one — that is the line this page exists to hold.

## When NOT to use Dagu

- For what dagnats already does (Q1 yes): two engines = 2x monitoring,
  2x secrets, 2x DLQ. Don't pay that price for redundancy.
- As the home for an agent loop / autonomous LLM pipeline. Dagu 2.x *can*
  run it, but that is a platform decision (cost, audit, client data
  boundary), not a level-2 script job. See Q1.5.
- As a library inside the Go worker (GPLv3 + coupling).
  Dagu is a separate process or nothing.
- As a replacement for `sandbox-run.sh` for one-offs: standing up a
  scheduler to run a script once is overkill.

## Consultancy examples

| Request | Level | Why |
|---|---|---|
| Qualified deal → Slack + Notion, approval >10k | 0 dagnats | SaaS glue + multi-day human gate |
| Nightly HubSpot → warehouse sync | 0 dagnats | cron + retries + DLQ |
| LLM-generated Python parser to clean client CSV | 1 sandbox-run | runs once, then becomes `transform.json` or dies |
| Weekly SSH backup on 3 client servers | 2 Dagu | recurrent command, what Dagu was born for |
| Deploy `git pull + docker compose up` for the client | 2 Dagu | SSH + retry + log in the UI |
| Client team wants a "generate report" button with form | 3 Windmill | generated UI + catalog, outside dagnats and plain Dagu scope |
| Agent that codes/tests in a loop until green | new decision | needs tool execution + per-iteration checkpoint; not a recipe |

## Recorded verdict

Dagu **is** an alternative to Windmill's orchestration part (scheduling +
retries + logs + run UI), with the advantage of not requiring Postgres.

Dagu is **not** a drop-in alternative to dagnats here, but the difference is
narrower than "Dagu can't do approvals". It is:

| Axis | Dagu 2.x | dagnats here |
|---|---|---|
| Unit of work | existing command/script/SSH/container | typed Go handler + JSON recipe |
| Arbitrary code from definition | yes — that is the product | forbidden by design (`AGENTS.md`) |
| State | files + run history | event log (JetStream) + stateless engine replay |
| Crash mid-step | zombie sweep aborts; manual `retry --step` | unacked task redelivered; handler owns checkpoint/idempotency |
| Human gate | approval gate / human task | native `approval` step + tokenized API + signals |
| Secrets | env/Vault/K8s, per instance | env + age, per client, in git |
| License | GPLv3 + paid ops features | Apache-2.0 |

Operate only one per client: dagnats default, Dagu ad-hoc for script-shaped
work, Windmill by migration.

## Platform choice beyond turvez

The levels above answer "where does this automation run inside turvez". The
question "should this client use turvez at all" needs a second cut:

| Need | Choose | Why |
|---|---|---|
| Typed API automation, native approvals, no arbitrary code, light footprint, Apache-2.0 | **dagnats** | contract + tests + git; 1 binary |
| Client-facing UI/forms/apps, replay from a step, test one step, code steps in TS/Python/Go | **Windmill** | flow editor, step results, restart from node |
| 400+ SaaS nodes, visual builder, client team edits the flow | **n8n** | catalog and UI; heavier, SUL license |
| Recurrent script/SSH/Docker job, step-level retry, one binary | **Dagu** | script-shaped work; GPLv3 |
| One command on a schedule, nothing else | **cron** | zero new surface |
| OAuth/refresh for many SaaS APIs | **Nango** (optional layer) | self-hosted credential broker; HTTP proxy |
| Agent tool auth over MCP | **Executor** (optional layer) | per-connection auth and policies |

Operate **one orchestrator per client**. Nango and Executor are layers, not
orchestrators: they can sit in front of dagnats or Windmill, but they do not
replace either.

### Windmill, free edition

- Replay/resume is real: select a top-level node in the flow preview and use
  "Re-start from X"; also test up to a step, test an iteration, test one step.
- Secrets: Variables/Secrets encrypted per workspace, masked in logs, path
  ACLs. Audit logs are 14 days in the community edition; SSO/SAML, durable
  audit, concurrency limits, critical alerts, autoscaling and external secret
  managers are Enterprise.
- **Alerting in the free edition is code, not a feature.** Put `http` or
  `smtplib` steps in the flow (Slack/Telegram/Discord webhook, email API or
  SMTP), or use a flow error handler. There is no native "critical alerts"
  without EE. Add an external dead-man switch (`GET /health`, Uptime Kuma,
  Healthchecks) because the alerting flow can fail silently.
- License: AGPLv3 core plus proprietary EE code. Hosting it for clients and
  exposing the service triggers AGPL obligations; white-label/SSO is EE.

### n8n

Catalog and visual building are its advantage; the cost is Node + Postgres
(+ Redis in queue mode), the Sustainable Use License (restrictions on offering
it as a service), and a Code node that runs arbitrary JavaScript. It does not
give typed contracts, git-first recipes or step replay at the level Windmill
does.

### Nango (optional)

Nango self-hosted handles OAuth, token refresh and encrypted credential
storage for 1,000+ APIs. It is a layer, not a runtime: dagnats and Windmill
call its **proxy** over HTTP with a static Nango key and the connection id, and
Nango injects the provider token. It adds Node + Postgres and is licensed
under Elastic License 2.0 (free self-host for auth/proxy; some features are
Enterprise). Confirm with the operator before installing: it is another
service, another database and another place where client tokens live.

### Channels

Windmill has no n8n-style channel nodes. It sends email, Discord, Telegram,
Slack or anything else through a script step (HTTP API, SMTP, provider SDK).
That is enough for operator alerts, but it is code, not a catalog.
