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

## How to ask — the interview protocol (applies to every Q below)

The tree is deterministic: **read first, ask second, recommend always, record
the answer.** The questions below are the agent's own checklist; most of them
are answered from the repo, not from the operator.

1. **Read before asking.** `client.yaml` (engine, runtime, target), the
   fieldbook §1-3, the existing `automation/`, `env/*.example`, `engines.json`,
   `naming.json`, `.github/dependabot.yml`, `go.mod`. A fact that is written
   down is never a question — asking for it teaches the operator to distrust
   the tree.
2. **Ask only what flips a branch.** A question is allowed only when two
   answers lead to different paths (engine, level, capability, host, library).
   At most **3 questions per decision**, one at a time, never a form.
3. **Every question carries the recommendation.** Format: the question, then
   `recommended: <option> — <one-line why>`, then the closed options. The
   operator may answer `default`; that settles it. "What do you think?" is not
   a question the tree asks.
4. **Closed answers only:** yes/no, one of the listed options, or a name
   (client id, engine, Nango provider slug). If the answer needs an essay, the
   question was wrong — pick the default and proceed.
5. **Stop rule.** After 3 questions without a clear branch, take the
   recommendation, say the assumption in one line, proceed, and record it. Do
   not loop, and do not ask the same question in different words.
6. **Determinism.** The same facts must select the same branch, every run. A
   recorded decision is re-opened only when a fact changes (new engine, new
   host, new scale), and then the change is named out loud.
7. **Record where the next run reads it** — `engine:`, `runtime:`,
   `target:` in `client.yaml`; the why in the fieldbook (`clients/<client>/README.md`).
   An unwritten decision is re-asked forever; `runtime` was written and never
   read until the contract test started failing clients for it.

**Libraries** are part of the same interview (Q1.6): the choice is
stdlib → a dependency already in `go.mod` → an approved capability (goai,
Nango, Genkit) → a **new dependency, proposed and confirmed**. Never install
one silently. Version policy: `go.mod` pins, Dependabot keeps it current — no
`go get -u` inside a build.

**Defaults at a glance** — start here, and let an interview fact move exactly
one row. This is what makes two runs on the same repo agree.

| Decision | Default | The fact that moves it |
|---|---|---|
| Engine | `windmill` | typed contracts, native approvals, no arbitrary code, AGPL/Postgres objection → `dagnats`; a script-shaped recurrent job → level 2 |
| Deployment | profile B (managed, shared) | regulation/residency, client-owned infra, noisy neighbour, per-client upgrade window → dedicated host |
| One-off code | level 1 sandbox, zero residue | it recurs → level 2; it needs replay/UI → level 3 |
| Answer over data | system of record via Nango | documents that don't fit one prompt → route-then-read |
| Model call | `llm.complete` (goai) | a human watches a loop → Genkit; nobody watching → recipe |
| Library | stdlib → `go.mod` → approved capability | none of those fit → propose a new dependency, confirm, then pin |

## Q0.8 — is the ask "answer questions over our data"?

Ask in order; stop at the first branch that answers. Full playbook:
`references/retrieval.md`, rationale `docs/retrieval.md`.

1. **System of record** (CRM, ERP, ticketing, wiki with an API) → a tool call
   through Nango: the fixed HTTP proxy in a recipe/flow. An Agent Session/MCP
   only when an agent must choose tools (Q1.5). Do not index a stale copy.
2. **A handful of pages that fit one prompt** → long context with a stable
   prefix; no index, no pipeline.
3. **Documents** → **route-then-read**:
   - classify the query first: identifiers → BM25 lexical only; natural language
     → BM25 + small dense hybrid (RRF over units) with an optional cross-encoder
     rerank of units;
   - the **routing unit** is a chunk (~256-512 tokens, 50-100 overlap) that
     remembers its parent section/chapter; the **reading unit** is the parent,
     not the chunk and not the whole chapter;
   - expand with the glossary → OpenWN-PT → FastText only behind an eval;
   - dedupe by `doc_id:unit_id`;
     caps: 0 rewrites by default, or 1 cached rewrite with a fallback to the
     original query; hybrid candidate cap 25-50; reranker batch + timeout +
     BM25 fallback;
   - hard token budget with the target tokenizer + output reserve; a cutoff
     calibrated per model (never a copied 0.35) chooses read vs the explicit
     "not found" answer; no cloud call when nothing passes;
   - eval 30-50 questions + 5-10 refusals; add vectors inside Bleve only when
     lexical + synonyms measurably miss.
4. **Synthesis across more than fits one context** → hierarchy/map-reduce, or
   vectors; not a bigger prompt.
5. **Open-domain, paraphrase-heavy, cross-lingual** → semantic retrieval is the
   right tool; still gate it with the eval.

"Bounded, not immune": measure worst-case tokens, reranker p95 and false
refusals. A vector DB is never the first answer, and "top 5" is not a limit.
A chapter is not the only index unit for long documents; top-5 snippets is not
the reading unit when the parent section fits the budget.

Then, **for a dagnats client only**, ask the levels below in order. First
"yes" wins. Full rationale: `docs/script-execution.md`. A Windmill client stops
at `references/windmill.md` — the level questions are dagnats-shaped.

## Q1.5 — agent loop gate

Does it need an agent that loops with tools (shell, files, browser) until
done? → **not** a recipe, **not** Dagu level 2. Split it by who is waiting:

- **A human is watching this request finish** (review/rewrite, triage with a
  short read-only toolset, extraction with a critic, a bounded RAG answer):
  that is an **agent-loop capability** — a Genkit flow in a Go service, with
  goai as the only model path and Nango for tools (`docs/genkit.md`). Bounded
  (`MaxIterations`, `MaxRunDuration`), escalates to a human instead of
  deciding alone. Build it with the scaffold, not from a blank file:
  `./scripts/new-agent-service.sh <client>` (plan) then `--apply --provider <slug>`,
  then the pinned `go get` the scaffold prints, in review — the commands are
  in `docs/genkit.md#install-and-scaffold-when-q15-says-yes`.
- **Nobody is watching, or it must survive a restart**: a new platform
  decision (Dagu 2.x `llm.*`/`harness.run`/agent DAGs, or a dedicated
  harness). Why: `docs/writing-recipes.md` → "Why AgentLoop/planner stay
  upstream-only".

Either way the durable consequence is a **run in the client's engine**
(dagnats `run start` / Windmill flow, idempotency key from the request) —
Genkit is a capability, never a third engine, and the engine count stays one.

**Not our agent?** If the agent belongs to the client and calls the tools we
authenticated, the question stops being "which engine runs the loop" and becomes
"what may it call": that is the **access plane** — `access:` in `client.yaml`,
a ceiling in `catalog/policy.yaml`, one broker per client by default
(`docs/access-plane.md`). It is a layer, like Nango, never a second engine.

## Q1.6 — is the ask "a library / new dependency"?

Ask in order; the first `yes` wins, and it is the whole answer.

1. **Stdlib does it** → stdlib. No dependency for `net/http`, `encoding/json`,
   `log`.
2. **Already in `go.mod`** → use it. No review, no new pin.
3. **An approved capability** → goai (the only model path), Nango (credentials
   and SaaS tools), Genkit (a human-watched loop, `docs/genkit.md`, scaffold
   `scripts/new-agent-service.sh`). Use the capability's own seam; never
   re-implement it beside itself.
4. **Something new** → propose, don't install: name, version, why the three
   above cannot do it, and what removing it costs. The operator confirms; then
   `go.mod` pins it and Dependabot keeps it current. Never `go get -u` in a
   build.

Refuse a dependency that adds a **second** path to something the repo already
owns: a provider SDK beside goai, a retry/queue library beside the engine, an
HTTP client beside `http.request`, an approval system beside the engine's
native one. A library that saves five lines is a no.

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
(per-client broker, ELv2; documented self-host is Enterprise) exposes an HTTP proxy that dagnats or Windmill can call.
Never put raw OAuth tokens in Windmill variables or dagnats recipes.
Where the broker runs is `access.plane`, not `runtime` (docs/access-plane.md):
client-hosted by default and mandatory for `data_class: sensitive`; a shared
broker on the consultancy host is an explicit pilot (`--shared`, `normal` only).
Guide: `docs/nango.md`; agent patterns: `references/nango.md`; checklist: `references/auth-layers.md`.
Windmill free has no native critical alerts; alert from a flow step (HTTP or
SMTP) plus an external dead-man switch. Rules and test gate: `references/windmill.md`.


Q: needs generated UI/forms, integration catalog, team-facing apps?
Yes → per-client migration (`docs/migrate-to-windmill.md` +
`docs/isolating-a-client.md`). Levels 2 and 3 are alternatives —
never operate both for the same client.

## Testing the tree

The tree ships operator scenarios and a deterministic transcript grader:
`skills/turvez/evals/README.md`. CI runs
`python3 scripts/decision-eval.py --check --self-test` (fixtures against
`engines.json` and real artifacts, plus the grader's own red proof). A model
run is local: produce a transcript with any agent, then `--grade` it. The
grader checks the branch, the question budget, the recommendation on every
question, the facts that must not be re-asked, and the recorded decision.
