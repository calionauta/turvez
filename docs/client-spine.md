# Client spine: one repo, any engine

The repo is the **spine**, not the runtime. A client can run on dagnats, Windmill
or Dagu, on a shared instance or its own server, without changing where its
source lives. This page defines the backbone and the migration rules.

## Principle

`client.yaml` is the single source of truth for **where and how** a client runs.
Everything else is engine-specific source under the same client folder. Scripts,
the skill and CI read the manifest; they never guess the engine from a path.

```yaml
# clients/acme/client.yaml
client: acme
engine: windmill          # dagnats | windmill | dagu
runtime: shared           # shared | dedicated
target:
  base_url: https://app.windmill.dev
  workspace: acme
  # dagnats: nats_url: nats://...   api_url: http://...
  # dedicated: host: acme.internal  (ssh target; same repo, different deploy)
data_class: normal        # normal | sensitive
owner: operator@example.com
```

## Layout

```
clients/<client>/
  client.yaml              # engine, runtime, target, data class
  README.md                # fieldbook (context, systems, contacts)
  automation/
    dagnats/*.json         # engine-specific source; only one engine is active
    windmill/*.flow/       # WAC flows, scripts, schedules
    dagu/*.yaml
  evals/                   # engine-agnostic: promptfooconfig, cases, labels
  env/<client>.env.example # names only
  secrets/<client>/        # age ciphertext (dagnats) or references (windmill)
  policies/                # retention, data class, redaction
```

Only the directory for the active engine is required. Keeping an old engine's
directory during a migration is allowed; `client.yaml` decides which one is
live.

## Engine differences stay at the edges

| Concern | dagnats | Windmill | Dagu |
|---|---|---|---|
| Source | `automation/dagnats/*.json` | `automation/windmill/*.flow/` | `automation/dagu/*.yaml` |
| Deploy | `workflow register` | `wmill flow push` | dags dir + `dagu start-all` |
| Secrets | env + age | Variables/Resources per workspace | host env |
| Triggers | `dagnats trigger` | schedules/webhooks | schedule YAML |
| Run history | NATS KV + events | job API/Postgres | file-based runs |
| Eval source | `run inspect --json` | job result API | run log |

`evals/` does not change. `select-cases.py` gains a `source` adapter
(`dagnats` or `windmill`) that normalizes a run to:

```json
{"run_id":"...","workflow":"acme.<wf>","status":"failed",
 "created_at":"...","steps":[{"id":"...","status":"...","output":...}]}
```

Promptfoo keeps reading the same cases.

## Migrations

- **Engine change**: keep both `automation/` directories during cutover, set
  `engine` in `client.yaml` to the new one only when parity is green, repoint
  webhooks, recreate secrets, run the eval on the same dataset, then delete the
  old directory. `docs/migrate-to-windmill.md` has the step mapping.
- **Shared → dedicated** (or back): only `runtime` and `target` change. The
  source, evals and secrets schema stay; secrets are recreated in the dedicated
  store and the deploy points at the new host. No code rewrite.
- **One engine per client**: the manifest is enforced by CI; a client with two
  active engines fails the check.
- **Client extraction**: a dedicated server is a deployment target, not a repo
  fork. Keep the client in the central repo; use `runtime: dedicated` and a
  target. Fork the repo only when the client requires its own governance.

## Phases

1. **Now**: add `client.yaml` inside `workflows/<client>/` (same paths), with
   `engine: dagnats` for everyone. Scripts read it; nothing moves.
2. **First Windmill client**: put it under `clients/<client>/automation/windmill/`
   and add the eval `source` adapter. Old clients stay where they are.
3. **Majority Windmill**: rename `workflows/<client>` → `clients/<client>` in one
   migration; `workflows/` becomes a legacy symlink or is archived.
4. **No dagnats clients**: drop the Go runtime; the spine and evals stay.

Until phase 3, both layouts coexist and `client.yaml` decides. Do not run two
engines for the same client.
