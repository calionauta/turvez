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
| **Nango** | self-hosted, ELv2, Node + Postgres, 1,000+ APIs, HTTP proxy | dagnats or Windmill calling APIs with OAuth |
| **Composio / Arcade** | SDK/CLI or agent runtime, mostly SaaS | agent tool calls with per-user auth |
| **Executor** | self-hosted MCP gateway, per-connection policies | an agent (Claude Code, Cursor) using many accounts |
| **Vault / Infisical** | secret store | static secrets, not OAuth refresh |

## Nango pattern

Nango stores the connection and injects the token; the recipe never sees it.

```
# dagnats: http.request
url: http://nango:3003/proxy/<provider>/<path>
headers: { "Connection-Id": "<client>-<provider>", "Provider-Config-Key": "<provider>" }
auth_secret: "NANGO_SECRET_KEY"

# Windmill: a script step does the same HTTP call
```

`NANGO_SECRET_KEY` is a secret in the runtime (env/age for dagnats, Variables
for Windmill). The connection id is not a secret; the token is.

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
