# Fieldbook: personal (you are the client)

> Personal automations use the same pipeline as client ones:
> recipe in `workflows/personal/*.json`, secrets by name, `validate` +
> `go test` + proposed `register` — nothing runs without your approval.
> Template: `workflows/_templates/FIELDBOOK.md`.

## 1. Context

- Owner, operator, and approver: you.
- `ping.json`: minimal recipe with no side effects (only `transform.json`),
  used as smoke for the personal namespace.

## 2. Systems and secret map (names only)

| System | Use | Secret NAME |
|---|---|---|
| Telegram | personal notify | `PERSONAL_TELEGRAM_BOT_TOKEN` + `PERSONAL_TELEGRAM_CHAT_ID` |
| Slack | alternative notify | `PERSONAL_SLACK_WEBHOOK_URL` |
| LLM | `llm.complete` | `LLM_API_KEY` (handler default) |

Values in `env/personal.env` (local, gitignored) — see `env/personal.env.example`.

## 3. Recipes and triggers

| Recipe | Trigger |
|---|---|
| `personal/ping` | manual (no trigger — smoke) |

## 4. Operation

```bash
./scripts/register-workflows.sh personal
dagnats workflow validate workflows/personal/ping.json
dagnats run start "personal/ping" '{"hello":"world"}' --watch
dagnats run list --workflow=personal/
```

## 5. Decisions

- Namespace `personal/` instead of an off-pattern folder: inherits tenancy,
  tests, and scripts with no exceptions (`go test ./workflows/` covers the prefix).

## 6. Adding your first automation

```bash
cp workflows/_templates/webhook-pipeline.json workflows/personal/<name>.json
# replace TODO-CLIENT → personal (name, trigger id, webhook path)
# secrets by NAME in env/personal.env — never values in JSON
dagnats workflow validate workflows/personal/<name>.json
go test ./workflows/
# request the register — nothing runs without your approval (skill policy)
./scripts/register-workflows.sh personal
```