# Client intake: short interview + tool picks + the process mode

Use when the fieldbook is thin (new client) or a recipe stalls on a tool
choice (which form? which notifier? middleware or consultancy?). Degustia
diagnoses and prescribes; this repo executes. Boundary detail:
`references/methodology.md`.

## 0. Full discovery? Hand the client the degustia interview

The 2-question intake below **unblocks a build**; it does not discover a
process. When the client has no fieldbook at all and nobody has mapped how
their work actually runs, do not do the discovery yourself in this repo — the
degustia interview already asks exactly what a fieldbook needs (typical day,
the task that hurts most, the last real occurrence step by step, who hands off
to whom, who approves, frequency, time per run, error rate, what they already
pay for).

**Which mode to ask for defines which artifact comes back:**

| Who asked | Degustia mode | What comes back |
|---|---|---|
| The client, buying a diagnosis | `diagnostico` | HTML curation (which tool to buy) + transcript + `processo.json` |
| A consultancy/agency that will implement | `processo` | `transcript.md` + `processo.json` (no LLM, no HTML) |

The `processo` mode is the one that feeds this repo. It is deterministic: the
same answers always produce the same JSON, and it costs no tokens.

**Language.** The interview asks the language in step 0 (`pt-BR` or `en-US`)
and everything after follows that choice. If you already know the client's
language (they wrote to you in English), tell them to open the interview with
`?idioma=en-US` (or pass `idioma` to `/api/start`) so the question is skipped.
`processo.json` records the choice in `cliente.idioma`, and every label in it
is already in that language — so the fieldbook can be written from it directly,
in the client's language, without translating rows.

**Do not ask the client to paste a report.** Ask for the `processo.json` (plus
the transcript if the fieldbook needs the person's own words). Mapping into
`FIELDBOOK.md`:

| `processo.json` | fieldbook |
|---|---|
| `cliente.*` | §1 Context |
| `quantificacao.quem_faz`, `quantificacao.aprovacao` | §2 Contacts and decisions |
| `sistemas.atuais`, `sistemas.pagos` | §3 Systems and secret map (NAMES only) |
| `dor_principal.*`, `quantificacao.*`, `tentativas.*` | §4 candidate recipes |
| `dor_principal.greenfield`, `tentativas.travas` | §6 Decisions (build vs buy vs wait) |

Two fields decide feasibility before anything else:

- **`quantificacao.aprovacao`** — `"nao"` means the automation can write
  freely; anything else means the recipe needs an approval step
  (`approval` step + `aux_steps`), and that is a bigger recipe.
- **`perguntas_abertas`** — non-empty means a decision rests on a hypothesis.
  Treat each entry as an intake question (see §1), not as a detail to assume
  silently.

Where to run it: the plugin/`app` is a separate repo
(`degustia`, checkout beside this one). Locally the web-agent is internal
(`DEGUSTIA_GENERATOR=stub` costs nothing) — see that repo's `app/README.md` for
the two endpoints. Never put a client's answers through a paid model on our
account just to get the `processo.json`: that mode never calls an LLM.

## 1. Thin fieldbook? Interview (max 2 questions)

Degustia Passo 3 style: only internal business facts, never "which tool do
you prefer" (the skill decides that via hierarchy) and never anything
verifiable on the web (that is websearch).

Why 2 is enough: they don't do discovery (that is §0 above, or FDEOps
`discover`, a long process) — they **unblock the build**. The 2 that almost
always block a recipe:

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
export DEGUSTIA_SKILL_DIR=~/Development/degustia/.agents/skills/degustia-diagnostico
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

**`--regiao` matters for a client outside Brazil.** The catalog tags tools
`BR` / `Global`; without the flag a BR-only tool can win for a client who
invoices abroad. Pass `--regiao BR` (or `US`/`Global`) whenever you know it —
`processo.json` already carries it in `cliente.region`.

## 3. What NOT to do here

- Don't copy the degustia catalog into this repo: it has its own CI
  (24 recommendation rules + golden tests). Point, don't copy.
- Don't hand-edit the `processo.json` to make it fit a recipe. If a field is
  wrong, the interview asked the wrong question — that is a proposal in the
  degustia repo (its `scripts/entrevista.py` is the single source, and the
  prose the MCP host reads is parity-checked against it), not a patch here.
- Don't run the degustia interview **instead of** the 2-question intake when
  the fieldbook only lacks the webhook shape. §0 is for "no process mapped
  anywhere", not for "one field is missing".
