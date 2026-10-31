# Fieldbook: ops (platform namespace, not a client)

> `ops/` holds platform automation (DLQ watchdog, etc.). No client secrets
> here — only `OPS_*` names. Per-client work lives in `clients/<client>/`.

## Recipes

| Recipe | Trigger | Id |
|---|---|---|
| `ops.dlq-watchdog` | cron every 5 min | `ops-dlq-watchdog-cron` |

## Operation

```bash
./scripts/register-workflows.sh ops
dagnats run list --workflow=ops/
```

Details in `docs/alerting.md` (layer 2: DLQ watchdog).
