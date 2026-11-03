#!/usr/bin/env bash
# Scaffold a new client on the neutral spine:
#   clients/<client>/client.yaml            # engine, runtime, target
#   clients/<client>/README.md              # fieldbook
#   clients/<client>/automation/<engine>/   # windmill or dagnats
#   clients/<client>/evals/                 # promptfoo cases
#   env/<client>.env.example                # secret NAMES
#   secrets/<client>/                       # age recipients
#
# Usage (inside template OR ops repo):
#   ./scripts/new-client.sh <client-id>                     # default engine
#   ./scripts/new-client.sh <client-id> --engine=dagnats    # typed/simple work
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/new-client.sh <client-id>
#
# The default engine is whatever `engines.json` says (`default: windmill`), not
# a hardcoded choice here: changing the policy is one edit, and the same file is
# read by the contract test and the validate gate.
# dagnats stays fully supported and chosen with --engine=dagnats when the
# criteria in docs/client-spine.md#choosing-the-engine pick it.
#
# Scaffold the engine the criteria pick and record it in client.yaml so the
# manifest can never contradict the tree.
#
# Template sources are read from clients/_template when it exists, else the
# legacy phase-1 workflows/_templates; both layouts keep working.
#
# Template repo guard: only _templates/_template/example-client/ops/personal
# may exist here — anything else aborts with a pointer to docs/ops-repo.md.
set -euo pipefail

CLIENT=""
# The default engine comes from engines.json, not from this script: one place to
# change the policy (docs/client-spine.md#choosing-the-engine).
ENGINES_JSON="$(cd "$(dirname "$0")/.." && pwd)/engines.json"
ENGINE=""
if [ -f "$ENGINES_JSON" ]; then
  ENGINE="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("default",""))' "$ENGINES_JSON" 2>/dev/null || true)"
fi
ENGINE="${ENGINE:-windmill}"
for arg in "$@"; do
  case "$arg" in
    --engine=*) ENGINE="${arg#--engine=}" ;;
    -*) echo "unknown flag: $arg" >&2; exit 1 ;;
    *) CLIENT="$arg" ;;
  esac
done

# Validate the engine against engines.json (fallback = built-in set, so a
# checkout pinned before engines.json existed still works).
VALID_ENGINES="windmill dagnats dagu"
if [ -f "$ENGINES_JSON" ]; then
  VALID_ENGINES="$(python3 -c 'import json,sys; print(" ".join(sorted(json.load(open(sys.argv[1]))["engines"])))' "$ENGINES_JSON" 2>/dev/null || echo "$VALID_ENGINES")"
fi
case " $VALID_ENGINES " in
  *" $ENGINE "*) ;;
  *) echo "bad engine: $ENGINE (known: $VALID_ENGINES — add it to engines.json)" >&2; exit 1 ;;
esac

[ -n "$CLIENT" ] || { echo "Usage: new-client.sh <client-id> [--engine=<$(echo $VALID_ENGINES | tr ' ' '|')>]" >&2; exit 1; }
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
ALLOW="^(_templates|_template|example-client|ops|personal)$"

if [ ! -f "$ROOT/TURVEZ_PIN" ] && [ -f "$ROOT/go.mod" ]; then
  # Template checkout: refuse real client names here.
  if ! [[ "$CLIENT" =~ $ALLOW ]]; then
    echo "refusing to create real client '$CLIENT' in the public template repo" >&2
    echo "create it in turvez-ops instead: ./scripts/new-ops-repo.sh ../turvez-ops, then TURVEZ_OPS_DIR=../turvez-ops ./scripts/new-client.sh $CLIENT" >&2
    echo "details: docs/ops-repo.md" >&2
    exit 1
  fi
fi

# Template sources: neutral spine first, phase-1 workflows/_templates second.
TEMPLATES=""
for d in "$TEMPLATE_DIR/clients/_template" "$TEMPLATE_DIR/workflows/_templates"; do
  [ -d "$d" ] && { TEMPLATES="$d"; break; }
 done
[ -n "$TEMPLATES" ] || { echo "no template dir found (clients/_template or workflows/_templates)" >&2; exit 1; }

# Eval skeleton: clients/_template/evals is the current home. The second path is
# NOT dead code — an ops repo pins TURVEZ_REF to a release tag, and releases up
# to v0.1.2 kept the eval template at the repo root (evals/_template). Dropping
# the fallback would break scaffolding from a pinned older checkout.
EVAL_TEMPLATE=""
for d in "$TEMPLATE_DIR/clients/_template/evals" "$TEMPLATE_DIR/evals/_template"; do
  [ -d "$d" ] && { EVAL_TEMPLATE="$d"; break; }
done

C="$ROOT/clients/$CLIENT"
WF="$C/automation/dagnats"
WMW="$C/automation/windmill"
EV="$C/evals"
[ -e "$C" ] && { echo "already exists: $C" >&2; exit 1; }
mkdir -p "$C/automation/$ENGINE" "$EV" "$ROOT/env" "$ROOT/secrets/$CLIENT"

# Engine/runtime manifest (client.yaml) — the single source of truth for where
# and how this client runs (docs/client-spine.md). The template ships
# `engine: windmill`; pin the chosen engine so the manifest never contradicts
# the directory that actually holds the source.
if [ -f "$TEMPLATES/client.yaml" ]; then
  sed "s/TODO-CLIENT/$CLIENT/g; s/^engine: .*/engine: $ENGINE/" "$TEMPLATES/client.yaml" > "$C/client.yaml"
fi

# Fieldbook from template (placeholder swap; portable upper-case for macOS bash 3.2).
UPPER="$(echo "$CLIENT" | tr 'a-z' 'A-Z')"
sed "s/TODO-CLIENT/$CLIENT/g; s/TODO_SLACK_WEBHOOK_URL/${UPPER}_SLACK_WEBHOOK_URL/g" \
  "$TEMPLATES/FIELDBOOK.md" > "$C/README.md"

# Connection ids Nango assigns (Connect UI picks them, not us). Empty at
# scaffold time; filled the first time the operator authorises a provider.
# In nango/, not at the client root: a root *.json is a legacy recipe to
# scripts/validate.sh and would be validated as a workflow.
if [ -f "$TEMPLATES/nango/connections.json" ]; then
  mkdir -p "$C/nango"
  sed "s/TODO-CLIENT/$CLIENT/g" "$TEMPLATES/nango/connections.json" > "$C/nango/connections.json"
fi

# Starter recipe (dagnats engine only): copy the cron-job skeleton with the
# client prefix, basename kept so the recipe name matches the file.
if [ "$ENGINE" = "dagnats" ] && [ -f "$TEMPLATES/cron-job.json" ]; then
  sed "s|TODO-CLIENT/|$CLIENT$SEP|g; s/TODO-CLIENT/$CLIENT/g" \
    "$TEMPLATES/cron-job.json" > "$WF/cron-job.json"
fi

# Eval scaffold: cases are generated from run history (see the eval template).
if [ -n "$EVAL_TEMPLATE" ]; then
  cp -R "$EVAL_TEMPLATE/." "$EV/"
  if [ -f "$EV/promptfooconfig.yaml" ]; then
    sed "s/TODO-CLIENT/$CLIENT/g" "$EV/promptfooconfig.yaml" > "$EV/promptfooconfig.yaml.tmp" \
      && mv "$EV/promptfooconfig.yaml.tmp" "$EV/promptfooconfig.yaml"
  fi
fi

# Env template (names only).
UPPER_ENV="$(echo "$CLIENT" | tr 'a-z' 'A-Z')"
cat > "$ROOT/env/$CLIENT.env.example" <<EOF
# Secret NAMES for $CLIENT. Copy to env/$CLIENT.env (gitignored) and fill values,
# or encrypt: age -r age1YOURPUBLICKEY -o secrets/$CLIENT/env.age env/$CLIENT.env
#
# SaaS OAuth and provider API keys do NOT go here: register the client as a
# Nango connection and let the operator authorise it (docs/nango.md):
#   ./scripts/nango-connect.sh $CLIENT --provider <slug> --link --open
# This file holds the Nango key plus secrets that are ours (webhook URLs,
# internal APIs), never a provider credential.
NANGO_HOST=
NANGO_SECRET_KEY=
${UPPER_ENV}_SLACK_WEBHOOK_URL=
EOF

cat > "$ROOT/secrets/$CLIENT/.recipients" <<'EOF'
# One age1… public key per line (committed — public keys are public).
# Ask each operator for their key over an authenticated channel first.
EOF

echo "client scaffolded: $C (engine: $ENGINE)"
echo "next:"
echo "  \$EDITOR $C/README.md   # fill §1-3 (context, contacts, systems)"
echo "  \$EDITOR $C/client.yaml # runtime/target (engine is already $ENGINE)"
case "$ENGINE" in
  dagnats)
    echo "  dagnats workflow validate $WF/*.json"
    echo "  cd $TEMPLATE_DIR && WORKFLOWS_DIR=$ROOT/clients go test ./clients/"
    echo "  evals: $EV  (generate cases: scripts/select-cases.py --dagnats --out $EV/cases; label: scripts/label-cases.py --dir $EV)"
    ;;
  windmill)
    # An example flow, so the default engine is not born empty. An empty
    # automation/windmill/ lints "ok" (there is nothing to check), which reads as
    # a passing gate while validating nothing.
    #
    # The node is Go on purpose: script nodes are Go in this repo
    # (docs/language-policy.md). Windmill infers the input schema and the
    # generated UI from `func main`'s signature, so the types are checked by the
    # compiler instead of guessed by an LLM.
    mkdir -p "$WMW/example-flow"
    cat > "$WMW/example-flow/flow.yaml" <<'WMFLOWEOF'
# Example flow — replace with the real one.
# `input_transforms` is REQUIRED by the flow schema even when a step takes no
# arguments; omitting it makes `wmill lint` reject the whole flow.
summary: "TODO replace with the real flow summary"
value:
  modules:
    - id: step_one
      summary: first step
      value:
        type: rawscript
        language: go
        input_transforms: {}
        content: '!inline step_one.go'
WMFLOWEOF
    cat > "$WMW/example-flow/step_one.go" <<'WMGOEOF'
package inner

// main is the step entrypoint. Its parameters become the step's input schema
// and generated UI; it must return (interface{}, error).
// Secrets stay in workspace variables — read them with the Windmill Go client,
// never inline here.
func main() (interface{}, error) {
	return map[string]any{"ok": true}, nil
}
WMGOEOF
    echo "  put the flows under $WMW (wmill flow push one item at a time)"
    echo "  script nodes are Go; TS/Python only for a WAC orchestrator (docs/language-policy.md)"
    echo "  read skills/turvez/references/windmill.md first: never \`wmill sync push\`"
    echo "  ./scripts/check-windmill.sh   # lint gate, then wmill flow preview"
    echo "  ./scripts/lint-scripts.sh     # compile + binding gate, no server needed"
    echo "  evals: $EV  (normalize a job: scripts/windmill-job-to-run.py --job-id <id> --out run.json)"
    ;;
  dagu)
    # A DAG example, because a client folder with an empty automation/dagu/ fails
    # validation (there is nothing to validate) and the operator would not know
    # whether the scaffold or their work was wrong.
    cat > "$C/automation/dagu/nightly.yaml" <<'DAGUEOF'
# Example DAG — replace with the real one. Level 2 only: script-shaped recurrent
# jobs (docs/script-execution.md). dagu is not a product surface.
steps:
  - name: fetch
    command: echo "replace with the real command"
  - name: report
    command: echo "replace with the real command"
    depends: [fetch]
DAGUEOF
    echo "  put the DAGs under $C/automation/dagu (level 2: docs/script-execution.md)"
    echo "  evals: $EV"
    ;;
esac
