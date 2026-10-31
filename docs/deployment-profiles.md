# Deployment profiles: who owns what

Three ways to run this repo for a client. The code is identical; ownership,
place and contract change. Pick one **per engagement** and write it down
before the first deploy.

| | A. Embedded | B. Managed | C. Multi-tenant |
|---|---|---|---|
| Runs on | client's server | your server, one stack per client | your server, one stack for many |
| Code lives in | client's ops repo | client's ops repo (yours to operate) | one repo, tenant-scoped data |
| Credentials | client's secret store | yours, per client (`env/<client>.env` / age) | virtual keys per tenant |
| Data boundary | stays with client | crosses to you — needs contract + DPA | crosses; needs tenancy isolation |
| Client can run it | yes | no (you operate) | yes, self-service |
| Gateway | only if 2+ callers | one per deployment, virtual key per client | required |
| Exit plan | repo handover | repo + secrets + run export | data export per tenant |

## Choosing

- **Default for SMB consulting: B.** One server, fast delivery, per-client
  isolation via `env/<client>.env`, `secrets/<client>/`, and separate
  `clients/<client>/`. Name it a managed service in the proposal; the
  client pays for operation, not just build.
- **Choose A** when the client requires data residency, has an internal
  platform team, or will not sign a DPA that lets data leave their
  infrastructure. Higher setup cost, lowest boundary risk.
- **Choose C only if you productize.** "Many clients" is not multi-tenant;
  one deployment serving many is. That is a product decision with billing,
  quotas and isolation consequences.

## Gateway placement (all profiles)

The gateway follows the deployment, it does not define it:

- **A:** install in the client's infra, same host as the automations. Skip it
  unless the client has other model callers (their own agents, scripts, n8n).
- **B:** one gateway in your stack, **virtual key per client**, so cost and
  volume are attributable. Same stack as the automations, never a separate
  service.
- **C:** required, with per-tenant virtual keys and limits.

The repo is agnostic: `llm.complete` needs a `base_url` (or `LLM_BASE_URL`).
Whether that URL is a gateway, a proxy or a provider endpoint is deployment
configuration, not a code branch.

## Engine implications per profile

The engine (`client.yaml`) changes what "your server" has to carry:

| | A. Embedded | B. Managed | C. Multi-tenant |
|---|---|---|---|
| dagnats | 1 binary + 1 container | one stack per client, 1 binary each | one stack, tenant-scoped names |
| Windmill | needs Postgres + workers in the client's infra | one workspace per client on your instance | one instance, workspace per tenant |
| Windmill license | AGPL core; EE for SSO/audit/PR gating | same, your cost | same |

**Windmill licence and ops, stated plainly** (it is the cost that decides the
engine, not a footnote):

- **AGPL core.** Self-hosting obliges you to honour AGPL for the server you
  operate. If the client's contract forbids AGPL anywhere in the stack, that
  alone rules Windmill out — check it before the pilot, not after.
- **EE for governance.** SSO, audit logs and PR-gated tests are Enterprise
  features. "Per-client audit trail" or "our SSO" turns into a licence line.
- **Postgres + workers.** Two stateful services to back up, upgrade and monitor,
  against dagnats' single binary. A backup that is not tested is not a backup;
  add it to the runbook before promising RPO.
- **No native critical alerts on free.** Alert from a flow step (HTTP or SMTP)
  **plus an external dead-man switch**, same rule as `docs/alerting.md`. Do not
  assume the platform pages you.

## When a client needs its own instance

"One shared instance serves everyone" is the right default for SMB work, and it
has nothing to do with the engine. A **dedicated instance** is justified by one
of these, and only these:

- **Regulation / data residency** — the contract names where data may live.
- **The client owns the infrastructure** — they will not let data leave.
- **Noisy neighbour** — one client's volume degrades another's latency.
- **Per-client upgrade window** — a client needs a pinned or independent
  release cadence.
- **A contract clause requiring a separate database.**

Everything else is over-engineering. Isolation is a **deployment profile**
(A vs B), recorded in `client.yaml` as `runtime: shared|dedicated` — not a repo
fork and not an engine change. The move procedure is
`docs/isolating-a-client.md`; it is a copy of definitions, not a history
migration.

## What never changes

- One audit surface: every model call is an `llm.complete` step.
- Run snapshots (prompts, outputs) live in the client's stack.
- Retention is a contract clause, not a config detail (`docs/observability.md`).
- A client's data never lands in this public repo — private ops repo only.
