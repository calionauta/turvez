# First run (verified end to end)

The sequence a fresh checkout needs, including the traps that cost a debugging
session on 2026-10-29. Everything below was executed against this repo.

## 0. Prerequisites

```bash
./scripts/setup.sh                 # go, dagnats CLI, docker, env, age, python
./scripts/setup.sh --init personal # creates .env and env/personal.env (gitignored)
```

## 1. Pick host ports before `make up`

Compose publishes `${DAGNATS_HTTP_PORT:-8080}` and `${DAGNATS_NATS_PORT:-4222}`.
If something already listens there (another stack, a port forward), the stack
still starts but **the CLI silently reads the other server**: runs stay
`queued` and `workers list` is empty.

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN   # or: ss -ltn | grep 8080
export DAGNATS_HTTP_PORT=31080 DAGNATS_NATS_PORT=31222
```

## 2. Console needs an auth mode in a container

Inside Docker the console binds `0.0.0.0`, and without auth it refuses to
serve (`console_disabled`). Set a password (HTTP basic auth, any user) or
trust a forwarding proxy:

```bash
export DAGNATS_CONSOLE_PASSWORD            # set the value (e.g. from .env); or:
# export DAGNATS_CONSOLE_TRUST_FORWARDED_AUTH=true
```

## 3. Start the stack, point the CLI at it

```bash
make up                                    # or: docker compose up -d --build
export DAGNATS_NATS_URL=nats://127.0.0.1:${DAGNATS_NATS_PORT:-4222}
```

The CLI is **NATS-first**. `DAGNATS_API_URL` (set in `docker-compose.yml` for
the worker's approval shim) does not move the CLI.

## 4. Ops repo + client

```bash
./scripts/new-ops-repo.sh ../turvez-ops
TURVEZ_OPS_DIR=../turvez-ops ./scripts/new-client.sh acme
```

## 5. Recipe traps (JSON, not YAML)

- `metadata` is `map[string]string`: numbers and objects must be strings
  (`"timeout_ms": "15000"`, `"headers": "{\"Accept\":\"application/json\"}"`).
- `skip_if.step_id` must be listed in `depends_on`.
- With several deps the input is keyed by parent id (`.["triage"].content`).

Validate before registering:

```bash
dagnats workflow validate ../turvez-ops/workflows/acme/<wf>.json
WORKFLOWS_DIR=../turvez-ops/workflows go test ./workflows/
```

## 6. Register and run

```bash
docker compose cp ../turvez-ops/workflows/acme/<wf>.json dagnats:/tmp/wf.json
docker compose exec -T dagnats dagnats workflow register /tmp/wf.json
docker compose exec -T dagnats dagnats run start acme/<wf> '{}' --json
```

(From the host, `dagnats workflow register` works too once
`DAGNATS_NATS_URL` points at the stack.)

## 7. Observe

```bash
dagnats run list --workflow=acme/
dagnats run inspect <id> --json
dagnats run events <id>
dagnats trace <id>
dagnats logs
```

Step outputs are base64:

```bash
jq -r '.run.steps["build-prompt"].output' run.json | base64 -d | jq .
```

Console: `http://127.0.0.1:${DAGNATS_HTTP_PORT}/console/` — HTTP basic auth,
**any username**; the password comes from `DAGNATS_CONSOLE_PASSWORD`.

Known upstream limit (dagnats v0.0.30): the console's workflow-detail link
`/console/workflows/<client>/<wf>` returns **404** when the name contains `/`
(`dispatchWorkflows` splits on the first slash and treats the rest as an
action). Workaround: open the run instead — `/console/runs?id=<run-id>` — or
use the CLI (`dagnats run list --workflow=<client>/`).

## 8. Eval

Per client (`evals/<client>/` in the ops repo, scaffolded by `new-client.sh`):

```bash
DAGNATS_NATS_URL=... python3 scripts/select-cases.py --dagnats --out evals/acme/cases --include-ok 3
make eval-client CLIENT=acme TURVEZ_OPS_DIR=../turvez-ops
make eval-client-view CLIENT=acme TURVEZ_OPS_DIR=../turvez-ops
```

Eval params must mirror production (`max_tokens`); a reasoning model with too
small a budget returns empty content and the eval fails for the wrong reason.

## Traps observed, in order

| Symptom | Cause | Fix |
|---|---|---|
| `docker compose up` prints CLI help, exits 1 | image had no `CMD` | `CMD ["serve"]` in `Dockerfile.dagnats` |
| runs stuck `queued`, no workers | CLI pointed at another NATS (port already forwarded) | `DAGNATS_NATS_URL` + free host ports |
| `/console/` → `console_disabled` | container binds 0.0.0.0, no auth | `DAGNATS_CONSOLE_PASSWORD` |
| `cannot unmarshal object into ... headers` | metadata values must be strings | JSON-encode them |
| `empty completion (finish=length)` | reasoning ate `max_tokens` | raise it; the gate catches the rest |
| eval fails with empty output | eval config used a smaller `max_tokens` than production | mirror production params |
