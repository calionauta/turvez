# Decision tree: where does this automation belong?

Q0 first: which repo am I in? `go.mod` only = template (public, no real
clients — refuse, point to turvez-ops via docs/ops-repo.md). `TURVEZ_PIN`
only = ops repo (private, any client OK).

Q0.5: which engine is this client on? Read `clients/<client>/client.yaml`.
`engine: windmill` → `references/windmill.md`. `engine: dagnats` → the levels
below. The **default is Windmill** (`engines.json`), but the manifest always
wins: an explicit `engine: dagnats` is honored. dagnats is chosen when the
contract needs typed contracts, native approvals, no arbitrary code, or when
AGPL/Postgres is a problem — see `docs/client-spine.md#choosing-the-engine`. If
the client has no manifest, run that decision first and write the answer down.

Q0.6: is this client on the right host? `./scripts/verify-isolation.sh <client>`
prints what `runtime:`/`target:` actually say. Ships `shared` but the client
needs its own box → `./scripts/extract-client.py <client>` packages the move
(read-only; it prints commands, you run them). Never answer "is it isolated?"
from memory — those fields were once written and never read.

Q0.7: engine change requested? dagnats → Windmill has a converter
(`scripts/convert-to-windmill.py <client>`, skeleton + explicit TODOs, lands in
`converted-windmill/`). The reverse has none. Never migrate unasked, and never
run both engines for one client.

## Q0.8 — is the ask "answer questions over our data"?

Split **documents** from **systems of record** before choosing anything
(`references/retrieval.md`): a system of record is a tool call through Nango (the
fixed proxy in a recipe/flow; an agent session/MCP only when an agent must
choose tools, which is Q1.5). A document corpus follows the ladder: long context
→ Bleve BM25 in Go + a curated glossary → OpenWN-PT → FastText only behind an
eval. Embeddings only when the eval proves lexical + synonyms insufficient.
A vector DB is never the first answer.

Then, **for a dagnats client only**, ask the levels below in order. First
"yes" wins. Full rationale: `docs/script-execution.md`. A Windmill client stops
at `references/windmill.md` — the level questions are dagnats-shaped.

## Q1.5 — agent loop gate

Does it need an agent that loops with tools (shell, files, browser) until
done? → **not** a recipe, **not** Dagu level 2. It is a new platform
decision (Dagu 2.x `llm.*`/`harness.run`/agent DAGs, or a dedicated
harness). Why: `docs/writing-recipes.md` → "Why AgentLoop/planner stay
upstream-only".

## Level 0 — dagnats recipe

Q: fits the 12 handlers (`http.request`, `transform.json`, `condition`,
`map`, `delay`, `notify.*`, `request.approval`, `llm.complete`, `dlq.check`)?
Yes → write `clients/<client>/automation/dagnats/*.json`, `validate` + `go test`, propose `register`.
On a Windmill client the same 12 shapes are flow steps (`docs/migrate-to-windmill.md`
has the mapping); do not add a dagnats recipe to a Windmill client.
Ex.: HubSpot deal → Slack + Notion; invoice VIP split; refund with 48h approval gate.

## Level 1 — sandbox one-off (zero residue)

Q: code that runs once (LLM-generated parser, CSV cleanup, API probe)?
Yes → read it, `cp` into repo, `./scripts/sandbox-run.sh --allow-net=off ./scratch-x.py`,
delete after. Never enters a recipe. Details: `docs/sandbox-policy.md`.

## Level 2 — Dagu ad-hoc (recurrent, still script-shaped)

Q: recurrent command the client already has (SSH backup, `deploy.sh`,
`python relatorio.py`, `ffmpeg`, `kubectl`)?
Yes → Dagu YAML in the dags dir (source in the client's private repo):
`dagu validate x.yaml` → `dagu start x.yaml` (test, once) →
`dagu start-all --dags <dags-dir>` (scheduler + UI :8080).
`--dags` belongs to `start-all`/`server`/`scheduler`, not to `dagu start`.
Secrets in host env, never YAML. No repo changes here.
Dagu is GPLv3 (v2.x, verified v2.22.2): separate binary only, never copy
code into this repo; SSO/RBAC/audit are paid.

Do not run Dagu for a client that already has dagnats recipes, and do not
move an LLM/approval job into Dagu "because it is easier there" — that is
two orchestrators for one client. Pick one (full rule:
`docs/script-execution.md` → "Competition signal").

## Level 3 — Windmill migration (product surface)

Before choosing, the one-line rule: **one orchestrator per client**. Windmill
wins when the client needs UI/forms/apps, replay from a step, test-one-step, or
code steps in Go and accepts Postgres + AGPL/EE governance. n8n wins
for 400+ SaaS nodes and a visual builder, at the cost of weight and the SUL
license. Dagu wins for script-shaped recurrent jobs and step retry. dagnats
stays for typed contracts, native approvals, no arbitrary code, git-first and
a single binary.

OAuth for many SaaS APIs is a **layer**, not an orchestrator: Nango
(self-hosted, ELv2) exposes an HTTP proxy that dagnats or Windmill can call.
Never put raw OAuth tokens in Windmill variables or dagnats recipes.
Scaling strategy: default to shared Nango on consultancy host for SMBs (`runtime: shared`);
dedicated Nango instance only for isolated clients (`runtime: dedicated`).
Guide: `docs/nango.md`; agent patterns: `references/nango.md`; checklist: `references/auth-layers.md`.
Windmill free has no native critical alerts; alert from a flow step (HTTP or
SMTP) plus an external dead-man switch. Rules and test gate: `references/windmill.md`.


Q: needs generated UI/forms, integration catalog, team-facing apps?
Yes → per-client migration (`docs/migrate-to-windmill.md` +
`docs/isolating-a-client.md`). Levels 2 and 3 are alternatives —
never operate both for the same client.
