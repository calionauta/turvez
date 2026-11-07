# Remotes: Windmill and Nango, cloud or self-hosted

The engine and the broker can run on a vendor's cloud, on the client's server,
or on ours. They are **separate answers** — a client can run its engine on
Cloud and its broker on its own host, or the reverse. Both are recorded under
`deployment:` in `client.yaml`; the repo never infers them
(`docs/access-plane.md` §3, `docs/deployment-profiles.md`).

At onboarding, `scripts/new-client.sh` asks for each layer separately:

```bash
./scripts/new-client.sh acme --engine=windmill --windmill=cloud --nango=self_hosted
```

With a terminal attached it prompts `windmill hosting [cloud/self_hosted] (cloud)`
and the same for `nango`; non-interactive runs use the `cloud` default or the
flag values. Each client gets its own answer; a dagnats client records only
`deployment.nango`, a Windmill client records both.

`scripts/connect-remotes.sh` is how the operator's answer reaches the shell:

```bash
./scripts/connect-remotes.sh          # prompts, validates, saves
./scripts/connect-remotes.sh --check  # re-validate what is saved
./scripts/connect-remotes.sh --print  # which names are set (values never shown)
```

It writes `~/.secrets/turvez-remotes.env` (mode 600, outside the repo) with the
names the tools already read:

| Variable | Read by |
| --- | --- |
| `BASE_INTERNAL_URL` | `wmill` CLI (Cloud or self-hosted) |
| `WM_TOKEN` | `wmill` CLI |
| `WM_WORKSPACE` | `wmill` CLI — one workspace per client |
| `NANGO_HOST` | recipes (proxy URL), `scripts/nango-*` |
| `NANGO_SECRET_KEY` | the same, as the `auth_secret` name |

Load them in a shell:

```bash
set -a; . ~/.secrets/turvez-remotes.env; set +a
```

## What validation proves, and what it does not

The script makes **one read-only GET per remote** — `/api/users/whoami` for
Windmill (falling back to `/api/version` and saying the token was not verified),
and `/integrations` for Nango. Nothing is created, pushed or run.

It proves: the URL answers, and the token/secret is accepted.

It does **not** prove: that the account belongs to the client (contract and
fieldbook), that the workspace is the right one for this client (that is
`target.workspace` in `client.yaml`, enforced by `go test ./clients/`), or that
the remote runs the same version as the local CLI.

## Cloud does not change the contracts

`go test`, `validate.sh` and `test-decisions.sh` are local and stay the gate. A
remote is **one smoke per deployment**, not a test suite:

- Windmill: `wmill flow preview <flow>` runs the local definition without
  deploying it; `wmill flow list` only proves auth. Never `wmill sync push`.
- Nango: the health endpoint plus one proxy call per provider. OAuth consent
  still needs a human and a real provider app.
- The access plane (grants, revocation, audit) has no remote to test yet — that
  is P2 (`docs/access-plane.md` §7), not a deployment choice.

## When the operator says "cloud"

Record it, do not infer it:

```yaml
engine: windmill
deployment:
  windmill: cloud
  nango: self_hosted
target:
  base_url: https://cloud.windmill.dev
  workspace: acme
access:
  enabled: true
  plane: cloud              # vendor cloud…
  vendor_account: client    # …under the client's own account and card
  identity: org
```

`vendor_account: consultancy` with `plane: cloud` fails the contract test: with
our account it is the consultancy-shared shape, whatever the URL says.
