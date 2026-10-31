# Decision tree: where does this automation belong?

Q0 first: which repo am I in? `go.mod` only = template (public, no real
clients — refuse, point to turvez-ops via docs/ops-repo.md). `TURVEZ_PIN`
only = ops repo (private, any client OK). Then ask in order. First "yes"
wins. Full rationale: `docs/script-execution.md`.

## Q1.5 — agent loop gate

Does it need an agent that loops with tools (shell, files, browser) until
done? → **not** a recipe, **not** Dagu level 2. It is a new platform
decision (Dagu 2.x `llm.*`/`harness.run`/agent DAGs, or a dedicated
harness). Why: `docs/writing-recipes.md` → "Why AgentLoop/planner stay
upstream-only".

## Level 0 — dagnats recipe (default)

Q: fits the 12 handlers (`http.request`, `transform.json`, `condition`,
`map`, `delay`, `notify.*`, `request.approval`, `llm.complete`, `dlq.check`)?
Yes → write `workflows/<client>/*.json`, `validate` + `go test`, propose `register`.
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
code steps in TS/Python/Go and accepts Postgres + AGPL/EE governance. n8n wins
for 400+ SaaS nodes and a visual builder, at the cost of weight and the SUL
license. Dagu wins for script-shaped recurrent jobs and step retry. dagnats
stays for typed contracts, native approvals, no arbitrary code, git-first and
a single binary.

OAuth for many SaaS APIs is a **layer**, not an orchestrator: Nango
(self-hosted, ELv2) exposes an HTTP proxy that dagnats or Windmill can call.
Confirm the install with the operator; it adds Postgres and a token store.
Windmill free has no native critical alerts; alert from a flow step (HTTP or
SMTP) plus an external dead-man switch.


Q: needs generated UI/forms, integration catalog, team-facing apps?
Yes → per-client migration (`docs/migrate-to-windmill.md` +
`docs/isolating-a-client.md`). Levels 2 and 3 are alternatives —
never operate both for the same client.
