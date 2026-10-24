# Sandbox policy (minimal, free, sufficient)

Parent rule: **this runtime never executes arbitrary scripts from JSON**
(`docs/writing-recipes.md` checklist, `AGENTS.md`). An external sandbox only
enters the picture when the work **leaves** dagnats. Everything else resolves
with `validate` + `go test` + a staging run.

## Decision in 3 questions

**Q1. Does the recipe use only known tasks?** (`internal/handlers/handlers.go`)
Yes → no external sandbox. Run:

```bash
dagnats workflow validate workflows/<client>/<wf>.json
go test ./workflows/   # executes every jq expression against samples
./scripts/register-workflows.sh <client>  # staging first
dagnats run start "<client>/<wf>" '<json>' --watch
```

Why this is enough: `transform.json` is pure gojq with no I/O
(`internal/handlers/transform_json.go`), `condition`/`skip_if` are
comparisons, `http.request`/`notify.*` have timeouts + permanent 4xx vs
retryable 5xx. The risk here is a wrong recipe, not an escape.

**Q2. Do you need a new Go handler?**
Only if ALL of the checklist applies (`docs/writing-recipes.md`). Then:

1. Table test in `internal/handlers/*_test.go` (mandatory).
2. Human review (an LLM may propose, never land without a human).
3. Run in the worker container (`distroless`, `Dockerfile.worker`) — that
   is your production sandbox, zero cost.
4. Flag the Windmill migration cost in the commit ("Windmill migration cost").

No E2B/Daytona here. Local `go test` + container already isolate.

**Q3. Is it an arbitrary script (LLM-generated Python/JS/shell, or legacy)?**
It does not enter dagnats. Two exits, both free:

- **One-off / test:** `scripts/sandbox-run.sh` (restricted local docker).
- **Recurring:** it became automation → decide in `docs/script-execution.md`
  (Dagu vs Windmill vs wrapping in a handler).

## What NOT to do

- Don't pay for E2B ($150/month + usage) or Daytona to run jq/recipes.
  Hobby E2B ($100 one-time) would only make sense if you started executing
  untrusted third-party code at scale — not the case today.
- Don't treat `llm.complete` output as code. It is **data**:
  validate the schema with `transform.json`/`condition` before using it.
  Prompt injection via SaaS data is the real risk, not missing microVMs.
- Don't use the IDE agent sandbox (Claude/Cursor) as multi-client control.
  Useful in the FDE day-to-day, but not auditable per client.

## `scripts/sandbox-run.sh`

```bash
# isolated by default: no network, read-only fs, no root, CPU/mem/pid caps
# the sandbox only mounts the repo — copy the script inside first if in /tmp
cp /tmp/migra.py ./scratch-migra.py
./scripts/sandbox-run.sh --allow-net=off ./scratch-migra.py
ls sandbox-out/  # script writes to ./sandbox-out/
```

No docker? Run `python3 script.py` as a dedicated user with no secrets in
env (`env -i`) — document as an exception in the client Fieldbook.
