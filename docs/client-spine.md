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

## Choosing the engine

Ask these in order. The first **yes** decides; a tie means the client does not
need Windmill yet, so it starts on dagnats.

| # | Question | Yes → | Why |
|---|---|---|---|
| 1 | Does the client need **generated UI/forms/apps** for the automation, or a teammate who is not you operating it? | **Windmill** | dagnats has no app surface; building one is a separate product |
| 2 | Does diagnosing a bad step require **re-running one step**, or reading each step's log? | **Windmill** | pilot: structured error (`step_id`, stack, job id) + `flow preview --step` is materially better than base64 step output |
| 3 | Is the logic **non-trivial code** (parsing, branching, an SDK) that a non-Go dev must own? | **Windmill** | TS/Python/Go code steps beat a jq expression the 12 handlers cannot express |
| 4 | Does the contract demand **typed contracts, native approvals, no arbitrary code, one binary**, or AGPL is a contract problem? | **dagnats** | dagnats' whole point; `go test` gates it before any run |
| 5 | Is the client **regulated/residency-bound**, or does it require its own instance/DB? | **dagnats** (or Windmill self-hosted, see `docs/isolating-a-client.md`) | instance isolation is a deployment choice, not an engine choice |

If none apply, the client is a plain integration job and **dagnats is the
cheaper answer**: one binary, no Postgres, a `go test` gate. That is not a
legacy statement; it is the reason dagnats exists.

**Do not flip the default to Windmill for everyone.** The pilot's verdict was
explicit: it justified offering Windmill **per client** (UI/replay clients) and
keeping dagnats for typed/regulated/light ones. A default-for-everyone flip is
a decision with Postgres, AGPL and governance consequences and needs a
measured win across most clients first — not one pilot.

Cost of being wrong, per direction:

- dagnats when Windmill was right → you pay in debugging time; step output is
  base64 and there is no per-step replay.
- Windmill when dagnats was right → you took on Postgres, AGPL/EE governance
  and a workspace per client for a workload one binary handled.

The second is the more expensive mistake, which is why the default stays where
the contract is simple.

## Engine differences stay at the edges

| Concern | dagnats | Windmill | Dagu |
|---|---|---|---|
| Source | `automation/dagnats/*.json` | `automation/windmill/*.flow/` | `automation/dagu/*.yaml` |
| Deploy | `workflow register` | `wmill flow push` | dags dir + `dagu start-all` |
| Secrets | env + age | Variables/Resources per workspace | host env |
| Triggers | `dagnats trigger` | schedules/webhooks | schedule YAML |
| Run history | NATS KV + events | job API/Postgres | file-based runs |
| Eval source | `run inspect --json` | job result API | run log |

`evals/` does not change. `select-cases.py` stays dagnats-only; a Windmill run
is converted into the same shape by a separate adapter,
`scripts/windmill-job-to-run.py`, so the normalizer is explicit instead of a
`--source` switch that would tempt one script to know two APIs:

```json
{"run_id":"...","workflow":"acme.<wf>","status":"failed",
 "created_at":"...","steps":[{"id":"...","status":"...","output":...}]}
```

```bash
# windmill -> normalized run -> cases
python3 scripts/windmill-job-to-run.py --job-id <id> --out run.json
python3 scripts/select-cases.py --runs run.json --out clients/acme/evals/cases
```

Promptfoo keeps reading the same cases either way.

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
   and `scripts/windmill-job-to-run.py` normalizes its jobs into the shape
   `select-cases.py` already reads.
3. **Done — one layout.** `workflows/<client>/` → `clients/<client>/` moved in a
   single migration (`scripts/migrate-to-clients.sh`). The test package lives in
   `clients/`; scripts, hooks, CI and docs speak one layout. `workflows/` is
   accepted for reading only, so an ops repo can migrate when it wants.
4. **Current — two engines, one per client, no default.** The engine is a
   **per-client decision**, not a rank order: `client.yaml` records it and the
   criteria live in § "Choosing the engine". dagnats remains the default for
   the work it was built for (typed contracts, native approvals, a single
   binary, a `go test` gate); Windmill is the answer when the client needs
   UI/forms, step-level replay, test-one-step, or code steps in TS/Python/Go.
   Neither engine is deprecated by fiat.

   The windmill pilot (`spike`, measured against pre-decided weighted criteria)
   found Windmill materially better at **diagnosis and step replay** — exactly
   where an LLM-authored flow is most likely to be wrong — and dagnats better
   at the **pre-run test gate, footprint and licensing**. That is a reason to
   offer both per client, not to flip a default.

   **Retirement trigger (unchanged):** the dagnats runtime goes away only when
   no client uses it, in a single commit — worker, handlers, `docker-compose`
   engine services, the `register` make targets. The spine, `evals/` and the
   scripts survive. Flipping the default is not a retirement step.

Both engines are supported and a client runs exactly one. Do not run two
engines for the same client.
