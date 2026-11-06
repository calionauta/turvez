# Tooling: what each layer is for, and when it earns a place

`docs/access-plane.md` decides **what** the access plane is. This doc decides
**which piece** provides it, and — more usefully — when a piece is not worth
installing.

None of these is an engine. Each one sits in front of the client's Windmill /
dagnats / Dagu run, or beside it as the credential and tool layer. Adding one
never relocates the source of truth away from git, and never becomes a second
orchestrator.

Two rules keep this list honest:

1. **A tool enters by a trigger, not by being interesting.** Each row names the
   condition that would make it the right answer; before that condition exists
   it is a service to monitor with no traffic.
2. **Weight saved is usually negative.** The lighter answer is often "do nothing
   extra": a client with one or two static-key APIs needs `env/` + age, which
   already ships, not a broker.

Facts below were read at the source (vendor docs, licence files, repo) on
2026-11-06. Re-read them before buying; licence and self-host terms move.

## The table

| Tool | Licence | What it actually is | What it does not do | Enters when |
| --- | --- | --- | --- | --- |
| **Nango** | Elastic 2.0 (not OSI) | OAuth broker: 1,000+ provider configs, token refresh with a distributed lock, HTTP proxy, Connect UI, Agent Sessions (an MCP endpoint per tenant) | Model routing; it is not an LLM gateway | A client has OAuth providers, or its agents call tools. **Today's default**, already in `docs/nango.md` |
| **Corsair** | Apache-2.0 | TypeScript broker: ~300 provider plugins, REST-first SDK, multi-tenancy, per-tool permissions with approval, signed webhooks | Guarantee a documented refresh lock; its OAuth consent flow runs through the vendor's Hub | The ELv2 terms block a use, and a Node sidecar is acceptable. Closest drop-in for the Nango *role* |
| **agentgateway** | Apache-2.0 | MCP/A2A data plane: CEL policy, credential injection on egress (CB4A model A), guardrails, OTel | OAuth consent and refresh — it injects from a vault, it does not obtain grants | **We host MCP servers ourselves** and need per-call policy plus injection |
| **Executor** | MIT | Agent-first app host: MCP gateway, "accounts" per app, tool approvals, 1 container + embedded Postgres | Stay out of the source of truth; durable workflows are only partly documented | Never as a broker. Only if we accept it as an app host beside the spine |
| **Bifrost** | Apache-2.0 | LLM gateway: provider unification, failover, semantic cache, virtual keys and budgets | SaaS credentials; it is the model path, not the tool path | The LLM key is **ours** (resale/budget per end user) or failover has an SLA |
| **OpenBao** | MPL-2.0 | Secrets service: KV, dynamic DB credentials, PKI, audit devices, leases | At-rest encryption for a repo; it is stateful and must be unsealed | 3+ operators touching client secrets, a regulated client demanding an access audit trail, or agentgateway's vault |
| **GoAI** | dependency | The only model path: one Go interface over providers, in-process | Anything above a single call — no queue, no budget | Never a decision: it is already the transport behind `llm.complete` |

## Why the answer today is "Nango, and nothing new"

- **The role is already filled.** `docs/nango.md` is the contract: provider
  config, `Connection-Id = <client>`, HTTP proxy, and Agent Sessions for the
  access plane. A second broker would duplicate the same role against the same
  connections.
- **The client owns the LLM key.** That deletes three quarters of Bifrost's
  value (virtual keys, per-client budget, billing) and leaves provider
  unification, which GoAI already does in-process.
- **There is no MCP traffic between processes yet.** agentgateway has nothing to
  police until we serve MCP servers we wrote; Genkit runs the agent inside our
  service and calls tools over HTTP.
- **A vault would add a failure domain.** `internal/secret/age.go` decrypts at
  worker start and never asks again. OpenBao sealed means every automation of
  every client stops, to solve audit and dynamic credentials we do not yet need.

## Two substitutions worth knowing

**Corsair instead of Nango** — same role, Apache-2.0, REST + MCP + approvals.
The cost is a Node sidecar and an OAuth consent flow through the vendor's Hub.
The open question to test before adopting: **concurrent refresh**. Fire ten
parallel calls with an expired token and see whether the rotating refresh
survives; if it does not, a mutex in front of it is your code, not theirs.
Switch the adapter, not the ceiling (docs/access-plane.md §8).

**Go instead of a broker** — for one or two OAuth providers, `golang.org/x/oauth2`
plus a `pg_advisory_lock` around the refresh is ~100 lines, no service, no
licence. The broker earns its place at three or more providers, or the moment a
client's agent needs a per-tenant tool surface.

## Adoption order if the product grows

```
age (have)  →  Nango per client (P1)  →  grants + audit (P2)
            →  adapter when a second backend exists (P3)
            →  agentgateway, only with self-hosted MCP servers
            →  OpenBao, only with an operator team or a regulated client
```

Bifrost is not on the path: it appears only if the LLM key becomes ours.
Executor is not on the path: it is a different product shape (an app host), not
a layer of this one.

## The test for any future addition

1. Which **plane** does it serve — control, execution, or access?
2. Which **existing** piece does it replace? (If none, it is additive weight.)
3. Where does the **source of truth** move to? (If it is not still git, no.)
4. What happens to **N-1 clients** the day it is down?
5. Does it make the **security review** easier to answer, not just the demo?
