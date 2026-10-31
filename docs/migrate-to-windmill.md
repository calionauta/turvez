# Migrate to Windmill (escape hatch, no dependency)

Windmill is **not** a runtime dependency of this repo. This document
exists so a future migration of client automations to Windmill
"workflow as code" is mechanical, not a rewrite from scratch.

Source of truth stays git: `workflows/<client-id>/*.json` plus the
generic Go handlers under `internal/handlers/`. The client's `client.yaml`
records `engine: windmill` once the cutover is complete; Windmill source goes
under `workflows/<client-id>/windmill/` in the phase-1 layout. Never migrate
historical run logs — only definitions, triggers, and secret *names*.

## Deterministic step mapping

| Our step type | Windmill target |
|---------------|-----------------|
| http.request | HTTP script step or native HTTP action (method, url, headers, body map 1:1) |
| transform.json | TypeScript/Python step with jq-equivalent (`jq` lib or JSON ops); the `expression` becomes the step body |
| condition / skip_if | if/else branches in the flow, or early return in a script step; `field/op/value` becomes the branch predicate |
| map (engine `type: map` or `map` task) | for-loop step, or parallel map/branch-per-item in the flow |
| delay / sleep | sleep step, or scheduled continuation per Windmill suspend/resume patterns |
| request.approval / approval | Windmill approval step, or form + suspend/resume; `OPS_*` shim becomes a Windmill webhook trigger URL |
| notify.webhook / slack / telegram / discord | HTTP POST step to the webhook URL (same payload shape) |
| dlq.check | scheduled script reading the dead-letter queue depth (API/CLI equivalent) |
| custom Go handler | rewrite as a Windmill script (TypeScript/Python/Go); list each handler with its inputs/outputs — it is the long pole |

## Per-client checklist

1. Read `workflows/<client-id>/*.json` — one Windmill flow per recipe,
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

## Example: lead pipeline rewritten (illustrative TypeScript)

Source: `workflows/example-client.hubspot-lead-to-slack.json`
(build-fetch → fetch-lead → build-notify → notify).

```typescript
// Windmill flow "example-client.hubspot-lead-to-slack", 4 script steps.
// Secrets (wmill resources): ACME_HUBSPOT_TOKEN, ACME_SLACK_WEBHOOK_URL.

type LeadInput = { lead_id: string } | { data: { lead_id: string } };

// Step 1 — build-fetch (was: transform.json metadata.expression).
export async function buildFetch(input: LeadInput) {
  const lead_id = (input as any).data?.lead_id ?? (input as any).lead_id;
  return {
    method: "GET",
    url: `https://api.hubapi.com/crm/v3/objects/contacts/${lead_id}?properties=firstname,lastname,email`,
    headers: { Authorization: `Bearer ${ACME_HUBSPOT_TOKEN}` },
  };
}

// Step 2 — fetch-lead (was: http.request). Native HTTP action equivalent.
export async function fetchLead(req: { method: string; url: string; headers: any }) {
  const res = await fetch(req.url, { method: req.method, headers: req.headers });
  if (!res.ok) throw new Error(`hubspot ${res.status}: ${await res.text()}`);
  return await res.json();
}

// Step 3 — build-notify (was: transform.json).
export async function buildNotify(lead: any) {
  const p = lead.properties ?? {};
  return {
    text: `New lead: ${p.firstname ?? "unknown"} ${p.lastname ?? ""} <${p.email ?? "no-email"}>`,
  };
}

// Step 4 — notify (was: notify.slack).
export async function notify(msg: { text: string }) {
  const res = await fetch(ACME_SLACK_WEBHOOK_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(msg),
  });
  if (!res.ok) throw new Error(`slack ${res.status}`);
  return { ok: true };
}
```

(Pseudo-code: adapt resource/secret access and HTTP helpers to the
Windmill version in use; the data flow is the normative part.)

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
