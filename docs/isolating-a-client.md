# Isolating a client

One shared dagnats instance is fine to start. The layout keeps the
extraction of one client to a dedicated host a copy operation, not a
redesign.

## What already isolates a client

- **Workflows**: `workflows/<client>/…`, names prefixed
  `<client>/…` (e.g. `acme/hubspot-lead-to-slack`). Triggers and run
  history follow the workflow name — filtering by prefix
  (`dagnats run list --workflow=acme/…`) scopes ops per client.
- **Secrets**: one env file per client (`secrets/acme.env.age`,
  server-side `~/.secrets/acme.env`). The worker only loads the files
  you point it at.
- **Notifications**: per-client webhook secrets; no shared channel.

## Extracting `acme` to its own host

On the new host:

```bash
# 1. dagnats (same pinned version)
go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.24
dagnats serve &

# 2. This repo, worker with ONLY acme secrets
git clone <this-template> && cd <this-template>
age --decrypt -i ~/.secrets/consultancy.age.key \
  -o /tmp/acme.env secrets/acme.env.age   # or copy ~/.secrets/acme.env
export DOTENV_FILE=/tmp/acme.env          # or SECRETS_AGE_FILE=…
export NATS_URL=nats://127.0.0.1:4222
go run ./cmd/worker &

# 3. Register only acme recipes, recreate triggers
./scripts/register-workflows.sh workflows/acme
dagnats trigger create 'acme/nightly-sync' --cron='0 2 * * *' --tz=UTC
```

Then cut over the webhook sender URLs to the new host
(`POST /hooks/acme/…`) and delete `workflows/acme/` from the shared
instance (`dagnats workflow delete 'acme/…'` per workflow, remove its
triggers first).

## Rules that keep this cheap

- Never share a secret across clients; never hardcode a client name in
  Go — client identity lives in workflow names + env files only.
- Keep per-client trigger ids prefixed (`acme-nightly`), so
  `dagnats trigger list` stays greppable.
- In-flight runs do not migrate — drain (let running finish, pause
  triggers) or accept re-runs on the new host. Design recipes
  idempotent-friendly (safe retries, natural keys in side effects)
  so a replay is boring.
