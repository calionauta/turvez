# Client spine: one repo, any engine

The repo is the **spine**, not the runtime. A client can run on dagnats, Windmill
or Dagu, on a shared instance or its own server, without changing where its
source lives. This page defines the backbone and the migration rules.

**Scope.** Repo split (template vs ops) is `docs/ops-repo.md`; tenancy and
isolation are `docs/tenancy-evolution.md` and `docs/isolating-a-client.md`.
This page only decides engine and runtime. It does not replace those documents.

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

One layout, one client folder. Engine-specific source lives under `automation/`,
the eval layer is engine-agnostic under `evals/`:

```
clients/<client>/
  client.yaml                 # engine, runtime, target, data class
  README.md                   # fieldbook
  automation/{dagnats,windmill,dagu}/   # only the active engine is required
  evals/                      # promptfoo config, cases, labels
env/<client>.env.example
secrets/<client>/
```

```
clients/_template/            # skeleton (FIELDBOOK, client.yaml, cron-job, evals)
clients/example-client/       # worked example, kept in the template repo
clients/ops/                  # platform overlays (DLQ watchdog)
clients/personal/             # your own namespace
```

`clients/recipes_test.go` is the contract test. It validates the recipes, the
fieldbook and the tenancy rules of **every** client folder. It also accepts the
legacy `workflows/<client>/*.json` layout, so an ops repo can finish its own
migration on its own schedule.

The legacy layout (`workflows/<client>/*.json` + top-level `evals/<client>/`)
still validates, but it is frozen: new clients are scaffolded under `clients/`.
Migrate a checkout with `./scripts/migrate-to-clients.sh --check` and then
`--apply`.

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
- **One engine per client**: `go test ./clients/` (TestClientManifests) reads
  every `client.yaml` and fails when a client has two engine directories, when
  the populated directory disagrees with `engine:`, when `client:` differs from
  the folder name, or when the engine is not one of `dagnats|windmill|dagu`.
  A folder with no engine source yet is only logged: it is a draft, not a
  violation.
- **Client extraction**: a dedicated server is a deployment target, not a repo
  fork. Keep the client in the central repo; use `runtime: dedicated` and a
  target. Fork the repo only when the client requires its own governance.

## Phases

1. **Done — manifest.** `client.yaml` exists per client with `engine: dagnats`
   for everyone. Scripts read it; nothing moved.
2. **Done — engine adapter.** A Windmill client lives under `automation/windmill/`
   and the eval `source` adapter normalizes its jobs (`scripts/windmill-job-to-run.py`).
3. **Done — one layout.** `workflows/<client>/` → `clients/<client>/` moved in a
   single migration (`scripts/migrate-to-clients.sh`). The test package lives in
   `clients/`; scripts, hooks, CI and docs speak one layout. `workflows/` is
   accepted for reading only, so an ops repo can migrate when it wants.
4. **Current — dagnats is legacy.** New clients default to Windmill
   (`engine: windmill` in `client.yaml`). The dagnats runtime (worker, handlers,
   `automation/dagnats/`) stays until no client uses it, then it is deleted in a
   single commit: worker, handlers, `docker-compose` engine services, the
   `register` make targets. The spine, `evals/` and the scripts survive.

Until phase 4 ends, both engines are supported but a client runs exactly one.
Do not run two engines for the same client.
