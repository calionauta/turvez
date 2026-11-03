# Decision eval — does the agent actually follow the tree?

`scripts/test-decisions.sh` proves the **contract**: every branch names an
artifact that exists and every engine is routed, scaffolded and gated. It cannot
prove **behavior**: that an agent reads `client.yaml` before asking, asks at most
three branch-flipping questions, puts a recommendation on each one, refuses the
documented traps, and records the answer. Those failures only appear in a
session, so they get a separate eval.

## Two layers

| Layer | Command | Runs where | Proves |
|---|---|---|---|
| Contract | `python3 scripts/decision-eval.py --check --self-test` | CI, no model | fixtures match `engines.json`/real artifacts; the grader rejects the five failure classes |
| Behavior | `--grade <transcript.json>` | local, any agent | what one run actually asked and decided |

The model run is **not a CI gate**. It is stochastic, costs tokens, and would
block merges on model drift instead of repo drift. CI checks the fixtures and
the grader; a model run is a local act before changing the tree.

## Produce a transcript

Any agent that can read the repo works. With Codex (read-only, structured
output, so the card is valid JSON):

```bash
codex exec -s read-only -C . \
  --output-schema skills/turvez/evals/transcript.schema.json \
  -o /tmp/decision.json \
  "You are the turvez agent. Read AGENTS.md, skills/turvez/SKILL.md and
   skills/turvez/references/decision-tree.md first. Handle this operator
   message:

   <paste the scenario ask>

   Facts already read from the repo:

   <paste facts.client_yaml, or 'no client.yaml yet'>

   Follow the interview protocol: ask only what flips a branch, at most the
   scenario budget, one question at a time; every question contains a
   recommendation and closed options. Then emit the transcript for scenario
   '<scenario id>'. Do not modify files."
python3 scripts/decision-eval.py --grade /tmp/decision.json
```

The transcript is the shape in `transcript.schema.json`: the questions the agent
would ask, and the decision it would record. Grading is mechanical:

- branch: `engine`, `runtime`, `level`, `capability`, optional `library`;
- interview: question count ≤ the scenario budget, every question has
  `recommended` and ≥2 closed `options`;
- read-before-ask: no question matches a `forbid` pattern (a fact the scenario
  already gave);
- record: `decision.record` points at `clients/<client>/client.yaml`.

## Scenarios

| id | What it catches |
|---|---|
| `windmill-ui-form` | over-asking: a new client with a form is Windmill + shared, no question needed |
| `dagnats-typed-contract` | not reading `client.yaml`, then re-litigating an engine the manifest already fixed |
| `approval-is-not-dagnats` | the documented trap: approval is not a dagnats criterion; Windmill does it natively |
| `human-watched-agent` | treating Genkit as an engine instead of a capability behind the existing run |
| `documents-retrieval` | vectors-first or whole-document reading instead of chunk→parent |
| `one-off-sandbox` | standing up Dagu/a handler for one execution |
| `level-2-dagu` | migrating a Dagu script job to a product engine |
| `new-library` | silently installing a second retry path instead of refusing it |

## What this does not do

It measures the **first turn** (read, ask, branch, record). It does not simulate
the operator with a second model: a canned scenario is more deterministic and
does not add a second source of noise. Multi-turn negotiation is out of scope;
if it becomes a real failure mode, extend the transcript with the answers and
grade the final card against the same fixtures.
