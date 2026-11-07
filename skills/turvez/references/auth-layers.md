# Auth layers (OAuth and API keys)

When a client has many SaaS APIs, the hard part stops being the workflow and
becomes the credentials: OAuth dance, refresh, rotation, per-user connections,
and who can read the token. This is a **layer**, not an orchestrator. It does
not replace dagnats or Windmill.

## When to propose one

- 3+ APIs that require OAuth, or tokens that expire and must refresh.
- Per-user or per-tenant connections, not one shared key.
- The client's security review asks where tokens live and who can decrypt them.
- The same connection is reused by several automations or agents.

Not worth it for one static API key in `env/<client>.env`.

## Options

| Option | Shape | Fit |
|---|---|---|
| **Nango** | Elastic 2.0 (not OSI); Node + Postgres; 1,000+ APIs, HTTP proxy, Connect UI, Agent Sessions (MCP) | dagnats or Windmill calling APIs with OAuth; the access plane. Documented self-host is Enterprise (BYOC/self-managed) |
| **Corsair** | Apache-2.0; TypeScript SDK, REST-first, ~300 plugins, per-tool approvals, multi-tenant | the same role when the ELv2 terms block a use and a Node sidecar is acceptable |
| **ACI.dev** | Apache-2.0; Python platform, 600+ tools, per-user OAuth, permission boundaries, unified MCP server | the access plane with a ready tool catalog, when a Python service is acceptable — tool-calling infra, not a proxy |
| **Composio / Arcade** | SDK/CLI or agent runtime, mostly SaaS | agent tool calls with per-user auth, self-host is an enterprise arrangement |
| **Executor** | MIT, self-hosted MCP gateway + app host, per-connection policies | an app host beside the spine; not a credential layer for automations |
| **Vault / Infisical / OpenBao** | secret store (stateful service) | static secrets and audit trails, not OAuth refresh |

A static key in `env/<client>.env` needs none of this. The broker earns its
place at 3+ OAuth providers, or when the client's own agent has to call the
tools — the access plane (`docs/access-plane.md`), where the default is one
broker **per client** (`access.plane: client`); a shared instance is allowed
only as a pilot.

## Policy and state are different things

The ceiling (which providers, which kinds, which approval) lives in git in
`clients/<client>/catalog/policy.yaml`. What is connected, and who granted it,
is state in the broker. That split is what lets a client connect an app that is
already allowed without a commit — and still blocks a provider nobody declared.
Full model: `docs/access-plane.md` §2. Tool-by-tool comparison:
`docs/tooling.md`.

## Nango pattern

Nango stores the connection and injects the token; the recipe/flow never sees it.
Full guide: `docs/nango.md`; quick agent reference: `skills/turvez/references/nango.md`.

```
# dagnats: http.request
url: http://localhost:8088/proxy/<path>
headers: { "Connection-Id": "<client>", "Provider-Config-Key": "<provider>" }
auth_secret: "NANGO_SECRET_KEY"

# Windmill: a Go node (language: go) makes the request with net/http through the
# same proxy. Never the TS/Python SDK (docs/language-policy.md).
```

`NANGO_SECRET_KEY` is a secret in the runtime (env/age for dagnats, Variables
for Windmill). The connection id (`<client>`) is not a secret; the token is.

## Confirm before installing

Nango adds a service, a PostgreSQL database, an encryption key and a place
where client tokens live. Ask the operator:

1. Where does it run: your infra or the client's?
2. Who holds the encryption key and the backup?
3. Which client connections go in it, and who can read them?
4. Is the Elastic License 2.0 acceptable for this client?
5. What is the fallback if Nango is down: fail closed, or a static key?

Never install it automatically. `scripts/doctor.sh` only checks whether
`NANGO_URL` is configured and reachable; it does not install anything.

## Rules

- Tokens never appear in recipes, logs or chat.
- One connection per client and provider; no shared connection across clients.
- Rotate the Nango key like any other secret.
- Executor is for agents over MCP, not for unattended recipes without an MCP
  client. Do not confuse the two.
