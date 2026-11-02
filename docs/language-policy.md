# Language policy: Go for the nodes, TypeScript/Python only for the graph

Status: normative. Enforced by `scripts/lint-scripts.sh`, which runs inside
`make validate` and CI.

## The rule

| What | Language | Why |
|---|---|---|
| A **script node** — the code that does the work (fetch, transform, notify, gate) | **Go** | The compiler validates the inputs, so Windmill's schema and generated UI come from a real signature instead of a guess. Same language as the dagnats handlers, so logic can move between engines. |
| A **dagnats recipe** | JSON + the 12 registered Go tasks | Recipes are data; the handlers are Go and gated by `go test`. |
| A **dagnats handler** (new task) | **Go** | `internal/handlers/`, table tests, human lands it. |
| A **Windmill flow graph** | `flow.yaml` | The default. One file, one flow, pushed one item at a time. |
| A **Windmill workflow-as-code orchestrator** | TypeScript or Python (**the only exception**) | Windmill only implements `workflow()` / `task()` / `taskScript()` / `taskFlow()` in those two languages. This is a product limit, not a preference. |
| Repo tooling (`scripts/*.py`, the gates, the eval loop) | Python, unchanged | Out of scope: it never reaches a client's runtime. |
| A sandbox one-off (level 1) | Anything, in `scratch/` | Out of scope: it runs once and never becomes a node. |

No JavaScript, no Bash for logic, no PHP, no Rust in a script node. If a client
needs a language the engine supports but this table does not, that is an operator
decision, not an agent decision — stop and ask.

A local model (SLM rewrite, cross-encoder reranker, embeddings) is an
**external inference service**, not a script node: a Go node calls it over HTTP.
There is no Python/TS node for inference, and the gate rejects one.

## Why Go for the nodes

The failure this prevents is specific: an LLM writes a Python step, the flow
lints clean (`wmill lint` checks the schema, not the body), the push succeeds,
and the run fails at 9am because a name was undefined or an input was never
passed. TypeScript/Python are checked at *run* time in Windmill. Go is checked
at *build* time, locally, in under a second.

- **The signature is the contract.** Windmill derives the input schema and the
  auto-generated UI from `func main(...)`'s parameters. Wrong types do not
  produce a plausible-looking form; they fail `go build`.
- **One language for the domain logic.** A `transform.json` rewritten in Go can
  be unit-tested and, if the client ever moves to dagnats, ported instead of
  rewritten.
- **The gate can see it.** `scripts/lint-scripts.py` runs `gofmt`, compiles the
  node, checks `func main` exists and returns `(interface{}, error)`, and
  cross-checks its parameters against the step's `input_transforms` — the class
  of bug `wmill lint` cannot catch.

## The exception: Windmill's graph is TypeScript or Python

Windmill's workflows-as-code are TypeScript or Python only
(https://www.windmill.dev/docs/core_concepts/workflows_as_code: `workflow()`,
`task()`, `step()`, `sleep()`, `waitForApproval()`, `parallel()`, and
`taskScript()`/`taskFlow()` for dispatching to existing scripts and flows by
path). There is no Go WAC surface.
Fighting that produces a worse system than using it deliberately.

So the graph is allowed — and it is **thin by contract**:

```typescript
// clients/<client>/automation/windmill/nightly_sync.ts
import { taskScript, workflow } from "windmill-client";

const extract = taskScript("f/<client>/extract_contacts");
const transform = taskScript("f/<client>/transform_contacts");
const load = taskScript("f/<client>/load_warehouse");

export const main = workflow(async (since: string) => {
  const raw = await extract({ since });          // Go
  const rows = await transform({ raw });          // Go
  return await load({ rows });                    // Go
});
```

What a graph file may contain:

- the `workflow()` entry point and the primitives (`task`, `step`, `sleep`,
  `parallel`, `waitForApproval`, `getResumeUrls`);
- `taskScript(...)` / `taskFlow(...)` calls that dispatch to Go scripts;
- branching, loops and error handling **over the results** of those calls.

What it may not contain:

- HTTP calls, JSON reshaping, string formatting, retries with business rules,
  or any other work a Go node could do;
- secrets access, token refresh, or credential plumbing;
- a second copy of logic that already exists in a Go node.

The gate enforces the boundary mechanically: a `.py`/`.ts` file under
`automation/windmill/` is accepted **only** if it imports the workflow
primitives and calls `taskScript()`/`taskFlow()`. Everything else must be Go, and
a flow step declared as `language: python3`/`deno`/`bun` fails validation.

When in doubt, the flow graph is `flow.yaml` and there is no orchestrator file at
all. Reach for WAC only when the graph needs loops or fan-out that YAML expresses
badly.

## dagnats: already Go, and this is what keeps it that way

dagnats has no arbitrary-code surface: a recipe is JSON over twelve Go tasks
(`docs/writing-recipes.md`), and a new capability is a new Go handler with table
tests. That is the strongest version of this policy and it must not be relaxed
to "just run this script" in a recipe.

## How it is enforced

```bash
./scripts/lint-scripts.sh     # Go compile + gofmt + bindings + policy; no server
make validate                 # includes the line above when a windmill dir exists
go test ./...                 # dagnats recipes, tenancy, secrets
```

CI runs all three. A language-policy violation fails with the file and the
reason, not a generic "lint failed".

## Migrating existing code

- A Python/TypeScript step with logic → rewrite as Go, then delete the old file.
  Do not leave both: the gate fails on the orphan.
- A WAC orchestrator that has grown logic → move the logic into Go nodes and
  leave the orchestrator as dispatch only.
- `scripts/convert-to-windmill.py` already emits Go skeletons, so a dagnats
  migration starts inside the policy instead of outside it.
