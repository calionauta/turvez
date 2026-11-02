# Windmill (when a client needs UI, replay or code steps)

## Traps that cost the most time (read before your first push)

Four things fail in ways that look like success. Each one cost a real session
multiple round-trips to diagnose:

1. **A flow dir uploads only `flow.yaml` + `!inline` files.** A sibling helper
   imported as `from .send_slack import ...` works locally and **fails on the
   server**. Duplicate the helper per flow, or push it as a standalone script.
2. **Trigger/schedule pushes need the typed suffix** — `*.http_trigger.yaml`,
   `*.schedule.yaml`. A plain `*.yaml` is silently not treated as a trigger.
3. **`wmill flow delete` does not exist.** Use
   `DELETE /api/w/<ws>/flows/delete/<path>`. The wrong endpoint answers 404, so a
   cleanup that "worked" was a no-op — always re-list to confirm.
4. **A `.env` file is not read at runtime.** Recreate values as workspace
   variables/resources; the age file is bootstrap only.

Read `docs/script-execution.md#platform-choice-beyond-turvez` and
`docs/client-spine.md` first. Windmill is a per-client choice, not a second
engine for a dagnats client.

## Non-negotiable rules

- **Never `wmill sync push`.** It treats the local directory as the source of
  truth and deletes remote items missing locally. During the pilot it removed
  12 items from a live workspace. Use `wmill flow push <flow-dir> <remote-path>`
  for one item, after `wmill flow preview`.
- **One workspace per client** on a shared instance; a dedicated instance when
  the contract requires it. `client.yaml` records which.
- **Secrets are workspace variables/resources.** Values never enter git,
  flows or logs. SaaS integrations with OAuth/expiring tokens: **Nango**
  (`references/nango.md`, `docs/nango.md`), never raw tokens in Windmill.
- **Pin the CLI version** and use `WMILL_CONFIG_DIR` for disposable pilots, so
  a test does not overwrite the operator's workspace config.

## Test gate before run

Three layers, in order:

1. **`wmill lint --fail-on-warn --locks-required`** in CI. It validates flows,
   schedules and triggers against the OpenFlow/API schemas and fails on missing
   lockfiles. `locksRequired: true` in `wmill.yaml` also blocks `sync push`.
   Go scripts carry their dependencies inside the file, so they need no
   lockfile — only a TypeScript/Python graph does.
2. **`go test` for logic.** Put the domain logic in a Go package and unit-test
   it; the Windmill Go script is a thin wrapper. This is the closest equivalent
   to the dagnats `go test ./clients/` gate.
3. **CI test scripts** (`// test: flow/...` annotation) for integration: run
   the flow with fixtures and assert the result. PR gating is EE.
   `wmill flow preview --data … --step <id>` runs local files without deploying;
   use it for one-step checks, not as an offline gate.

Flow expressions are JavaScript, not jq. Keep them trivial and move logic into
scripts; a syntax error in an expression is only caught by `wmill lint` if the
schema sees it, otherwise by preview/CI.

## Language: Go for the nodes, TypeScript/Python only for the graph

Normative policy: `docs/language-policy.md`. Short version:

| What | Language | Why |
|---|---|---|
| Script node (the work: fetch, transform, notify, gate) | **Go** | the compiler validates the inputs; Windmill derives the schema and the generated UI from `func main(...)` |
| Flow graph | `flow.yaml` | the default; one file per flow |
| WAC orchestrator | TypeScript or Python | Windmill only implements `workflow()`/`taskScript()`/`taskFlow()` there — a product limit, not a preference |

Never a JS/TS/Python *node*. `wmill lint` does not read the script body, so a
Python step with an undefined name lints clean and fails at run time — after the
push, often after a schedule fires. Go fails locally:

```bash
./scripts/lint-scripts.sh   # gofmt + compile + main + input_transforms, ~1s
```

A Go node, in full:

```go
package inner

// Parameters become the step's input schema and generated UI.
func main(leadID string) (interface{}, error) {
	return map[string]any{"lead_id": leadID}, nil
}
```

The one legitimate non-Go file is a **thin orchestrator** that only sequences Go
scripts. It may branch and loop over results; it may not do HTTP, reshape JSON,
format strings, or touch secrets:

```typescript
import { taskScript, workflow } from "windmill-client";

const extract = taskScript("f/acme/extract_contacts");
const load = taskScript("f/acme/load_warehouse");

export const main = workflow(async (since: string) => {
  return await load({ rows: await extract({ since }) });
});
```

The gate enforces it: a `.py`/`.ts` under `automation/windmill/` passes only if
it imports the workflow primitives and calls `taskScript()`/`taskFlow()`; a step
declared `language: python3|deno|bun` fails validation. When in doubt, use
`flow.yaml` and write no orchestrator at all.

Bash stays out of both roles: no types, no tests.

## YAML or JSON for WAC?

Windmill's flow directory uses `flow.yaml`; that is the supported authoring
format. Use YAML for WAC, JSON for dagnats recipes and machine artifacts.

For an LLM, YAML is the higher-risk format (indentation, colons in strings).
Mitigate it:

- generate the smallest flow that works, then `wmill lint`;
- quote every `summary`/description containing `:`;
- keep expressions out of YAML: put them in scripts with typed inputs;
- never hand-edit a pushed flow; edit the local file and push that one item.

## SaaS integrations: use Nango, not raw tokens

Never put third-party OAuth access tokens or refresh tokens into Windmill
variables. Tokens expire in 1-24 hours; refresh token rotation triggers race
conditions across parallel flow executions.

Instead, route calls through the **Nango Request Proxy** (`references/nango.md`):
- Windmill workspace stores only `NANGO_HOST` and `NANGO_SECRET_KEY`.
- Go nodes call `${NANGO_HOST}/proxy/...` with `Provider-Config-Key` (e.g. `hubspot`)
  and `Connection-Id: <client>`.
- Nango refreshes expired tokens centrally, attaches credentials, and handles rate limits.
- Full guide and examples: `references/nango.md` and `docs/nango.md`.

## Eval

Normalize a flow job into the shape the selector already reads:

```bash
python3 scripts/windmill-job-to-run.py --job-id <id> --out run.json
python3 scripts/select-cases.py --runs run.json --out clients/<client>/evals/cases
```

## Flow directories: what actually gets uploaded

A flow directory is uploaded as `flow.yaml` plus the scripts it references with
`!inline <file>`. **Any other file in that directory is NOT uploaded.** Concretely:

- A sibling helper imported by another script in the same flow dir (`from .send_slack
  import ...`) works locally and **fails on the server**. Duplicate the helper per
  flow directory, or move it to a shared script and reference it by path.
- So a helper you want in two flows must exist twice, or be a standalone pushed
  script. Do not design cross-flow imports assuming the folder is a package.

### Trigger and schedule files need exact suffixes

`wmill trigger push` and `wmill schedule push` only accept the documented
filenames: `<name>.http_trigger.yaml`, `<name>.schedule.yaml` (and the other
typed suffixes). A generic `*.yaml` is silently not treated as a trigger.

### `wmill flow delete` does not exist

To remove a pushed flow use the REST API:
`DELETE /api/w/<workspace>/flows/delete/<path>`. A cleanup that "worked" may have
been a no-op — verify by listing afterwards.

## Replay and debug

- `wmill job get <id>` shows the step tree with per-step status and job ids.
- `wmill job logs <id>` shows step logs.
- UI: "Re-start from X" restarts from a top-level step; "Test this step" and
  "Test up to step" run part of the flow.
- Restart re-runs the step's side effects. Make steps idempotent, or replay
  only steps that are safe.

## Hosting and license

- Cloud is the fastest pilot; data lives in Windmill's account.
- Self-host needs Postgres + server + workers. The EE image requires a license;
  the community edition is built from source. AGPLv3 core; SSO, durable audit,
  concurrency limits and critical alerts are EE.
- Alerts are code in the free edition: a flow/error handler calling Slack,
  Telegram, Discord or an email API, plus an external dead-man switch.
