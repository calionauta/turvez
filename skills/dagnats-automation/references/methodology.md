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
| Per-client ops memory (contacts, secrets map, runbook)? | `workflows/<client>/README.md` fieldbook (in git) |
| Cross-session engagement memory? | FDEOps `~/fde-engagements/` record (local, out of git) |

Handoff: FDEOps `runbook` output fills `FIELDBOOK.md` §1–§5;
`scope`/`options` decisions land in fieldbook §6.

## Degustia (diagnóstico) vs este repo (execução)

Degustia (repo privado local, skill `cali-degustia-diagnostico`) prescreve
ferramentas a partir de entrevista (transcript → JSON Pydantic → HTML).
Para no relatório: não desenha arquitetura nem opera nada. Ponte sem
mesclar repos:

- Diagnóstico aponta automação (encadeamento 2.2b) + cliente contrata
  implementação (upsell Projeto) → seed o fieldbook (ver FIELDBOOK.md:
  dores → §1, sistemas → §3, automações → §4) e construa a recipe aqui.
- Nunca edite o catálogo degustia a partir daqui: tem CI própria
  (regras de recomendação + golden tests). Mudança lá (ex.: citar
  "consultoria implementa via dagnats" no roteamento de middleware)
  é proposta no repo deles, não commit deste.
