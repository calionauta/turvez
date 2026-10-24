# Where to execute? dagnats vs one-off vs Dagu vs Windmill

Short answer: **dagnats is the default**. Everything else is an exception,
and the exception has an owner. This page is the day-to-day decision tree;
`docs/migrate-to-windmill.md` remains the migration plan.

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
Yes → level 0. Write the recipe, `validate` + `go test ./workflows/`.
Ex.: HubSpot deal → Slack → Notion; invoice VIP split; refund with approval.

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
Yes → level 3, Windmill, **per client** (see
`docs/migrate-to-windmill.md` + `docs/isolating-a-client.md`):
repoint webhooks, run in parallel until green, turn off dagnats
for that client.

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
