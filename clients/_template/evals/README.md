# Client eval

Files, and who owns them:

| Path | Format | Owner | Hand-edit? |
|---|---|---|---|
| `promptfooconfig.yaml` | YAML | you | yes |
| `prompts/classify.json` | JSON chat messages | you | yes |
| `cases/cases.json` | JSON | `select-cases.py` | no, regenerated |
| `cases/curated.yaml` | YAML | you | yes |
| `labels.json` | JSON map (hash → label) | `label-cases.py` | via the script |

YAML for humans, JSON for machine output. promptfoo accepts either, plus CSV,
XLSX, Google Sheets, SharePoint and Azure Blob. The config loads both case
files:

```yaml
tests:
  - file://cases/cases.json
  - file://cases/curated.yaml
```

Prefer a spreadsheet? `cases/curated.example.csv` is the same dataset in CSV.
Copy it to `cases/curated.csv` and swap the second line for
`file://cases/curated.csv`. Columns are vars, and `__expected` holds the
assertion (`javascript: JSON.parse(output).category === '{{expected_category}}'`),
`__metadata:split` holds the dev/holdout tag. promptfoo also reads XLSX and
Google Sheets.


Cases are **generated from run history**, not written by hand, so an eval always
reflects what actually ran.

```bash
# 1. Point the CLI at the client's stack (NATS-first)
export DAGNATS_NATS_URL=nats://127.0.0.1:4222

# 2. Generate cases from the client's runs into this directory
python3 "$TURVEZ_CHECKOUT/scripts/select-cases.py" --dagnats --out cases --include-ok 3

# 3. Edit promptfooconfig.yaml: model + gateway URL (and keep max_tokens in
#    sync with the recipe — reasoning models spend it before the answer).

# 4. Run / view
make eval-client CLIENT=<client>
make eval-client-view CLIENT=<client>
```

`select-cases.py` selects failed runs, retries, gate rejections and malformed
output, deduplicates by content hash, and writes an audit trail
(`selection.json`) next to `cases.json`. `--include-ok N` adds N clean runs as
controls.

One command for the whole loop:

```bash
python3 scripts/eval-loop.py --dir .
```

## Judge (only for rubric)

Deterministic assertions (`is-json`, label) cost nothing. `llm-rubric` needs a
grader model; `eval-loop.py` passes it with `--grader`:

```json
"judge": { "provider": "openrouter:typesafe/jev-router" }
```

Any promptfoo provider works (OpenRouter, OpenAI-compatible gateway, local).
Set `rubric` only when no deterministic check is possible.

## Logs only

No production output means no suggestion. Import inputs, then label by hand:

```bash
python3 scripts/import-logs.py --input logs.jsonl --text-field message --out .
# fill expected_category in cases/logs.csv
npx promptfoo eval -c promptfooconfig.logs.yaml
```

## Labeling cases

Each case carries `metadata.hash`, a stable id derived from system prompt,
input and production output. Labels live in `labels.json` next to `cases/`
(the selector reads `<out>/../labels.json` by default, or pass `--labels`):

```json
{
  "abd65cebaf140c13": { "expected_category": "billing", "note": "invoice complaint" }
}
```

With a label, the selector sets `vars.expected_category` and appends
`JSON.parse(output).category === "<label>"`. Regenerating cases keeps labels,
because the hash does not include the label.

Before the first run, declare what the operator is labeling. This is the only
per-client configuration; the scripts are generic:

```bash
python3 scripts/label-cases.py --dir clients/acme/evals --setup
# fields (comma-separated) [category]: category,urgency
# values for category [billing,technical,other]: billing,technical,other
# values for urgency []: low,high
```

It writes `eval.config.json`. Shortcuts are generated from the first letter of
each value; override with a `keys` map. `select-cases.py` reads the same file
for step names, schema and rubric.

Fast path: label by exception. The script shows the text and what production
answered; enter accepts a shortcut or a number, any other text becomes a free
value, s skips.

```bash
python3 "$TURVEZ_CHECKOUT/scripts/label-cases.py" --dir .
```

Get the hashes with:

```bash
jq -r '.[] | "\(.metadata.hash) \(.metadata.run_id)"' cases/cases.json
```

Label a sample (10-30), not everything. The rubric covers the long tail.

Do not commit raw customer text: the generator reads what the recipe persisted.
If the recipe redacts before persisting (see `docs/observability.md`), the cases
are already safe.
