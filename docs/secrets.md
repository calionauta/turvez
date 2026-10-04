# Per-client secret management (age)

The repo stores ciphertext only. Cleartext values exist only in worker
memory at runtime, or in files outside git. Asymmetric encryption with
[age](https://github.com/filosottile/age): encrypting needs just the
**public** key; only the execution environment holds the private one.

## Layout

| Path | Content | In git? |
|---|---|---|
| `env/<client>.env.example` | Variable names (template) | Yes |
| `env/<client>.env` | Real values, local dev | No (`env/*.env` in `.gitignore`) |
| `secrets/<client>/env.age` | Same `.env`, encrypted | Yes (ciphertext; may need `git add -f`) |
| `~/.secrets/` (operator/server) | Production `.env` and/or `*.age.key` (chmod 600) | Never |
| `*.age.key` | age private identity | Never, anywhere versioned |

Workflow JSON references **names** only (`"auth_secret": "ACME_X"`). The
`TestRecipeStructure` test fails the build if a secret-looking value
(`hooks.slack…`, `xoxb-`, `sk-`, `ghp_`, PEM) shows up in any recipe.

## Keys: one per environment, per client when needed
- **Default:** one consultancy key per execution environment (laptop,
  server). One file per client, all encrypted to the same recipient —
  simple to operate.
- **Per client:** generate a dedicated pair when a client requires
  isolation or compliance (stage 3 in `docs/tenancy-evolution.md`).
  The loader does not care: each host uses its own `AGE_IDENTITY_FILE`.
- **Per person (partners):** recipients are listed per client in the
  committed `secrets/<client>/.recipients` — see the handoff section
  below. age encrypts to several recipients natively; no extra tooling.

```bash
age-keygen -o ~/.secrets/consultancy.age.key   # once per machine
chmod 600 ~/.secrets/consultancy.age.key
age-keygen -y ~/.secrets/consultancy.age.key   # prints the public key
```

## Cycle: create → edit → version

Always edit via `scripts/edit-secrets.sh <client>` — it decrypts into a
trapped temp dir (wiped on any exit, editor swap included), re-encrypts
to a recipient derived from your own identity (never encrypts to a key
you cannot open), warns about keys missing from the template, and
verifies decryption before finishing:

```bash
make edit-secrets CLIENT=acme
# …edit in $EDITOR, save, quit…
git add -f secrets/acme/env.age && git commit
```

Bootstrap (first time, no ciphertext yet): the script starts from
`env/<client>.env.example`. Git history is the backup — the previous
ciphertext stays committed until the next commit.

## Clients handing you keys, and partners supporting clients

**Receiving keys from a client.** Agree on a channel first — shared
1Password vault item is best; encrypted email acceptable; plain chat or
email never. Paste straight into the edit flow (`make edit-secrets`),
commit the ciphertext, then delete the original message/file. If the
client is capable, they can encrypt to your public key themselves with
plain `age -r` and send only ciphertext.

**Partner supporting another client.** Each person holds their own age
keypair; the private key never leaves their machine:

1. Partner sends you their **public** key (authenticated channel —
   confirm it out-of-band the first time).
2. You append it to `secrets/<client>/.recipients` (committed — public
   keys are public) for the clients they support, and commit.
3. Partner runs the same `make edit-secrets CLIENT=<client>` — the
   script encrypts to every listed recipient.

The effect is cryptographic least privilege on a shared private repo:
the partner decrypts only the clients whose `.recipients` lists them;
everyone else's ciphertext is opaque, including to anyone browsing git.
Repo access ≠ secret access. Clients get no repo access at all.

**Revoking.** Delete the line, re-encrypt without editing, commit:

```bash
make edit-secrets CLIENT=acme   # after removing their key from .recipients
./scripts/edit-secrets.sh --re-encrypt acme
git add -f secrets/acme/env.age && git commit
```

Honest limit: revocation blocks **future** reads only. Values already
decrypted while access lasted cannot be unseen — for a hostile split,
rotate the underlying SaaS tokens too.

## age cheat-sheet (for when you need the manual cycle)
```bash
age-keygen -o ~/.secrets/x.age.key && chmod 600 ~/.secrets/x.age.key
age-keygen -y ~/.secrets/x.age.key                    # public key
age -r age1PUBLICKEY -o secrets/acme/env.age env/acme.env
age --decrypt -i ~/.secrets/x.age.key -o - secrets/acme/env.age | head -c 200  # inspect
```

Prefer the script over manual: the manual cycle forgets to shred the
temp file and leaves editor backup/swap behind.

## Runtime: just-in-time decryption, no `.env` on disk

`internal/secret` decrypts via the `age` binary and merges **in memory**
— the worker never needs a decrypted `.env` on disk. Avoid the
`age --decrypt … > .env` pattern; if an entrypoint must have a file, use
tmpfs and destroy it after boot (and `.gitignore` already covers `.env`
and `env/*.env`).

```bash
# Local dev: client file directly
CLIENT_ID=acme go run ./cmd/worker

# Server / container: ciphertext + identity outside the repo
export SECRETS_AGE_FILE=/run/secrets/acme/env.age
export AGE_IDENTITY_FILE=/run/secrets/consultancy.age.key
./worker   # in-memory merge; already-exported env wins
```

```dockerfile
# Example entrypoint that only mounts secrets (never bake them in the image)
# docker run -v ~/.secrets:/run/secrets:ro -e SECRETS_AGE_FILE=/run/secrets/acme/env.age \
#   -e AGE_IDENTITY_FILE=/run/secrets/consultancy.age.key worker
```

## Rotation

Leaked or retired machine: generate a new pair, re-encrypt every
`secrets/<client>/env.age` to the new public key, commit, destroy the
old private key on hosts. Rotation is per **file** — no code changes.

## sops: evaluation

[`sops`](https://github.com/getsops/sops) also uses age under the hood,
but adds in-place editing (`sops file` opens the editor, decrypts to
temp, re-encrypts on save), multiple recipients, and key rotation
(`sops updatekeys` / `rotate`).

**Verdict (reviewed against the real setup): keep native age.** Three
reasons, the first decisive:

1. **sops cannot edit plain age files.** Adopting it changes the format
   (sops envelope) — and then the runtime changes with it: the
   `age --decrypt` in `internal/secret` cannot read a sops envelope, so
   the container would need the sops binary (`sops exec-env`) or a
   loader change. Real migration cost, zero runtime benefit (values land
   in the same env vars the same way).
2. sops superpowers (teams, multiple recipients, Shamir/KMS, frequent
   rotation) do not match a solo operator editing rarely.
3. The real pain — editing without leaking cleartext into temp/swap —
   is already solved with no new dependency:
   `scripts/edit-secrets.sh` (tested cycle: bootstrap, roundtrip,
   no-op, typo guard).

**When to migrate to sops — concrete triggers, not before:**

- a second operator with secret access, or
- per-client recipients becoming routine (compliance-heavy stage 3), or
- weekly+ editing where the manual cycle becomes friction.

**If a trigger fires, integration is:**

1. Root `.sops.yaml` mapping each folder to its recipient:
   ```yaml
   creation_rules:
     - path_regex: secrets/[^/]+/\.env$
       age: age1PUBLICKEYOFCONSULTANCY
   ```
2. Create/edit: `sops secrets/acme/prod.env` (opens `$EDITOR`; never
   let the temp escape — sops cleans it on save).
3. Runtime **with no code changes**: entrypoint with the sops binary
   available, no file on disk:
   ```bash
   sops exec-env secrets/acme/prod.env './worker'
   ```
   (`exec-env` injects decrypted values into the environment and execs.)
4. **Format caveat (important):** a sops file uses its own envelope —
   `age --decrypt` in `internal/secret` **cannot** read it. Either the
   runtime uses `sops exec-env` / `sops --decrypt`, or the file stays in
   plain age. Do not mix both formats in one folder.

## dotenvx: evaluation

[`dotenvx`](https://dotenvx.com) encrypts values in place (`KEY=
`encrypted:…``), keeping names readable and the public key in the file;
the private key lives in `.env.keys` (never commit) or the platform.
Crypto is ECIES/secp256k1 per value. Editing UX is genuinely the best
of the three (`dotenvx set KEY v -f file`, per-key `-k`, mixed
plaintext, `rotate`). Tested locally against v1.66.0.

**Verdict: not for this repo.** Two structural mismatches, both
verified, not matters of taste:

1. **One keypair per file, no multi-recipient.** A file carries a
   single `DOTENV_PUBLIC_KEY`; onboarding a partner means **sending
   them the private key** over a secure channel. Compare with age
   here: the partner generates a pair and sends you only the *public*
   key (`secrets/<client>/.recipients`). The direction of trust is
   inverted — dotenvx makes the sensitive material the thing you
   distribute. Per-client files scope the blast radius, but multiply
   private-key distribution instead of eliminating it.
2. **Runtime is Node.js** (the binary is a node script). The worker
   image is distroless static Go; integration would mean wrapping the
   process (`dotenvx run -- ./worker`) or shelling out per key —
   dragging Node into a Go stack for zero secrecy gain (values land in
   env identically). Format lock-in also differs: age files open with
   any age implementation; dotenvx files need dotenvx (commercially
   backed, with vault upsell baked into the CLI).

Fair credit: for a Node.js shop sharing keys via platform secret
managers, with no partner least-privilege requirement, dotenvx's
editing flow and readable diffs are the most ergonomic option.
Reconsider only if this stack becomes Node, or `DOTENV_PRIVATE_KEY`
moves to a managed platform per environment.

## Hygiene against LLM/agent reads

- Agents work with secret **names** (`env/*.env.example`), never ask
  for or print values.
- Never paste `.env` / `~/.secrets` content into chat, issues, or prompts.
- Identities (`*.age.key`) live in `~/.secrets` with `600`, outside any
  repo — even if an agent lists the directory, there is nothing to read.
- The value scan in `go test ./workflows/` is the safety net: a secret
  landing in a recipe breaks CI before commit.
