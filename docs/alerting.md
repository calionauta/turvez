# Alerting

The console shows failures; nothing pages you. This repo wires three
layers so a solo operator hears about breakage without watching the UI.

## 1. In-workflow failure pages (per client recipe)

A recipe with a trigger runs unattended, so `on_failure` is the only
in-engine page. `go test ./clients/` fails a triggered recipe that
declares none — including `ops.dlq-watchdog`, which must obey the rule
it is part of.

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

(`clients/example-client/automation/dagnats/approval-refund.json` — the reference example.)

Three engine facts shape this pattern (verified against the pinned
dagnats source, v0.0.32):

- An `on_failure` step receives `{"failed_step":"…","error":"…"}` as
  input, and it must be a **single terminal step**: no `depends_on`,
  no downstream chain. Auxiliary steps never resolve through normal
  scheduling, so anything chained after the failure step would never
  run — and an unlisted downstream step would block completion. The
  target itself must be listed in top-level `aux_steps`.
- **The envelope alone pages nobody.** Slack, Telegram and Discord
  ignore unknown keys, so posting `{"failed_step","error"}` verbatim
  returns 200 and renders an empty message. `notify.webhook` therefore
  synthesizes a `text` line (`step "x" failed: … (run r)`) when the
  payload is a failure envelope and the author supplied no message.
  An explicit `message` still wins. Prefer `notify.slack` +
  `message` when the page is just text; use `notify.webhook` when you
  need the run id and error as structured fields as well.
- The failure path has no engine retry: the target runs once. If the
  webhook is down, the page is lost — which is why layer 3 is not
  optional.

## 2. DLQ watchdog (inside dagnats, best-effort)

`clients/ops/automation/dagnats/dlq-watchdog.json` runs every 5 minutes
(`ops-dlq-watchdog-cron`): `dlq.check` reads the `DEAD_LETTERS` stream
depth (there is no dagnats HTTP route for the DLQ, hence the dedicated
handler), `skip_if` drops the quiet path, `notify.webhook` pages
`OPS_SLACK_WEBHOOK_URL` when depth exceeds the threshold (default 0,
so the first dead letter pages).

The message is actionable without opening the console — depth, stream,
the command that lists the entries, the replay command, and the fact
that the alert repeats every 5 minutes until the stream is drained.
That repetition is deliberate: silence must require an action.

The watchdog also has `on_failure` on both its `check` and its `notify`
step. A watchdog that dies quietly is worse than no watchdog, because
you believe you are covered.

### Draining it

```bash
dagnats dlq list                 # entries (run id, step, task, error)
dagnats dlq list --json --all    # machine-readable, past the 500-entry cap
dagnats dlq list --run=<run-id>  # one run
dagnats dlq replay --run=<run-id>  # re-publish the preserved body
```

Replay needs the body preserved (`body_preserved: true`); entries that
predate that schema must be recovered upstream. Fix the cause before
replaying, or the message lands back in the DLQ.

### Scope, stated plainly

The watchdog only runs while dagnats + worker are healthy. It catches
**silent DLQ buildup** (retries exhausted on side-effect steps). It
cannot fire when the engine itself is down — that is layer 3.

It is a dagnats-engine layer. The other engines have no DLQ stream, so
the equivalent is different, not missing:

| Engine | Failure paging |
|---|---|
| dagnats | `on_failure` per recipe (layer 1) + this watchdog (layer 2) |
| Windmill | a flow **error handler** posting to the ops webhook (free edition has no native critical alerts); failed jobs stay in the queue/UI, so there is nothing to drain |
| Dagu | `handlerOn: {failure: …}` / `mailOnFailure` on the DAG; the run log is the record |

`ops/` is the reserved platform namespace (same prefix conventions as
client folders, enforced by `go test ./clients/`). Register it with
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
