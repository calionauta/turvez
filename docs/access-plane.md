# The access plane: the client's own agents calling authenticated tools

Turvez has two planes. **Control** is git (`clients/<client>/`, `client.yaml`,
fieldbook, evals) and belongs to the operator and the authoring agent.
**Execution** is Windmill / dagnats / Dagu and runs unattended.

The consultancy sells a third thing: **the client's own AI agents calling the
client's authenticated tools**, through a broker that holds the credential the
client authorised. That is neither control nor execution. It has a property
neither has — **per-request identity** — and a security property neither has:
**the credential is the client's asset, not ours.**

Left unnamed, it leaks into both planes: one more Genkit toolset, one more Nango
endpoint, one more ad-hoc route. Three places expose capability, and no single
place answers "what can this client's agent do?". This doc names the plane and
wires it into the isolation machinery the engines already have.

## 1. Why client-hosted is the default

The decisive external artifact is the IETF draft **Credential Broker 4 Agents
(CB4A)**, Kenneth G. Hartman, March 2026. Its core rule: the component that
**decides** (policy decision point) must never touch credentials, and the one
that **delivers** credentials must never decide.

It defines three modes: **A** the proxy injects and the agent never sees a
credential; **B** the broker mints a narrow, short-lived token; **C** hand over
the long-lived credential and revoke later — which the draft calls the weakest
and does not recommend.

Its stated motivation is the failure mode this plane is designed against: when
AI infrastructure concentrates long-lived credentials in one process or
configuration, compromising that process yields catastrophic access. One shared
broker for N clients is one blast radius for N.

Derived guidance, consistent with the vendors' own 2026 posture:

1. **Concentration is the risk, not the count.**
2. **BYOC is the recommended self-host shape.** Nango positions "we deploy into
   your account" as recommended over self-managed; both are Enterprise.
3. **Data residency beats convenience.** Tokens inside the client's perimeter
   take the cross-border transfer question out of the DPA.
4. **The agent must never hold the credential.** Only the tool call crosses.

## 2. Policy in git, state in the broker

The single split that unlocks self-serve without giving up review:

| | Lives in | Changes by | Example |
| --- | --- | --- | --- |
| **Policy** (ceiling) | `catalog/policy.yaml`, git | commit + review | "acme may use hubspot, `read` + `write`, approval on write" |
| **Connections** (state) | broker DB | the client, in a UI | "acme connected hubspot 2026-11-06" |
| **Grants** (state) | broker DB | the client, in a UI | scope `contacts.read`, approved by a named person, expiring |

`catalog/policy.yaml` is a **ceiling, not an inventory**. Nothing in git lists
individual connections, so **connecting an app that is already allowed needs no
PR**. The effective tool catalog is derived: `policy ∩ connections ∩ grants`.

A **grant** is `tenant × tool × scope × expiry × approval-policy`, and can only
exist inside the ceiling. Self-serve means create/revoke a grant. Revocation is
a row delete, not a deploy — strictly safer than the git-only model, where
revoking waits for the next push.

## 3. Topology

| Profile | Broker runs on | Who holds the licence and pays | Default for |
| --- | --- | --- | --- |
| **A. Client-hosted** (`plane: client`) | client infra, under their account | the client | **default**, mandatory for `sensitive` |
| **B. Client cloud** (`plane: cloud`) | vendor Cloud, under the **client's own account and card** | the client | only if they ask and pay |
| **C. Consultancy-hosted shared** (`plane: shared`) | our host, one broker for all clients | us | **pilots only**, `data_class: normal` |

Three rules, in order of precedence:

1. **`sensitive` → A, no exception.** The consultancy does not become the
   custodian of another company's refresh tokens unless that is written into the
   contract and priced.
2. **B is only B while the account and the card are the client's.** If we open
   the cloud account we are the custodian, the profile is C, and it must not be
   sold as B.
3. **C is legitimate, not forbidden — but it is a pilot.** It exists so a first
   engagement with `data_class: normal` is not killed by the cost of an instance
   per client. It is time-boxed: it converts to A at the first renewal, or the
   moment the client becomes `sensitive`.

`runtime` and `plane` must agree: `runtime: dedicated` requires
`access.plane: client`, and a `sensitive` client on a shared broker is a build
failure, not a note in a fieldbook.

## 4. Identity: `org` or `per-user`

Decided per client in the fieldbook, before the first tool is exposed:

- `identity: org` — one connection per provider; the agent acts as the company.
  One grant per provider, simple onboarding, simple revocation.
- `identity: per-user` — each staff member connects their own account and the
  agent acts as that person. Closer to what people expect from an agent, at the
  cost of N grants, N onboarding flows and N revocations.

The naming already supports both: `acme` and `acme:fulano`. Start `org` where it
is enough; the data does not change shape when a client upgrades.

## 5. Invariants

Each one is either a test in this repo or a rule the broker enforces; a rule
that is neither is a wish.

| # | Invariant | Enforced by |
| --- | --- | --- |
| 1 | Every tool name is `<tenant>.<provider>.<op>` | `internal/access.ToolName` + its unit test |
| 2 | `write`/`destructive` requires an approval policy | `Policy.Validate` (P0) |
| 3 | A provider the client uses must be in the ceiling | `access.Coverage` over connections + recipes (P0) |
| 4 | A grant outside the ceiling is refused server-side, not hidden in the UI | broker (P2) |
| 5 | The credential never appears in a recipe, log, agent context or tool response | the secret guard + broker design |
| 6 | Revocation stops the next tool call | broker (P2) |
| 7 | A `sensitive` client cannot run a shared broker | `Config.Validate` (P0) |
| 8 | Every grant and tool call is audited: who, what, when, tenant | broker (P2) |
| 9 | A `cloud` plane names the client's own account | `Config.Validate` (P0) |

## 6. Declaring it

`clients/<client>/client.yaml`:

```yaml
data_class: normal          # normal | sensitive
access:
  enabled: true
  plane: client             # client | cloud | shared
  surface: mcp              # mcp | none
  identity: org             # org | per-user
  policy: catalog/policy.yaml
  vendor_account: client    # client | consultancy — required when plane: cloud
```

`clients/<client>/catalog/policy.yaml`:

```yaml
providers:
  hubspot:
    kinds: [read, write]
    approval: writes        # none | writes | all
limits:
  max_grants: 10
  scopes_ceiling: [".read", ".write"]
```

The scaffold ships the block **off** (`access.enabled: false`) and an empty
ceiling: a new client opts in, it never inherits an exposure.

## 7. What is not implemented yet

Status: **P0 and P1 done** (contract, ceiling, tests, docs, per-client broker
scoping). P2 — grants, revocation and audit — needs a running broker; this doc
does not claim it exists.

| Phase | Work | Gate |
| --- | --- | --- |
| **P0 — name the plane** | `access:` block, ceiling schema, `internal/access`, the contract tests, this doc, `docs/tooling.md` | `go test ./...` fails a write with `approval: none`, a `sensitive` client off `plane: client`, a `cloud` plane on our account, a provider used but not declared |
| **P1 — per-client broker** ✅ | `NANGO_CLIENT_ID` scopes the Compose project, containers, volume and `turvez.client` label; `setup-nango.sh` refuses `--up` without it (or an explicit `--shared`); `extract-client.py` prints the access-plane cutover; `verify-isolation.sh` reports the plane | a `sensitive` client with a shared broker fails `go test`; two stacks cannot share a volume or a key |
| **P2 — self-serve grants** | Connect UI for account linking, grant creation bounded by the ceiling, revocation endpoint, audit events | connecting an already-allowed provider needs zero repo changes; a provider outside the ceiling is refused server-side |
| **P3 — portability** | adapter contract (`connection`, `grant`, `toolref`, `audit`), one adapter per backend | switching backend changes the adapter only — policy, recipes and evals untouched |
| **P4 — evals** | tool-selection cases: right tool, no scope creep, refusal when a grant is missing | the eval gate |

## 8. Portability: what actually carries over

| Concept | Nango | Corsair | Executor |
| --- | --- | --- | --- |
| tenant | `connection_id` + tags | tenant id | org + app + account |
| provider | `provider_config_key` | plugin | provider requirement |
| tool ref | catalog tool | plugin operation | `app.tool` path |
| grant | connection + tags | connection + permissions | linked account + approval |
| revocation | delete connection | delete connection | unlink account |

Portable: tenant, provider slug, tool ref, kind, approval, grant lifecycle,
audit events. **Not portable**: each backend's own consent/callback surface and
its internal storage model.

ACI.dev is a fourth candidate for the same role, with a different unit: it
stores an end-user's linked account plus a permission boundary rather than a
connection per tenant. Mapping it is a real adapter, not a rename — which is the
point of separating the contract from the backend. Its shape and the honest
comparison: `docs/tooling.md`.

The promise this doc makes is deliberately narrow: switching backend changes the
**adapter**; it does not touch the ceiling, recipes, evals or tenant naming. Do
not build the adapter until a second backend exists — one adapter is a
hypothetical, two is a pattern. When each backend earns a place:
`docs/tooling.md`.

## 9. Open decisions (human, not code)

1. **Identity**: `org` first with an upgrade path, or `per-user` from day one
   for clients whose staff will drive the agent directly?
2. **The licence** when the broker is client-hosted: the client buys Enterprise,
   or the community image runs unsupported on their infra with that risk stated
   in the fieldbook?
3. **Which providers are pre-seeded**: only the ones in the first automation, or
   the full set the client may want later? Each needs OAuth app credentials,
   which is the real ceiling.
4. **Custody at offboarding**: who owns the grant, and what happens to the
   connections when the engagement ends? This belongs in the contract.

## References

- IETF draft, *Credential Broker 4 Agents (CB4A)*, K. G. Hartman, 2026 —
  PDP/CDP separation, models A/B/C, DPoP (RFC 9449), SPIFFE/SPIRE.
- agentgateway, *Credential Injection Patterns for AI Agents*, 2026 — a
  practical implementation of model A with a token vault.
- Nango docs, *Self-host* (BYOC vs Self-Managed, both Enterprise), *Agent
  Sessions*, *Limits*.
- `docs/nango.md` — the credential-layer contract this plane builds on.
- `docs/tooling.md` — when each broker/gateway earns a place.
