# Tenancy evolution

No multi-tenant control plane, no big-bang. Three stages; each is a
convention + script change, never a handler or recipe-format redesign.

| Stage | Model | When |
|-------|--------|------|
| 1. Single server | One dagnats + one worker pool; isolation by client id prefix + env | Default (this repo as-is) |
| 2. Logical multi-tenant | Same process; stricter prefix checks in register scripts; optional per-client approval tokens | Several clients, still one host |
| 3. Physical multi-tenant | One stack per client (compose/host); same worker binary, different env | Client needs isolation, compliance, or a dedicated host |

## Stage 1 — single server (default)

- Recipes: `clients/<client-id>/automation/dagnats/*.json`, names `<client-id>.<wf>`.
- Secrets: `env/<client-id>.env` (+ `env/<client-id>.env.example`
  committed); worker loads all of them (single `.env` or several
  `DOTENV_FILE`s merged — env wins on conflict).
- Triggers: ids `<client-id>-…`, webhook paths contain `<client-id>`.
- Ops: shared `OPS_APPROVAL_TOKEN`; console shared.

## Stage 2 — logical multi-tenant (same host)

Already wired in this repo:

- `scripts/register-workflows.sh [client-id]` aborts on namespace
  violations (workflow name must match its folder prefix), so a
  misnamespaced recipe never reaches the shared engine. Run it per
  client in CI: `./scripts/register-workflows.sh acme`.
- `go test ./clients/` enforces the same prefix rule plus trigger
  id/path conventions for every client folder (except `_template`).
- Optional per-client approval tokens: run one ops-HTTP shim per
  client with distinct `OPS_APPROVAL_TOKEN` values (separate
  `OPS_HTTP_ADDR` ports, or separate processes with
  `CLIENT_ID` set). Votes for client A can never approve client B's
  runs — the token check happens before any dagnats call.
- Operate per client via prefix filters:
  `dagnats run list --workflow=acme.<wf>`,
  `dagnats trigger list` (grep `acme-`).

## Stage 3 — physical multi-tenant (one stack per client)

Same worker binary, different env — full runbook in
`docs/isolating-a-client.md`. Summary:

1. New host: `dagnats serve` (same pinned version) + worker with
   `CLIENT_ID=<client-id>` (loads only `env/<client-id>.env`) + the
   client's age file if used.
2. `./scripts/register-workflows.sh <client-id>` (prefix-checked).
3. Recreate only that client's triggers; smoke + webhook test.
4. Old host: `trigger disable` (rollback-safe) → later `delete`.
5. Repoint SaaS webhooks; run history stays on the old NATS.

## Promotion without redesign

What never changes across stages: handler task names and semantics,
recipe JSON format (`metadata` static + input dynamic, secret names
only), trigger declaration shape, ops-HTTP contract. A stage 3 host
runs the identical container image with a different env file —
`docker-compose.yml` already parameterizes this (`CLIENT_ID`,
`SECRETS_AGE_FILE`).
