# Fieldbook: example-client

> Exemplo preenchido do template `workflows/_templates/FIELDBOOK.md`.
> Nomes de secrets sim, valores nunca.

## 1. Contexto

- Cliente fictício para validar o runtime.
- Recipes: `hubspot-lead-to-slack` (webhook → HubSpot → Slack), `invoice-paid-route` (split VIP/standard), `nightly-sync` (cron 02:00 UTC), `approval-refund` (gate humano 48h).
- Fora de escopo: scripts arbitrários, SSH, deploy — ver `docs/script-execution.md`.

## 2. Contatos e decisão

| Papel | Quem | Canal |
|---|---|---|
| Dono no cliente | — (exemplo) | — |
| Operador | — | `OPS_SLACK_WEBHOOK_URL` |
| Nós (FDE) | solo-operator | console `:8080` |

## 3. Sistemas e mapa de secrets (nomes apenas)

| Sistema | Uso | Secret NAME |
|---|---|---|
| HubSpot | GET contacts | `ACME_HUBSPOT_TOKEN` |
| Slack | notify | `ACME_SLACK_WEBHOOK_URL` |
| Billing | refund/invoice | `ACME_BILLING_TOKEN`, `ACME_REFUND_API_TOKEN` |
| Warehouse | nightly push | `ACME_WAREHOUSE_TOKEN` |

Valores em `env/example-client.env` (local, gitignored) — ver `env/example-client.env.example`.

## 4. Recipes e triggers

| Recipe | Trigger | Id |
|---|---|---|
| `example-client/hubspot-lead-to-slack` | webhook `/hooks/example-client/lead` | `example-client-lead-webhook` |
| `example-client/nightly-sync` | cron `0 2 * * * UTC` (embedded) | `example-client-nightly-sync-cron` |
| `example-client/approval-refund` | manual + `approval` nativo | — |

## 5. Operação

```bash
./scripts/register-workflows.sh example-client
dagnats run start "example-client/hubspot-lead-to-slack" '{"lead_id":"cutover-1"}' --watch
dagnats run list --workflow=example-client/
./scripts/smoke-test.sh
```

## 6. Decisões

- Refund usa `type: approval` nativo (digna de 48h, sem ocupar worker) em vez de `request.approval` por chunks.
- Split VIP usa `skip_if` (top-level) em vez de `condition` — comparação simples, sem task extra.
- Nenhum handler custom: tudo coube em `http.request` + `transform.json` + `skip_if` + `map`.
- Reutilizável: (nada ainda — quando um padrão repetir, vira template em `_templates/` ou proposta de handler; productize é o que o próximo cliente ganha de graça).

## 7. Extrair

Ver `docs/isolating-a-client.md` — copiar esta pasta + `env.age`, registrar só este prefixo.
