# Fieldbook: TODO-CLIENT

> Copie para `workflows/TODO-CLIENT/README.md` ao abrir o cliente.
> Git é source of truth; KV é deploy target (mesma regra das recipes).
> Nomes de secrets sim, valores nunca (`go test ./workflows/` quebra se vazar).
> Seed de diagnóstico degustia (se houver Encorpado): `dores_mapeadas` → §1;
> sistemas citados → §3 (só NOMES); automações do encadeamento 2.2b →
> recipes candidatas na §4. Direção oposta vale também: fieldbook + transcript
> novo alimentam o próximo diagnóstico.

## 1. Contexto (5 linhas)

- O que o cliente faz:
- O que automatizamos aqui (1 frase por recipe):
- Fora de escopo (o que NÃO fazemos):

## 2. Contatos e decisão

| Papel | Quem | Canal |
|---|---|---|
| Dono no cliente (aprova) | | |
| Operador dia-a-dia | | |
| Nós (FDE) | | |

Como decidem: (ex. Slack #comercial, SLA 4h úteis)

## 3. Sistemas e mapa de secrets (nomes apenas)

| Sistema | Uso | Secret NAME | Dono do valor |
|---|---|---|---|
| HubSpot | deals stage=qualified | `TODO_HUBSPOT_TOKEN` | cliente via 1Password |
| Slack #comercial | notify | `TODO_SLACK_WEBHOOK_URL` | nós via age |

Regra: valor só em `env/TODO-CLIENT.env` (local) ou `secrets/TODO-CLIENT/env.age`.

## 4. Recipes e triggers

| Recipe | Trigger | Id do trigger |
|---|---|---|
| `TODO-CLIENT/hubspot-qualified-to-slack` | webhook `/hooks/TODO-CLIENT/lead` | `TODO-CLIENT-lead-webhook` |
| `TODO-CLIENT/nightly-sync` | cron `0 2 * * * UTC` | `TODO-CLIENT-nightly-sync-cron` |

Convenções: workflow `<client>/…`, trigger `<client>-…`, webhook contém `<client>`.

## 5. Operação (copy-paste)

```bash
CLIENT=TODO-CLIENT
dagnats workflow validate workflows/$CLIENT/*.json
./scripts/register-workflows.sh $CLIENT
dagnats run start "$CLIENT/<wf>" '{"lead_id":"teste-1"}' --watch
dagnats run list --workflow=$CLIENT/
dagnats dlq list --run=<run-id> && dagnats dlq replay --run=<run-id>
dagnats trigger disable <client>-<id>  # rollback: enable de volta
```

Alertas: `on_failure` → notify (camada 1) + `ops/dlq-watchdog` (camada 2) + probe externa `GET /health` (camada 3). Ver `docs/alerting.md`.

## 6. Decisões (log, 1 linha cada)

- 2026-10-24: threshold aprovação = 10000 porque… (quem pediu)
- …: handler novo? não — coube em `transform.json` + `skip_if`
- …: o que virou reutilizável? (template novo em `_templates/`? padrão que
  vira handler? — productize: o que este cliente ensinou que o próximo ganha de graça)

## 7. Extrair (stage 3, se precisar)

Ver `docs/isolating-a-client.md`. Resumo: copiar `workflows/TODO-CLIENT/` + `env.age`, `register` no host novo, repoint webhooks, `disable` no antigo.
