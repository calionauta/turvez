# Fieldbook: personal (você é o cliente)

> Automações pessoais usam o mesmo pipeline das de cliente:
> recipe em `workflows/personal/*.json`, segredos por nome, `validate` +
> `go test` + proposta de `register` — nada roda sem sua aprovação.
> Template: `workflows/_templates/FIELDBOOK.md`.

## 1. Contexto

- Dono, operador e aprovador: você.
- `ping.json`: recipe mínima sem side-effect (só `transform.json`),
  usada como smoke do namespace pessoal.

## 2. Sistemas e mapa de secrets (nomes apenas)

| Sistema | Uso | Secret NAME |
|---|---|---|
| Telegram | notify pessoal | `PERSONAL_TELEGRAM_BOT_TOKEN` + `PERSONAL_TELEGRAM_CHAT_ID` |
| Slack | notify alternativo | `PERSONAL_SLACK_WEBHOOK_URL` |
| LLM | `llm.complete` | `LLM_API_KEY` (default do handler) |

Valores em `env/personal.env` (local, gitignored) — ver `env/personal.env.example`.

## 3. Recipes e triggers

| Recipe | Trigger |
|---|---|
| `personal/ping` | manual (sem trigger — smoke) |

## 4. Operação

```bash
./scripts/register-workflows.sh personal
dagnats workflow validate workflows/personal/ping.json
dagnats run start "personal/ping" '{"hello":"world"}' --watch
dagnats run list --workflow=personal/
```

## 5. Decisões

- Namespace `personal/` em vez de pasta fora do padrão: herda tenancy,
  testes e scripts sem exceção (`go test ./workflows/` cobre o prefixo).

## 6. Adicionar sua primeira automação

```bash
cp workflows/_templates/webhook-pipeline.json workflows/personal/<nome>.json
# troque TODO-CLIENT → personal (nome, trigger id, webhook path)
# secrets por NOME em env/personal.env — nunca valores no JSON
dagnats workflow validate workflows/personal/<nome>.json
go test ./workflows/
# peça o register — nada roda sem sua aprovação (política da skill)
./scripts/register-workflows.sh personal
```
