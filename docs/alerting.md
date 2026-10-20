# Alerting

The console shows failures; nothing pages you. This repo wires three
layers so a solo operator hears about breakage without watching the UI.

## 1. In-workflow failure pages (per client recipe)

Client workflows set `on_failure` to a notify step with a short payload:
client id, workflow name, run id, error summary.

```json
{
  "id": "approve",
  "type": "approval",
  "on_failure": "notify-rejected",
  "config": {"timeout": 172800000000000, "subject": "approvals.acme.refund"}
},
{
  "id": "notify-rejected",
  "task": "notify.slack",
  "type": "normal",
  "timeout": 30000000000,
  "metadata": {
    "webhook_secret": "ACME_SLACK_WEBHOOK_URL",
    "message": "A refund request was rejected — see the run in dagnats console for details."
  }
}
```

(`workflows/example-client/approval-refund.json` — the reference example.)

Two engine facts shape this pattern (verified at v0.0.30):

- An `on_failure` step receives `{"failed_step":"…","error":"…"}` as
  input, and it must be a **single terminal step**: no `depends_on`,
  no downstream chain. Auxiliary steps never resolve through normal
  scheduling, so anything chained after the failure step would never
  run — and an unlisted downstream step would block completion. The
  target itself must be listed in top-level `aux_steps`.
- For a rich payload (client + workflow + run id + error in one
  message), point `on_failure` at `notify.webhook` instead: it forwards
  the `failed_step`/`error` input as the payload, merges `client` /
  `workflow` from step metadata, and always stamps `run_id` / `step_id`.

## 2. DLQ watchdog (inside dagnats, best-effort)

`workflows/ops/dlq-watchdog.json` runs every 5 minutes
(`ops-dlq-watchdog-cron`): `dlq.check` reads the `DEAD_LETTERS` stream
depth (there is no dagnats HTTP route for the DLQ, hence the dedicated
handler), `skip_if` drops the quiet path, `notify.webhook` pages
`OPS_SLACK_WEBHOOK_URL` when depth > 0.

Scope, stated plainly: the watchdog only runs while dagnats + worker
are healthy. It catches **silent DLQ buildup** (retries exhausted on
side-effect steps). It cannot fire when the engine itself is down —
that is layer 3.

`ops/` is the reserved platform namespace (same prefix conventions as
client folders, enforced by `go test ./workflows/`). Register it with
everything else: `./scripts/register-workflows.sh` picks it up.

## 3. Platform health (outside dagnats — document only)

Process, NATS, or host death must use an **external uptime check**:

- Probe `GET /health` on the dagnats HTTP port (and the NATS port if
  your monitor speaks TCP) from outside the host.
- On failure, post to the same ops webhooks (`OPS_*`) — a dead man's
  switch dagnats can never send itself.
- Suggested cadence: 1 minute, alert after 2–3 consecutive failures.

Do not claim dagnats alone can alert when it is fully down. Layers 1
and 2 cover "runs are failing"; only layer 3 covers "nothing is running".
