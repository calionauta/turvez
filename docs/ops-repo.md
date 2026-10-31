# Ops repo (private) vs template (public)

This repo (`turvez`) is the **template + engine**: public, MIT, no real
customers. Real work lives in a private **ops repo** (`turvez-ops` default,
`turvez-<client>` for physical isolation). Git is source of truth in both;
dagnats KV is a deploy target.

## Which repo am I in?

| File present | Meaning |
|---|---|
| `go.mod` only | Template (this repo). Real clients forbidden — `go test` fails them. |
| `TURVEZ_PIN` only | Ops repo (data-only). Any client folder allowed. |
| Both / neither | Ambiguous — stop, check `TURVEZ_CHECKOUT` / `TURVEZ_OPS_DIR`. |

LLM rule (Q0, before anything else): detect repo type first. Real client in
template → refuse, point to `turvez-ops` + `new-client.sh`.

## Ops repo layout (data-only, no Go)

```text
turvez-ops/
  TURVEZ_PIN                  # TURVEZ_REF=<tag|sha> + DAGNATS_VERSION=v0.0.30
  AGENTS.md                   # ops rules (validate from template checkout)
  .github/workflows/contract.yml  # CI: template tests @TURVEZ_PIN vs this repo
  clients/<client>/client.yaml    # engine, runtime, target (see docs/client-spine.md)
  clients/<client>/README.md + *.json
  clients/<client>/evals/                # promptfooconfig + generated cases
  clients/personal/ + clients/ops/   # seeded from template, watchdog included
  env/<client>.env.example    # NAMES only (commit)
  env/<client>.env            # values (never commit)
  secrets/<client>/env.age + .recipients # ciphertext (git add -f) + age pubkeys
  docker-compose.override.yml # CLIENT_ID, age mounts
```

## Pinning

`TURVEZ_REF` accepts an annotated release tag (`v0.1.0`) or a full SHA.
Prefer the tag: it is immutable once pushed, readable in incident notes, and
gives `sync-template.sh` a release range to compare. Use a SHA only for an
unreleased commit you deliberately want to test.

```bash
git fetch --tags                 # sync-template resolves the tag locally
# TURVEZ_PIN
TURVEZ_REF=v0.1.0
DAGNATS_VERSION=v0.0.30
```

Bump only after reading the release notes and running the ops contract tests
(`make validate-ops TURVEZ_OPS_DIR=…`).

## Lifecycle

```bash
# 1. Scaffold (from template checkout; skill proposes the command and waits — Q0b):
./scripts/new-ops-repo.sh ../turvez-ops   # local-only unless --github-private
cd ../turvez-ops && git init && git add -A && git commit -m 'ops init'
gh repo create --private --source=. --push   # never public; only after operator says so

# 2. New client (inside ops, or TURVEZ_OPS_DIR from template):
export TURVEZ_CHECKOUT=~/Development/turvez TURVEZ_OPS_DIR=$PWD
$TURVEZ_CHECKOUT/scripts/new-client.sh acme

# 3. Validate (contract tests live in template — run from the template checkout):
cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$TURVEZ_OPS_DIR/clients go test ./clients/
# or: make validate-ops TURVEZ_OPS_DIR=$TURVEZ_OPS_DIR   (from $TURVEZ_CHECKOUT)

# 4. Register / run (prefix-checked per client):
WORKFLOWS_DIR=$PWD/workflows $TURVEZ_CHECKOUT/scripts/register-workflows.sh acme
dagnats run start "acme/nightly-sync" '{}' --watch

# 5. Secrets (ciphertext in git, plaintext only in trapped tmp):
TURVEZ_OPS_DIR=$PWD $TURVEZ_CHECKOUT/scripts/edit-secrets.sh acme
git add -f secrets/acme/env.age && git commit

# 6. Drift check (never auto-merge, run from $TURVEZ_CHECKOUT):
./scripts/sync-template.sh   # with TURVEZ_CHECKOUT=$TURVEZ_CHECKOUT TURVEZ_OPS_DIR=$PWD
# bump TURVEZ_REF to the new release tag after review
```

## Tenancy map

- Stage 1–2: one `turvez-ops` for all clients (folder + `<client>.<wf>` names + env).
- Stage 3: `turvez-<client>` per isolated client — same scaffold, `CLIENT_ID`
  set, triggers re-created, old triggers disabled (see `isolating-a-client.md`).
- Second operator: they send their age pubkey, you append to
  `secrets/<client>/.recipients` — they decrypt only listed clients.
- Another consultancy: forks `turvez`, scaffolds their own `turvez-ops`.
  No shared state; template improvements return as upstream PRs.

## What never moves to the template

Fieldbooks with real contacts/systems, recipes with real business logic,
`env.age` with real ciphertext — even encrypted, the metadata (client names,
secret names, systems) leaks. Template keeps only `_template/`,
`example-client/`, `ops/`, `personal.ping` as fixtures.
