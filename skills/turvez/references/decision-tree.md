# Decision tree: where does this automation belong?

Q0 first: which repo am I in? `go.mod` only = template (public, no real
clients — refuse, point to turvez-ops via docs/ops-repo.md). `TURVEZ_PIN`
only = ops repo (private, any client OK). Then ask in order. First "yes"
wins. Full rationale: `docs/script-execution.md`.

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
Yes → Dagu YAML next to the script, `dagu start` to test, `dagu start-all`
for schedule + UI. Secrets in host env, never YAML. No repo changes here.
Dagu is GPLv3: separate binary only, never copy code into this repo.

## Level 3 — Windmill migration (product surface)

Q: needs generated UI/forms, integration catalog, team-facing apps?
Yes → per-client migration (`docs/migrate-to-windmill.md` +
`docs/isolating-a-client.md`). Levels 2 and 3 are alternatives —
never operate both for the same client.
