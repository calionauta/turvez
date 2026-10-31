# Windmill (when a client needs UI, replay or code steps)

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
  flows or logs. OAuth-heavy clients: Nango (`references/auth-layers.md`).
- **Pin the CLI version** and use `WMILL_CONFIG_DIR` for disposable pilots, so
  a test does not overwrite the operator's workspace config.

## Test gate before run

Three layers, in order:

1. **`wmill lint --fail-on-warn --locks-required`** in CI. It validates flows,
   schedules and triggers against the OpenFlow/API schemas and fails on missing
   lockfiles. `locksRequired: true` in `wmill.yaml` also blocks `sync push`.
2. **`go test` for logic.** Put the domain logic in a Go package and unit-test
   it; the Windmill Go script is a thin wrapper. This is the closest equivalent
   to the dagnats `go test ./workflows/` gate.
3. **CI test scripts** (`// test: flow/...` annotation) for integration: run
   the flow with fixtures and assert the result. PR gating is EE.
   `wmill flow preview --data … --step <id>` runs local files without deploying;
   use it for one-step checks, not as an offline gate.

Flow expressions are JavaScript, not jq. Keep them trivial and move logic into
scripts; a syntax error in an expression is only caught by `wmill lint` if the
schema sees it, otherwise by preview/CI.

## Go, TypeScript or Python for scripts?

| Use | Language | Why |
|---|---|---|
| Domain logic, typed contracts, shared with the repo | **Go** | `go test`, type checking, same language as the handlers; needs a lockfile |
| Thin glue, HTTP calls, string/JSON reshaping | TypeScript or Python | faster iteration, smaller locks |
| Anything non-trivial | never Bash | no types, no tests |

Go is the right default when the script is part of the automation's contract
and you want a real unit-test gate. TS/Python are fine for one-purpose glue.
Do not spread one client's logic across three languages.

## YAML or JSON for WAC?

Windmill's flow directory uses `flow.yaml`; that is the supported authoring
format. Use YAML for WAC, JSON for dagnats recipes and machine artifacts.

For an LLM, YAML is the higher-risk format (indentation, colons in strings).
Mitigate it:

- generate the smallest flow that works, then `wmill lint`;
- quote every `summary`/description containing `:`;
- keep expressions out of YAML: put them in scripts with typed inputs;
- never hand-edit a pushed flow; edit the local file and push that one item.

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
