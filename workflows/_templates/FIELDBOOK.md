# Fieldbook: TODO-CLIENT

> Copy to `turvez-ops/workflows/TODO-CLIENT/README.md` when opening the client
> (private ops repo — never in the public template). Git is source of truth;
> KV is deploy target (same rule as recipes).
> Secret names yes, values never (`go test ./workflows/` fails if one leaks).
> Diagnosis seed from degustia (if an Encorpado exists): `dores_mapeadas` → §1;
> systems mentioned → §3 (NAMES only); automations from chaining 2.2b →
> candidate recipes in §4. The reverse works too: fieldbook + fresh transcript
> feed the next diagnosis.

## 1. Context (5 lines)

- What the client does:
- What we automate here (1 sentence per recipe):
- Out of scope (what we do NOT do):

## 2. Contacts and decisions

| Role | Who | Channel |
|---|---|---|
| Owner at client (approves) | | |
| Day-to-day operator | | |
| Us (FDE) | | |

How they decide: (e.g. Slack #sales, 4h business-hours SLA)

## 3. Systems and secret map (names only)

| System | Use | Secret NAME | Value owner |
|---|---|---|---|
| HubSpot | deals stage=qualified | `TODO_HUBSPOT_TOKEN` | client via 1Password |
| Slack #sales | notify | `TODO_SLACK_WEBHOOK_URL` | us via age |

Rule: value only in `env/TODO-CLIENT.env` (local) or `secrets/TODO-CLIENT/env.age`.

## 4. Recipes and triggers

| Recipe | Trigger | Trigger id |
|---|---|---|
| `TODO-CLIENT/hubspot-qualified-to-slack` | webhook `/hooks/TODO-CLIENT/lead` | `TODO-CLIENT-lead-webhook` |
| `TODO-CLIENT/nightly-sync` | cron `0 2 * * * UTC` | `TODO-CLIENT-nightly-sync-cron` |

Conventions: workflow `<client>.<wf>`, trigger `<client>-…`, webhook contains `<client>`.
The separator lives in `naming.json` at the template root; change it there, then run
`scripts/rename-workflows.sh --apply`.

## 5. Operation (copy-paste)

```bash
CLIENT=TODO-CLIENT
dagnats workflow validate workflows/$CLIENT/*.json
./scripts/register-workflows.sh $CLIENT
dagnats run start "$CLIENT/<wf>" '{"lead_id":"test-1"}' --watch
dagnats run list --workflow=$CLIENT/
dagnats dlq list --run=<run-id> && dagnats dlq replay --run=<run-id>
dagnats trigger disable <client>-<id>  # rollback: enable back
```

Alerts: `on_failure` → notify (layer 1) + `ops.dlq-watchdog` (layer 2) + external `GET /health` probe (layer 3). See `docs/alerting.md`.

## 6. Decisions (log, 1 line each)

- 2026-10-24: approval threshold = 10000 because… (who asked)
- …: new handler? no — `transform.json` + `skip_if` covered it
- …: what became reusable? (new template in `_templates/`? pattern that
  becomes a handler? — productize: what this client taught that the next one gets for free)

## 7. Extract (stage 3, if needed)

See `docs/isolating-a-client.md`. Summary: copy `workflows/TODO-CLIENT/` + `env.age`, `register` on the new host, repoint webhooks, `disable` on the old one.
