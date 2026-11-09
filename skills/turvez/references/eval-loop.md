# Eval loop (agent procedure)

The exact sequence an agent follows to turn run history into a measured eval.
Confirm at the marked points; never run the eval unasked.

Which strategy to use when — deterministic, reference, production sampling,
logs-only, curated, synthetic, LLM-as-judge, human — is
`docs/eval-strategies.md`.

## 0. Prerequisites

- the client's stack is running and the CLI points at it:
  `export DAGNATS_NATS_URL=nats://127.0.0.1:4222`
- the client folder exists: `clients/<client>/` and `clients/<client>/evals/`
  (both scaffolded by `scripts/new-client.sh`)

## One command

```bash
python3 scripts/eval-loop.py --dir clients/<client>/evals
```

It runs steps 2 to 6 below, asks before spending judge tokens, and prints pass
rate plus per-field accuracy. Flags: `--skip-label`, `--skip-eval`, `--yes`,
`--strict`, `--include-ok N`. Without a terminal it skips labeling.

## 1. Declare the taxonomy (once per client)

```bash
python3 scripts/label-cases.py --dir clients/<client>/evals --setup
# fields (comma-separated) [category]: category,urgency
# values for category [billing,technical,other]: billing,technical,other
# values for urgency []: low,high
```

Writes `clients/<client>/evals/eval.config.json`. It also holds the step names
(`prompt_step`, `result_step`, `gate_step`) and the optional `rubric`.

## 2. Generate cases from run history

```bash
python3 scripts/select-cases.py --dagnats --out clients/<client>/evals/cases --include-ok 3
```

Reads every run, picks failures, retries, gate rejections and malformed
output, dedupes by case hash, and writes `cases.json` plus `selection.json`.
Show the operator the counts and reasons. **Ask before continuing.**

## 3. Label

```bash
python3 scripts/label-cases.py --dir clients/<client>/evals
```

Keys: `enter` accepts the production value, a unique prefix (`b`, `hi`) picks a
value, `1..N` picks by position, any other text becomes a new value and is
appended to `eval.config.json`, `s` skips, `q` quits. Labels key on the input
hash, so the same ticket in two runs is labeled once.

Use `--blind` for a sample, to avoid rubber-stamping production.

## 3b. Logs only (no production output)

When the client has only logs, there is no output to suggest a label. Import
the inputs and label by hand:

```bash
python3 scripts/import-logs.py --input logs.jsonl --text-field message --out clients/<client>/evals
# fill expected_category in cases/logs.csv
npx promptfoo eval -c promptfooconfig.logs.yaml
```

The import dedupes by text and never invents a label. Use `--split holdout`
for the rows you will not tune on. This path bypasses `select-cases.py`, which
is tied to dagnats runs.

## 4. Add curated cases (optional)

Hand-written cases live in `clients/<client>/evals/cases/curated.yaml` (rich asserts)
or `cases/curated.csv` (spreadsheet). Keep `metadata.source: curated` and
`metadata.split: dev|holdout`.

## 5. Run

```bash
make eval-client CLIENT=<client> TURVEZ_OPS_DIR=...
make eval-client-view CLIENT=<client> TURVEZ_OPS_DIR=...   # :15500
```

Ask first: the judge costs tokens. With only `is-json` + label assertions the
run is deterministic and cheap.

## 6. Read

- `results.json` → per case: `success`, `response.output`, each assertion's
  `pass` and reason, tokens, latency.
- `View YAML` shows the frozen case; `metadata.run_id` links back to the run.
- accuracy by field:

```bash
jq -r '.results.results[]
  | select(.testCase.vars.expected_category)
  | [.testCase.vars.expected_category,
     (.response.output | fromjson | .category)] | @tsv' results.json \
| sort | uniq -c
```

## 7. Confirm before side effects

- `workflow register` / `run start`: ask.
- eval run: ask (tokens).
- label edits that change the taxonomy: ask (they change future schemas).

## Troubleshooting

### promptfoo (each was hit, not read)

- **Provider silently calls api.openai.com**: the key is `apiBaseUrl`, not
  `apiHost` (the name other OpenAI-compatible tools use). Wrong name = no error,
  just a cloud call. Symptom: eval passes with no traffic in the gateway log.
- **`llm-rubric` fails or demands `OPENAI_API_KEY`**: the grader inherits the
  default provider, not yours. Pin it under `defaultTest.options.provider`.
- **`Unexpected token 'return'`**: a JS assertion written as an expression body.
  Use `is-json` + JSON schema instead of code; code assertions are the brittle
  path here.
- **Rubric grades blind** ("No ticket content was provided"): the rubric value
  does not interpolate the case. Put `{{text}}` inside the rubric string.
- **Eval traffic re-ingested as production samples**: tag it (`x-eval-run`, kept
  via the gateway's logging-headers allowlist) or the next sampling round
  feeds on itself.

A green eval that never reached the gateway, or a rubric grading an empty
prompt, is the failure mode these five produce. Check the gateway log before
trusting a 100% pass rate.

### dagnats

- **`empty completion (finish=length)`**: reasoning model spent `max_tokens`;
  raise it in the recipe, not in the eval.
- **rubric disagrees with a correct label**: judges vary. Prefer schema +
  label; enable `rubric` only where no label exists.
- **labels disappeared**: they key on the input hash; if the prompt changed,
  the input changed. Re-label the new input.
- **`cannot connect to NATS`**: the CLI is NATS-first; check
  `DAGNATS_NATS_URL`, not `DAGNATS_API_URL`.
- **A step's prompt is missing from the run**: prompts are not recorded unless
  a `build-prompt` step emits `messages` (see `docs/observability.md`).

### Windmill eval source

`select-cases.py` reads dagnats runs. For a Windmill client, normalize first:

```bash
python3 scripts/windmill-job-to-run.py --job-id <id> --out run.json
python3 scripts/select-cases.py --runs run.json --out clients/<client>/evals/cases
```
