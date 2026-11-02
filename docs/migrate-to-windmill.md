# Migrate to Windmill (the default engine, no dependency)

Windmill is **not** a runtime dependency of this repo. It is the **default
engine for new clients** (`engines.json` → `"default": "windmill"`, see
`docs/client-spine.md`), and this document is both the step mapping for a dagnats
client whose contract changed and the reference for what a new Windmill client
starts from.

## Let the converter do the mechanical half

```bash
./scripts/convert-to-windmill.py <client-id>          # -> clients/<id>/converted-windmill/
./scripts/convert-to-windmill.py <client-id> --force  # re-run over a previous attempt
```

It reads `automation/dagnats/*.json` and emits a WAC flow skeleton per recipe,
plus a `REPORT.md` counting how many steps per recipe need a human. It **never**
writes into `automation/windmill/`, so a half-finished conversion cannot be
pushed by accident.

What it deliberately will not do — do not expect otherwise:

- **jq → Go.** The expression is carried verbatim into a comment and
  marked `TODO(human)`. A wrong translation that looks migrated is worse than a
  TODO.
- **`condition` / `skip_if` predicates.** Same reason.
- **Custom Go handlers.** Any task outside the mapping table below appears as an
  unmapped step. Each one is the long pole of the migration; budget it with the
  operator before promising a date.

The table below is what the converter implements. Read it before trusting a
generated skeleton.

Source of truth stays git: `clients/<client-id>/automation/dagnats/*.json` plus the
generic Go handlers under `internal/handlers/`. The client's `client.yaml`
records `engine: windmill` once the cutover is complete; Windmill source goes
under `clients/<client-id>/automation/windmill/`. Never migrate
historical run logs — only definitions, triggers, and secret *names*.

## Deterministic step mapping

| Our step type | Windmill target |
|---------------|-----------------|
| http.request | Go script step or native HTTP action (method, url, headers, body map 1:1) |
| transform.json | Go step with the jq-equivalent (`encoding/json` + typed structs); the `expression` becomes the step body |
| condition / skip_if | if/else branches in the flow, or early return in a Go step; `field/op/value` becomes the branch predicate |
| map (engine `type: map` or `map` task) | for-loop step, or parallel map/branch-per-item in the flow |
| delay / sleep | sleep step, or scheduled continuation per Windmill suspend/resume patterns |
| request.approval / approval | Windmill approval step, or form + suspend/resume; `OPS_*` shim becomes a Windmill webhook trigger URL |
| notify.webhook / slack / telegram / discord | Go step doing the HTTP POST (same payload shape) |
| dlq.check | scheduled Go script reading the dead-letter queue depth (API/CLI equivalent) |
| custom Go handler | rewrite as a **Go** Windmill script; list each handler with its inputs/outputs — it is the long pole |

Script nodes are Go (`docs/language-policy.md`). TypeScript/Python is only for a
workflow-as-code orchestrator that dispatches to those nodes, and only when the
graph needs loops or fan-out that `flow.yaml` expresses badly.

## Per-client checklist

1. Read `clients/<client-id>/automation/dagnats/*.json` — one Windmill flow per recipe,
   preserving step order and `depends_on` fan-in (multi-dep steps become
   a join: pass each parent's result as a named flow input).
2. Recreate secrets in Windmill from `env/<client-id>.env.example`
   (names only) + real values from the operator vault — same names,
   new store. Verify no value ever lived in the JSON (grep for `sk-`,
   `xoxb-`, `hooks.slack`).
3. Convert triggers: cron expressions copy verbatim; webhook paths
   become Windmill webhook triggers (repoint SaaS sender URLs at
   cutover, exactly like `docs/isolating-a-client.md` step 7).
4. Convert approvals: native `approval` steps → Windmill approval;
   `request.approval` signal flows → form + resume token.
5. Cut over per client, keep dagnats running until the Windmill flow
   shows green runs; rollback = repoint webhooks back.

## Example: lead pipeline rewritten (Go nodes, thin graph)

Source: `clients/example-client/automation/dagnats/hubspot-lead-to-slack.json`
(build-fetch → fetch-lead → build-notify → notify).

Each step becomes a Go script in `automation/windmill/<flow>/`; the flow wires
them by `!inline`. The step signature is the contract: Windmill derives the input
schema and the generated UI from `func main(...)`.

```go
// clients/<client>/automation/windmill/lead-pipeline/build_fetch.go
package inner

// Step 1 — build-fetch (was: transform.json metadata.expression).
// The envelope tolerance from the recipe becomes a plain struct.
type LeadInput struct {
	LeadID string `json:"lead_id"`
}

func main(in LeadInput) (interface{}, error) {
	return map[string]any{
		"method": "GET",
		"url":    "https://api.hubapi.com/crm/v3/objects/contacts/" + in.LeadID,
	}, nil
}
```

```go
// clients/<client>/automation/windmill/lead-pipeline/build_notify.go
package inner

// Step 3 — build-notify (was: transform.json).
type Lead struct {
	Properties struct {
		Firstname string `json:"firstname"`
		Lastname  string `json:"lastname"`
		Email     string `json:"email"`
	} `json:"properties"`
}

func main(lead Lead) (interface{}, error) {
	p := lead.Properties
	return map[string]any{
		"text": "New lead: " + p.Firstname + " " + p.Lastname + " <" + p.Email + ">",
	}, nil
}
```

```yaml
# clients/<client>/automation/windmill/lead-pipeline/flow.yaml
summary: "lead pipeline"
value:
  modules:
    - id: build_fetch
      value:
        type: rawscript
        language: go
        input_transforms: {}
        content: '!inline build_fetch.go'
    # fetch_lead, build_notify, notify follow the same shape
```

Fetching and posting are `net/http` in Go, and the credentials come from the
workspace (or from Nango — `docs/nango.md`), never from the script body.

If the same pipeline needs per-item parallelism that `flow.yaml` expresses badly,
add **one** thin orchestrator that only dispatches:

```typescript
// clients/<client>/automation/windmill/lead_pipeline.ts
import { parallel, taskScript, workflow } from "windmill-client";

const runOne = taskScript("f/<client>/lead_pipeline");

export const main = workflow(async (leads: string[]) => {
  return await parallel(leads, (lead) => runOne({ lead }), { concurrency: 3 });
});
```

That file is the only place TypeScript/Python is allowed, and it contains no
logic of its own (`docs/language-policy.md`).

## Portability rule (applies NOW, not at migration time)

- Agents must keep recipes in portable step types: the eight generic
  tasks + engine `map`/`sleep`/`approval`/`skip_if`. Anything expressible
  in the recipe stays in the recipe.
- Any new Go handler MUST be flagged in its PR/commit as **"Windmill
  migration cost"**: list its inputs, outputs, and which Windmill
  script type would replace it. Prefer recipe-only solutions precisely
  to keep this table short — today it is empty (all handlers above map
  to native Windmill steps; only future *custom* handlers add cost).

## Topology: one central instance or one per client?

Workspaces are a **logical** boundary, not an instance boundary. One Windmill
with a workspace per client shares the server, PostgreSQL, workers and upgrade
window. That is fine for many clients, and cheaper to operate than N instances.

| | Central + workspace per client | Instance per client |
|---|---|---|
| Isolation | logical: secrets, folders, ACLs, runs | process, database, network, upgrade window |
| Ops cost | 1 Postgres, 1 server, 1 worker pool | N stacks; N backups, N upgrades |
| Blast radius | one bad upgrade or DB incident hits all | contained to one client |
| Noisy neighbour | shared worker pool; concurrency limits are EE | isolated |
| Superadmin | sees every workspace | only that instance |
| Contract fit | SMB, normal data | regulated, data residency, client-owned infra |

Start central with workspaces when the clients are SMB and the contract does
not require instance isolation. Split when any of these is true:

- the client requires data residency or a separate database;
- the client owns the infrastructure and you operate it;
- one client's volume starves the others (worker pool, Postgres);
- an upgrade must be scheduled per client;
- the contract says "separate instance".

Splitting is mechanical: Windmill git sync exports flows per workspace, secrets
are recreated per workspace, and webhook URLs are repointed per client. The
per-client checklist above still applies; the only change is the target
instance. Do not run a central instance and a per-client instance for the same
client.
