# Triggers

A trigger starts a run without a human typing `run start`. Three kinds are
used here: webhook, cron, subject. Create exactly one per trigger.

```bash
# webhook
dagnats trigger create <client>.<wf> --webhook=/hooks/<client>/<event>
dagnats trigger create <client>.<wf> --webhook=/hooks/<client>/<event> --secret=<16+ chars>

# cron (5 fields, minute precision)
dagnats trigger create <client>.<wf> --cron="0 9 * * 1-5" --tz=America/Sao_Paulo
dagnats trigger create <client>.<wf> --cron="*/15 * * * *" --backfill

# NATS subject
dagnats trigger create <client>.<wf> --subject=<subject>
```

## Webhook

- `POST` only, body up to 1 MB. The body becomes `data` in the envelope
  `{trigger, source, workflow_id, timestamp, data}`; recipes open with
  `((.data // .) | …)`.
- Response is `200 ok` immediately. The run is asynchronous and the response
  carries no run id. Correlate through `trigger history` or the run input.
- Secret is optional but recommended. With `--secret` (or
  `DAGNATS_WEBHOOK_SECRET`), the caller must send
  `X-Signature-256: sha256=<hex hmac of the raw body>`; invalid → `401`.
- A disabled trigger still answers `200 ok` and does nothing (`skipped` in the
  firing history). Check `ENABLED` in `trigger list`.
- The path is served by the dagnats HTTP port, so production needs a public
  route (cloudflared/ingress) and a path containing `<client>`.

## Cron

- 5 fields: minute, hour, day of month, month, day of week. Minute precision.
- Timezone defaults to **UTC**; `--tz` stores an IANA zone on the trigger.
- `--backfill` runs windows missed while the engine was down.
- Preview before creating: `dagnats trigger test "0 9 * * 1-5" --count=3`
  (prints fire times in UTC).
- Fire now to test: `dagnats trigger fire <trigger-id>`.

## Lifecycle

```bash
dagnats trigger list
dagnats trigger enable|disable <trigger-id>
dagnats trigger delete <trigger-id>
dagnats trigger history <trigger-id>
```

`list`, `history` and `test` are read-only. **Propose** `create`, `update`,
`enable`, `disable` and `delete`, then wait: they change what runs unattended.

## Reading runs

`run list --workflow=` is an **exact** match, not a prefix. Use the full
`<client>.<wf>` name, or list all runs and filter the output.

Full CLI reference: `docs/agent-tools.md`. Alerting rules: `docs/alerting.md`.
