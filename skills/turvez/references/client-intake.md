# Client intake: short interview + tool picks via degustia

Use when the fieldbook is thin (new client) or a recipe stalls on a tool
choice (which form? which notifier? middleware or consultancy?). Degustia
diagnoses and prescribes; this repo executes. Boundary detail:
`references/methodology.md`.

## 1. Thin fieldbook? Interview (max 2 questions)

Degustia Passo 3 style: only internal business facts, never "which tool do
you prefer" (the skill decides that via hierarchy) and never anything
verifiable on the web (that is websearch).

Why 2 is enough: they don't do discovery (that is FDEOps `discover`, a long
process) — they **unblock the build**. The 2 that almost always block a recipe:

1. Where does the trigger come from and in what shape? (which system webhook,
   real payload example — without it the first `transform.json` is a guess)
2. Who holds the credentials and who approves the side effect? (secret owner,
   human approver — nothing with side effects registers without this)

1 question per round; no answer in 2 rounds, assume with an explicit
assumption and record it in the fieldbook. Worked example: "does the HubSpot
webhook payload carry `dealstage`? send an example" → recipe ships in one go;
without it, 3 rounds of guessing.

## 2. Open tool choice? Query the local degustia catalog

The ranker is deterministic, dependency-free, network-free — it runs against
the degustia checkout (another repo; never copied here):

```bash
export DEGUSTIA_SKILL_DIR=~/Development/degustia/.agents/skills/cali-degustia-diagnostico
# adjust the path to your machine
python3 "$DEGUSTIA_SKILL_DIR/scripts/rank_candidates.py" \
  --dor "notify the team on slack when a crm deal closes" \
  --tag Automação --perfil Intermediario --k 4
```

Preference order (degustia 03-priorizacao): what the client already has >
free catalog covering requirements + volume > paid with documented gap.
If the answer is "middleware" and the client will hire implementation
(Projeto), the path is consultancy via this repo (level 0) instead of DIY —
see `docs/script-execution.md` Q1. Record the decision in fieldbook §6
(tool, why it won, date).

## 3. What NOT to do here

- Don't call degustia's public MCP from this flow: it has an IP allowlist
  (OpenAI egress; 403 outside it) and is stateless by design. The local
  ranker gives the same result with no network or auth.
- Don't copy the degustia catalog into this repo: it has its own CI
  (24 recommendation rules + golden tests). Point, don't copy.
