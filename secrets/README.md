# Secrets — quick start

Full doc: `docs/secrets.md` (age per client, JIT runtime, sops evaluation,
LLM hygiene). This directory holds only ciphertext and docs — never values.

```bash
cp env/acme.env.example env/acme.env   # fill values (gitignored)
age -r age1YOURPUBLICKEY -o secrets/acme/env.age env/acme.env
git add -f secrets/acme/env.age        # -f: secrets/ is often globally ignored

# Runtime (in-memory merge, no plaintext on disk):
export CLIENT_ID=acme                                              # dev: env/acme.env
# or:
export SECRETS_AGE_FILE=$PWD/secrets/acme/env.age                 # server/container
export AGE_IDENTITY_FILE=~/.secrets/consultancy.age.key           # chmod 600, never in git
```

Rules: JSON references secret **names** only; one file per client
(`secrets/<client>/env.age`, `env/<client>.env`); new keypair per
execution environment, per client when compliance demands it.
