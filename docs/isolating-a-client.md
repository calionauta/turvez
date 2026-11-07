# Isolating a client

A dedicated host is a **deployment profile** decision (`runtime: dedicated` in
`client.yaml`, see `docs/deployment-profiles.md#when-a-client-needs-its-own-instance`),
not an engine or repo decision. The layout keeps extracting one client a copy
operation, not a redesign: client identity lives in folder + name prefix + env
file only — never in Go code.

The procedure below is the **dagnats** path, which is the common case for
typed/regulated work. For a Windmill client see
["Windmill client on its own instance"](#windmill-client-on-its-own-instance).

## Start from the generated plan, not from this page

```bash
./scripts/verify-isolation.sh <client-id>   # what does the manifest claim today?
./scripts/extract-client.py <client-id>     # package the move
```

`extract-client.py` emits the concrete list for *this* client: the definitions to
carry, the env file whose names must be recreated, the trigger ids to disable,
the ordered commands per server, and the rollback. It is **read-only** — it
prints, you execute. Use `--out DIR` to keep a copy with the engagement.

Both scripts exist because `runtime:` and `target:` used to be written into
`client.yaml` and read by nothing, so "is this client isolated?" was answered
from memory. Now the contract test validates them and `make validate` runs the
isolation check.

## What to copy

- `clients/<client-id>/automation/dagnats/*.json` — all recipes for that client
  (for a Windmill client: `clients/<client-id>/automation/windmill/`)
- Trigger definitions for that client (ids start with `<client-id>-`;
  recreate from the JSON `triggers` arrays, see below)
- `env/<client-id>.env` (or `secrets/<client-id>/env.age` +
  the age identity out-of-band)
- SaaS Auth (Nango): start the client's own broker on the new host with
  `NANGO_CLIENT_ID=<client-id> NANGO_PORT=8188 NANGO_CONNECT_PORT=8189 ./scripts/setup-nango.sh --up`,
  then re-authorize connections for `Connection-Id: <client-id>` (see `docs/nango.md`).
- Access plane, when `access.enabled: true`: carry `clients/<client-id>/catalog/policy.yaml`,
  move the connections (a Postgres dump or a fresh consent), recreate the grants for the
  tenant, revoke them on the old broker, and set `access.plane: client`. `./scripts/extract-client.py
  <client-id>` prints the exact steps.
- Worker image at the same git SHA / version (same binary, different env)

## What NOT to migrate

Historical run events and NATS streams stay on the old server (audit
only). Cutover needs **definitions**, not history: re-register the JSON
from git and start fresh runs on the new host. In-flight runs do not
migrate — drain them (let running finish) or accept re-runs. Design
recipes idempotent-friendly (safe retries, natural keys in side effects)
so a replay is boring.

## Procedure: extract `<client-id>` to its own server

Conventions assumed: workflow names `<client-id>.<wf>` (the separator lives in
`naming.json`), trigger ids
`<client-id>-…`, webhook paths containing `<client-id>`. The register
script and `go test ./clients/` enforce the name prefix.

```bash
CLIENT=acme   # example

# ---------- On the NEW server ----------
# 1. Engine (same pinned version as the old server)
go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.32
dagnats serve &
# or: docker compose up dagnats   (Dockerfile.dagnats pins the version)

# 2. Worker repo + ONLY this client's secrets.
#    Stage 1–2 (shared turvez-ops): clone the private ops repo at the pinned ref.
#    Never clone the public template for client data — it carries no real clients
#    (see docs/ops-repo.md).
git clone <turvez-ops-private-url> && cd <turvez-ops-dir>
git checkout $(awk -F= '/^TURVEZ_REF=/{print $2}' TURVEZ_PIN) 2>/dev/null || git checkout <same-sha-as-old-server>
export TURVEZ_CHECKOUT=<path-to-turvez-template-clone> TURVEZ_OPS_DIR=$PWD
cp env/$CLIENT.env.example env/$CLIENT.env   # fill real values (gitignored)
# or decrypt: age --decrypt -i ~/.secrets/consultancy.age.key \
#   -o /tmp/$CLIENT.env secrets/$CLIENT/env.age
#   export DOTENV_FILE=/tmp/$CLIENT.env
export CLIENT_ID=$CLIENT
export DAGNATS_NATS_URL=nats://127.0.0.1:4222
go run ./cmd/worker &

# 3. Register that client's recipes only (prefix-checked by the script)
./scripts/register-workflows.sh $CLIENT
dagnats workflow list   # expect only $CLIENT.<wf> names

# 4. Recreate that client's triggers only.
#    Cron (dry-check the expression first):
dagnats trigger test "0 2 * * *" --tz=UTC --count=3
dagnats trigger create "$CLIENT.nightly-sync" --cron="0 2 * * *" --tz=UTC
#    Webhook (path must contain the client id):
dagnats trigger create "$CLIENT.hubspot-lead-to-slack" --webhook=/hooks/$CLIENT/lead
dagnats trigger list   # expect only $CLIENT-… ids

# 5. Smoke on the new server
RUN_ID=$(dagnats run start "$CLIENT.hubspot-lead-to-slack" '{"lead_id":"cutover-1"}' --json \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["run_id"])')
sleep 8
dagnats run status $RUN_ID
dagnats run events $RUN_ID | head -20
#    Webhook test (path from the recipe's triggers array):
curl -X POST http://127.0.0.1:8080/hooks/$CLIENT/lead \
  -H 'Content-Type: application/json' -d '{"lead_id":"cutover-webhook"}'
dagnats run list --workflow=$CLIENT.<wf>   # new run appears (exact name)

# ---------- On the OLD server ----------
# 6. Disable ONLY this client's triggers (others untouched).
#    Prefer disable first (instant rollback); delete after bake-in.
dagnats trigger list   # note the $CLIENT-… ids
dagnats trigger disable $CLIENT-nightly-sync-cron
dagnats trigger disable $CLIENT-lead-webhook
# later: dagnats trigger delete <id> … ; dagnats workflow delete "$CLIENT.<name>"

# ---------- External systems ----------
# 7. Repoint SaaS webhook sender URLs from old host → new host
#    (HubSpot/Stripe/… workflow or app settings), then re-run the
#    curl webhook test against the NEW host and confirm the run.
```

## Verification checklist

- [ ] `dagnats workflow list` on new server shows only `$CLIENT.<wf>`
- [ ] Smoke run completes transform steps; side-effect steps succeed
      with real secrets (or fail with a clear secret/config error, not
      a protocol error — see `scripts/smoke-test.sh`)
- [ ] Webhook POST to the new host creates a run under `$CLIENT.`
- [ ] `dagnats trigger test` shows expected cron fire times
- [ ] Old server: `dagnats trigger list` shows the client's triggers
      disabled; other clients' triggers still enabled; no new
      `$CLIENT.` runs appear there

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

## Windmill client on its own instance

Same principle (definitions from git, history stays), different commands. A
self-hosted Windmill instance is Postgres + workers, so "own server" for a
Windmill client is a heavier commitment — confirm the licence path first
(`docs/deployment-profiles.md#engine-implications-per-profile`).

```bash
CLIENT=acme   # example

# ---------- On the NEW server ----------
# 1. Windmill instance (Docker Compose from upstream; pin the version).
#    One instance can hold several workspaces; a dedicated instance is the
#    deployment-profile decision, a workspace is the tenancy unit.
# 2. Create the workspace, then push the client's flows ONE AT A TIME.
#    NEVER `wmill sync push` (it deletes remote items missing locally —
#    it removed 12 items from a live workspace during the pilot).
for f in clients/$CLIENT/automation/windmill/*/; do
  wmill flow preview "$f"          # lint locally, no deploy
  wmill flow push  "$f" "f/$CLIENT/$(basename "$f")"
done

# 3. Recreate secrets as workspace variables/resources — values never in git.
#    env/<client>.env / age is the BOOTSTRAP source, not the runtime store:
#    the workspace variable is what the flow reads.
# 4. Recreate schedules/triggers in the workspace.
# 5. Smoke: `wmill flow run f/$CLIENT/<wf>` and `wmill job get <id> --json`.
#    For the eval loop: scripts/windmill-job-to-run.py --job-id <id>

# ---------- On the OLD server ----------
# 6. Disable this client's schedules only. Prefer disable; delete after bake-in.

# ---------- External systems ----------
# 7. Repoint SaaS webhook URLs, then re-run the smoke flow.
```

There is no `jobs` history migration. Run history stays where it ran; the
cutover needs **definitions**, and a `windmill-job-to-run.py` export if the
client's eval set must survive the move.

### Windmill verification checklist

- [ ] `wmill flow list` in the workspace shows only this client's flows
- [ ] Every pushed flow has a lockfile (`locksRequired: true`, `wmill lint` clean)
- [ ] Secrets exist as workspace variables; the flow reads no `.env` at runtime
- [ ] A schedule fires a run in the new workspace
- [ ] Old instance: this client's schedules disabled, others untouched
- [ ] Licence path recorded for the contract (AGPL self-host vs Cloud vs EE)
