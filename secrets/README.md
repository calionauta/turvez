# Secrets

Primary: **process environment variables**. Secondary (optional): **age-encrypted
env files** committed as `secrets/<client-id>/*.env.age` (ciphertext is safe in git).

Rules:

1. Workflow JSON references secret **names** only (e.g. `"auth_secret":
   "ACME_HUBSPOT_TOKEN"`). The value is resolved at runtime by
   `internal/secret`. Never put a credential value in a workflow file.
2. Real values live in `env/<client-id>.env` (gitignored) or
   `~/.secrets/<client-id>.env` on the server (chmod 600).
3. The age private key (`*.age.key`) lives only on the operator machine and
   the server (`~/.secrets/`), never in the repo.
4. Secret *names* are documented per client in `env/<client-id>.env.example`
   (committed). No shared secrets across clients.

## Setup (per client)

```bash
# 1. Generate an age key (once per machine)
age-keygen -o ~/.secrets/consultancy.age.key
chmod 600 ~/.secrets/consultancy.age.key

# 2. Fill the per-client env file (start from the committed template)
cp env/example-client.env.example env/example-client.env
$EDITOR env/example-client.env   # real values; gitignored

# 3a. Dev/server plaintext path — point the worker at it:
export CLIENT_ID=example-client            # worker loads env/$CLIENT_ID.env
# or: export DOTENV_FILE=$PWD/env/example-client.env

# 3b. Encrypted path — commit ciphertext, keep plaintext out of git:
age --encrypt -r age1YOURPUBLICKEYHEREREPLACEWITHYOUROWN \
  -o secrets/example-client/prod.env.age env/example-client.env
git add -f secrets/example-client/prod.env.age
# -f because many operators globally ignore secrets/; ciphertext is safe.
export SECRETS_AGE_FILE=$PWD/secrets/example-client/prod.env.age
export AGE_IDENTITY_FILE=~/.secrets/consultancy.age.key
```

At startup the worker loads, in order: `DOTENV_FILE` (or
`env/$CLIENT_ID.env` when `CLIENT_ID` is set), then `SECRETS_AGE_FILE`
decrypted with `age --decrypt` — merged in memory only, never written
to disk. **Already-set env vars always win**, so per-host overrides
keep working.

## Per-client layout

- `env/acme.env.example` — committed secret *names* for ACME
- `env/acme.env` — real values, gitignored (or `~/.secrets/acme.env` on server)
- `secrets/acme/prod.env.age` — ciphertext, committable
- Repeat per client: `env/beta.*`, `secrets/beta/…`

See `docs/isolating-a-client.md` for extracting a client to its own host.
