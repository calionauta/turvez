# Client eval

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

Do not commit raw customer text: the generator reads what the recipe persisted.
If the recipe redacts before persisting (see `docs/observability.md`), the cases
are already safe.
