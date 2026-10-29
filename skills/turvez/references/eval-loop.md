# Eval loop (agent procedure)

The exact sequence an agent follows to turn run history into a measured eval.
Confirm at the marked points; never run the eval unasked.

## 0. Prerequisites

- the client's stack is running and the CLI points at it:
  `export DAGNATS_NATS_URL=nats://127.0.0.1:4222`
- the client folder exists: `workflows/<client>/` and `evals/<client>/`
  (both scaffolded by `scripts/new-client.sh`)

## One command

```bash
python3 scripts/eval-loop.py --dir evals/<client>
```

It runs steps 2 to 6 below, asks before spending judge tokens, and prints pass
rate plus per-field accuracy. Flags: `--skip-label`, `--skip-eval`, `--yes`,
`--strict`, `--include-ok N`. Without a terminal it skips labeling.

## 1. Declare the taxonomy (once per client)

```bash
python3 scripts/label-cases.py --dir evals/<client> --setup
# fields (comma-separated) [category]: category,urgency
# values for category [billing,technical,other]: billing,technical,other
# values for urgency []: low,high
```

Writes `evals/<client>/eval.config.json`. It also holds the step names
(`prompt_step`, `result_step`, `gate_step`) and the optional `rubric`.

## 2. Generate cases from run history

```bash
python3 scripts/select-cases.py --dagnats --out evals/<client>/cases --include-ok 3
```

Reads every run, picks failures, retries, gate rejections and malformed
output, dedupes by case hash, and writes `cases.json` plus `selection.json`.
Show the operator the counts and reasons. **Ask before continuing.**

## 3. Label

```bash
python3 scripts/label-cases.py --dir evals/<client>
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
python3 scripts/import-logs.py --input logs.jsonl --text-field message --out evals/<client>
# fill expected_category in cases/logs.csv
npx promptfoo eval -c promptfooconfig.logs.yaml
```

The import dedupes by text and never invents a label. Use `--split holdout`
for the rows you will not tune on. This path bypasses `select-cases.py`, which
is tied to dagnats runs.

## 4. Add curated cases (optional)

Hand-written cases live in `evals/<client>/cases/curated.yaml` (rich asserts)
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

- **`empty completion (finish=length)`**: reasoning model spent `max_tokens`;
  raise it in the recipe, not in the eval.
- **rubric disagrees with a correct label**: judges vary. Prefer schema +
  label; enable `rubric` only where no label exists.
- **labels disappeared**: they key on the input hash; if the prompt changed,
  the input changed. Re-label the new input.
- **`cannot connect to NATS`**: the CLI is NATS-first; check
  `DAGNATS_NATS_URL`, not `DAGNATS_API_URL`.
