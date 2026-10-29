#!/usr/bin/env bash
# Scaffold a new client: fieldbook + env template + secrets stub.
#
# Usage (inside template OR ops repo):
#   ./scripts/new-client.sh <client-id>
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/new-client.sh <client-id>
#
# Template repo guard: only _templates/example-client/ops/personal may exist
# here — anything else aborts with a pointer to docs/ops-repo.md.
set -euo pipefail

[ -n "${1:-}" ] || { echo "Usage: new-client.sh <client-id> (lowercase, digits, dashes)" >&2; exit 1; }
CLIENT="$1"
[[ "$CLIENT" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad client id: $CLIENT (lowercase, digits, dashes)" >&2; exit 1; }

# Resolve target root: explicit ops dir wins; else current checkout.
if [ -n "${TURVEZ_OPS_DIR:-}" ]; then
  ROOT="$TURVEZ_OPS_DIR"
elif [ -f "TURVEZ_PIN" ]; then
  ROOT="."
else
  ROOT="."
fi

TEMPLATE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SEP="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["client_workflow_separator"])' "$TEMPLATE_DIR/naming.json")"
ALLOW="^(_templates|example-client|ops|personal)$"

if [ ! -f "$ROOT/TURVEZ_PIN" ] && [ -f "$ROOT/go.mod" ]; then
  # Template checkout: refuse real client names here.
  if ! [[ "$CLIENT" =~ $ALLOW ]]; then
    echo "refusing to create real client '$CLIENT' in the public template repo" >&2
    echo "create it in turvez-ops instead: ./scripts/new-ops-repo.sh ../turvez-ops, then TURVEZ_OPS_DIR=../turvez-ops ./scripts/new-client.sh $CLIENT" >&2
    echo "details: docs/ops-repo.md" >&2
    exit 1
  fi
fi

WF="$ROOT/workflows/$CLIENT"
[ -e "$WF" ] && { echo "already exists: $WF" >&2; exit 1; }
mkdir -p "$WF" "$ROOT/env" "$ROOT/secrets/$CLIENT"

# Eval scaffold: cases are generated from run history (see evals/_template).
EV="$ROOT/evals/$CLIENT"
mkdir -p "$EV"
if [ -d "$TEMPLATE_DIR/evals/_template" ]; then
  cp -R "$TEMPLATE_DIR/evals/_template/." "$EV/"
  sed "s/TODO-CLIENT/$CLIENT/g" "$EV/promptfooconfig.yaml" > "$EV/promptfooconfig.yaml.tmp" \
    && mv "$EV/promptfooconfig.yaml.tmp" "$EV/promptfooconfig.yaml"
fi

# Fieldbook from template (placeholder swap; portable upper-case for macOS bash 3.2).
UPPER="$(echo "$CLIENT" | tr 'a-z' 'A-Z')"
sed "s/TODO-CLIENT/$CLIENT/g; s/TODO_HUBSPOT_TOKEN/${UPPER}_HUBSPOT_TOKEN/g; s/TODO_SLACK_WEBHOOK_URL/${UPPER}_SLACK_WEBHOOK_URL/g" \
  "$TEMPLATE_DIR/workflows/_templates/FIELDBOOK.md" > "$WF/README.md"

# Starter recipe: copy cron-job skeleton with client prefix (keep basename so
# recipe name matches file: acme/cron-job in cron-job.json).
if [ -f "$TEMPLATE_DIR/workflows/_templates/cron-job.json" ]; then
  sed "s|TODO-CLIENT/|$CLIENT$SEP|g; s/TODO-CLIENT/$CLIENT/g" \
    "$TEMPLATE_DIR/workflows/_templates/cron-job.json" > "$WF/cron-job.json"
fi

# Env template (names only).
UPPER_ENV="$(echo "$CLIENT" | tr 'a-z' 'A-Z')"
cat > "$ROOT/env/$CLIENT.env.example" <<EOF
# Secret NAMES for $CLIENT. Copy to env/$CLIENT.env (gitignored) and fill values,
# or encrypt: age -r age1YOURPUBLICKEY -o secrets/$CLIENT/env.age env/$CLIENT.env
${UPPER_ENV}_HUBSPOT_TOKEN=
${UPPER_ENV}_SLACK_WEBHOOK_URL=
EOF

cat > "$ROOT/secrets/$CLIENT/.recipients" <<'EOF'
# One age1… public key per line (committed — public keys are public).
# Ask each operator for their key over an authenticated channel first.
EOF

echo "client scaffolded: $WF"
echo "next:"
echo "  \$EDITOR $WF/README.md   # fill §1-3 (context, contacts, systems)"
echo "  dagnats workflow validate $WF/*.json"
echo "  cd $TEMPLATE_DIR && WORKFLOWS_DIR=$ROOT/workflows go test ./workflows/"
echo "  evals: $EV  (generate cases: scripts/select-cases.py --dagnats --out $EV/cases; label: scripts/label-cases.py --dir $EV)"
