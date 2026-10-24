# Where to execute? dagnats vs one-off vs Dagu vs Windmill

Short answer: **dagnats is the default**. Everything else is an exception,
and the exception has an owner. This page is the day-to-day decision tree;
`docs/migrate-to-windmill.md` remains the migration plan.

## The 4 levels (cheapest to most expensive to operate)

| # | When | Tool | Operating cost |
|---|---|---|---|
| 0 | SaaS glue, cron, human approval, retries, DLQ | **dagnats** (this repo) | 1 stack, already operated |
| 1 | One-off script / LLM-generated code test | `scripts/sandbox-run.sh` | zero — local docker, throw away after |
| 2 | **Recurrent** script/SSH/Docker that stays script-shaped | **Dagu** (ad-hoc CLI, not integrated into the repo) | 1 binary, no DB, no Postgres |
| 3 | Recurrent + needs generated UI, internal apps, integration catalog | **Windmill** (per-client migration) | Postgres + server + workers |

Levels 2 and 3 **replace each other** for orchestration —
never operate both in production for the same client. Official comparison:
`https://dagu.sh/dagu-vs-windmill`.

## Decision tree (ask in order)

**Q1. Can it be expressed with the 12 handlers?**
`http.request`, `transform.json`, `condition`, `map`, `delay`,
`notify.*`, `request.approval`, `llm.complete`, `dlq.check`
(see `docs/handlers-reference.md`).
Yes → level 0. Write the recipe, `validate` + `go test ./workflows/`.
Ex.: HubSpot deal → Slack → Notion; invoice VIP split; refund with approval.

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
# YAML workflow next to the client script, versioned together
cat > /tmp/backup-acme.yaml <<'EOF'
schedule: "0 3 * * *"
steps:
  - id: backup
    run: ssh deploy@acme.internal /opt/backup.sh
    retry_policy: {limit: 3, interval_sec: 30}
EOF
dagu start --dags /tmp /tmp/backup-acme.yaml   # test
dagu start-all --dags ~/.dagu/dags             # schedule + UI :8080
```

Rules: one YAML per job, logs in the Dagu UI, secrets in host env
(never in YAML), no integration with this repo. Dagu is GPLv3 — separate
binary only, never copy code into this repo.

**Q4. Did it become the client's internal product?** (needs web form for
the team, integration catalog, approvals with UI, low-code apps)
Yes → level 3, Windmill, **per client** (see
`docs/migrate-to-windmill.md` + `docs/isolating-a-client.md`):
repoint webhooks, run in parallel until green, turn off dagnats
for that client.

## When NOT to use Dagu

- For what dagnats already does (Q1 yes): two engines = 2x monitoring,
  2x secrets, 2x DLQ. Don't pay that price for redundancy.
- As a library inside the Go worker (GPLv3 license + coupling).
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

## Recorded verdict

Dagu **is** an alternative to Windmill's orchestration part
(scheduling + retries + logs + run UI), with the advantage of not requiring
Postgres. It is not an alternative to dagnats in this repo (NATS durability +
multi-day approvals + 12 stable handlers). Keep all three documented and
operate only one per client: dagnats default, Dagu ad-hoc, Windmill by
migration.
