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
  `workflows/<client>/`. Name it a managed service in the proposal; the
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

## What never changes

- One audit surface: every model call is an `llm.complete` step.
- Run snapshots (prompts, outputs) live in the client's stack.
- Retention is a contract clause, not a config detail (`docs/observability.md`).
- A client's data never lands in this public repo — private ops repo only.
