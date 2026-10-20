# Isolating a client

One shared dagnats instance is fine to start (stage 1 in
`docs/tenancy-evolution.md`). The layout keeps extracting one client to
a dedicated host a copy operation, not a redesign: client identity lives
in folder + name prefix + env file only — never in Go code.

## What to copy

- `workflows/<client-id>/*.json` — all recipes for that client
- Trigger definitions for that client (ids start with `<client-id>-`;
  recreate from the JSON `triggers` arrays, see below)
- `env/<client-id>.env` (or `secrets/<client-id>/env.age` +
  the age identity out-of-band)
- Worker image at the same git SHA / version (same binary, different env)

## What NOT to migrate

Historical run events and NATS streams stay on the old server (audit
only). Cutover needs **definitions**, not history: re-register the JSON
from git and start fresh runs on the new host. In-flight runs do not
migrate — drain them (let running finish) or accept re-runs. Design
recipes idempotent-friendly (safe retries, natural keys in side effects)
so a replay is boring.

## Procedure: extract `<client-id>` to its own server

Conventions assumed: workflow names `<client-id>/…`, trigger ids
`<client-id>-…`, webhook paths containing `<client-id>`. The register
script and `go test ./workflows/` enforce the name prefix.

```bash
CLIENT=acme   # example

# ---------- On the NEW server ----------
# 1. Engine (same pinned version as the old server)
go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30
dagnats serve &
# or: docker compose up dagnats   (Dockerfile.dagnats pins the version)

# 2. This repo + worker with ONLY this client's secrets
git clone <this-template> && cd <this-template>
git checkout <same-sha-as-old-server>
cp env/$CLIENT.env.example env/$CLIENT.env   # fill real values (gitignored)
# or decrypt: age --decrypt -i ~/.secrets/consultancy.age.key \
#   -o /tmp/$CLIENT.env secrets/$CLIENT/env.age
#   export DOTENV_FILE=/tmp/$CLIENT.env
export CLIENT_ID=$CLIENT
export DAGNATS_NATS_URL=nats://127.0.0.1:4222
go run ./cmd/worker &

# 3. Register that client's recipes only (prefix-checked by the script)
./scripts/register-workflows.sh $CLIENT
dagnats workflow list   # expect only $CLIENT/… names

# 4. Recreate that client's triggers only.
#    Cron (dry-check the expression first):
dagnats trigger test "0 2 * * *" --tz=UTC --count=3
dagnats trigger create "$CLIENT/nightly-sync" --cron="0 2 * * *" --tz=UTC
#    Webhook (path must contain the client id):
dagnats trigger create "$CLIENT/hubspot-lead-to-slack" --webhook=/hooks/$CLIENT/lead
dagnats trigger list   # expect only $CLIENT-… ids

# 5. Smoke on the new server
RUN_ID=$(dagnats run start "$CLIENT/hubspot-lead-to-slack" '{"lead_id":"cutover-1"}' --json \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])')
sleep 8
dagnats run status $RUN_ID
dagnats run events $RUN_ID | head -20
#    Webhook test (path from the recipe's triggers array):
curl -X POST http://127.0.0.1:8080/hooks/$CLIENT/lead \
  -H 'Content-Type: application/json' -d '{"lead_id":"cutover-webhook"}'
dagnats run list --workflow=$CLIENT/   # new run appears

# ---------- On the OLD server ----------
# 6. Disable ONLY this client's triggers (others untouched).
#    Prefer disable first (instant rollback); delete after bake-in.
dagnats trigger list   # note the $CLIENT-… ids
dagnats trigger disable $CLIENT-nightly-sync-cron
dagnats trigger disable $CLIENT-lead-webhook
# later: dagnats trigger delete <id> … ; dagnats workflow delete "$CLIENT/<name>"

# ---------- External systems ----------
# 7. Repoint SaaS webhook sender URLs from old host → new host
#    (HubSpot/Stripe/… workflow or app settings), then re-run the
#    curl webhook test against the NEW host and confirm the run.
```

## Verification checklist

- [ ] `dagnats workflow list` on new server shows only `$CLIENT/…`
- [ ] Smoke run completes transform steps; side-effect steps succeed
      with real secrets (or fail with a clear secret/config error, not
      a protocol error — see `scripts/smoke-test.sh`)
- [ ] Webhook POST to the new host creates a run under `$CLIENT/`
- [ ] `dagnats trigger test` shows expected cron fire times
- [ ] Old server: `dagnats trigger list` shows the client's triggers
      disabled; other clients' triggers still enabled; no new
      `$CLIENT/` runs appear there

## Rollback

Re-enable on the old server, repoint SaaS URLs back, disable on the new:

```bash
# On old server
dagnats trigger enable $CLIENT-nightly-sync-cron
dagnats trigger enable $CLIENT-lead-webhook
# On new server (stop it taking work)
dagnats trigger disable $CLIENT-nightly-sync-cron
dagnats trigger disable $CLIENT-lead-webhook
```

Definitions are portable (git + register); history never moves. If the
new host misbehaves, the old host still has its definitions, secrets,
and history — rollback is two enables plus a URL change.
