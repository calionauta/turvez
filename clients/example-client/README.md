# Fieldbook: example-client

> Worked example of the `clients/_template/FIELDBOOK.md` template.
> Secret names yes, values never.

## 1. Context

- Fictional client to validate the runtime.
- Recipes: `hubspot-lead-to-slack` (webhook → HubSpot → Slack), `invoice-paid-route` (VIP/standard split), `nightly-sync` (cron 02:00 UTC), `approval-refund` (48h human gate).
- Out of scope: arbitrary scripts, SSH, deploys — see `docs/script-execution.md`.

## 2. Contacts and decisions

| Role | Who | Channel |
|---|---|---|
| Owner at client | — (example) | — |
| Operator | — | `OPS_SLACK_WEBHOOK_URL` |
| Us (FDE) | operator (you) | console `:8080` |

## 3. Systems and secret map (names only)

| System | Use | Secret NAME |
|---|---|---|
| HubSpot | GET contacts | `ACME_HUBSPOT_TOKEN` |
| Slack | notify | `ACME_SLACK_WEBHOOK_URL` |
| Billing | refund/invoice | `ACME_BILLING_TOKEN`, `ACME_REFUND_API_TOKEN` |
| Warehouse | nightly push | `ACME_WAREHOUSE_TOKEN` |

Values in `env/example-client.env` (local, gitignored) — see `env/example-client.env.example`.

## 4. Recipes and triggers

| Recipe | Trigger | Id |
|---|---|---|
| `example-client.hubspot-lead-to-slack` | webhook `/hooks/example-client/lead` | `example-client-lead-webhook` |
| `example-client/nightly-sync` | cron `0 2 * * * UTC` (embedded) | `example-client-nightly-sync-cron` |
| `example-client/approval-refund` | manual + native `approval` | — |

## 5. Operation

```bash
./scripts/register-workflows.sh example-client
dagnats run start "example-client.hubspot-lead-to-slack" '{"lead_id":"cutover-1"}' --watch
dagnats run list --workflow=example-client/
./scripts/smoke-test.sh
```

## 6. Decisions

- Refund uses native `type: approval` (durable 48h, no worker slot) instead of chunked `request.approval`.
- VIP split uses `skip_if` (top-level) instead of `condition` — simple comparison, no extra task.
- No custom handler: everything fit in `http.request` + `transform.json` + `skip_if` + `map`.
- Reusable: (nothing yet — when a pattern repeats, it becomes a `clients/_template/` template or a handler proposal; productize is what the next client gets for free).

## 7. Extract

See `docs/isolating-a-client.md` — copy this folder + `env.age`, register only this prefix.
