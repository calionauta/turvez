# Methodology boundary: FDEOps vs this repo

[FDEOps](https://github.com/suboss87/FDEOps) (MIT, 35 task skills + `fde`
coordinator, `npx skills add suboss87/fdeops --skill <name>`) is the
**strategy/discovery layer**: `discover`, `who-decides`, `scope`,
`options`, `plan`, `runbook`, `debrief`. Customer memory lives in
`~/fde-engagements/<customer>/.fde/`.

This repo is the **execution layer**: durable automations on dagnats.
Do NOT vendor FDEOps here (35 skills would swamp a lean runtime repo).

| Question | Answer with |
|---|---|
| What is worth building? Who decides? | FDEOps skills (harness-level, upstream) |
| Where does it run + build it now? | this skill (levels 0–3) |
| Per-client ops memory (contacts, secrets map, runbook)? | `clients/<client>/README.md` fieldbook (in git) |
| Cross-session engagement memory? | FDEOps `~/fde-engagements/` record (local, out of git) |

Handoff: FDEOps `runbook` output fills `FIELDBOOK.md` §1–§5;
`scope`/`options` decisions land in fieldbook §6.

## Degustia (diagnosis) vs this repo (execution)

Degustia (private sibling repo, `degustia-diagnostico` skill) prescribes
tools from interviews (transcript → Pydantic JSON → HTML). It stops at the
report: it designs no architecture and operates nothing. Bridge without
merging repos:

- Diagnosis flags automation (chaining 2.2b) + client hires implementation
  (Projeto upsell) → seed the fieldbook (see FIELDBOOK.md: pains → §1,
  systems → §3, automations → §4) and build the recipe here.
- **Same interview, second consumer:** degustia's `modo=processo` returns
  `processo.json` (steps, tools per step, who approves, frequency, time per
  run, error rate, what they already pay) with no LLM and no HTML. That is the
  seed for `FIELDBOOK.md`, and it is the mode a consultancy should ask a
  client for — see `references/client-intake.md` §0. `aprovacao != "nao"`
  means the recipe needs an approval step; `perguntas_abertas` non-empty means
  a decision rests on a hypothesis.
- Never edit the degustia catalog from here: it has its own CI
  (recommendation rules + golden tests). Changes there (e.g. citing
  "consultancy implements via dagnats" in middleware routing)
  are proposals in their repo, not commits in this one. The interview itself
  lives in that repo's `scripts/entrevista.py` (single source, parity-checked
  against the prose the MCP host reads) — questions are added there, not by
  editing a report.
