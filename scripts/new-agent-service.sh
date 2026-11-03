#!/usr/bin/env bash
# new-agent-service.sh — scaffold the Genkit capability for one client.
#
# Genkit is a capability, not an engine (docs/genkit.md): a bounded loop that
# runs inside one request, in a Go service we own, calling the same goai client
# the recipes use. The decision tree asks first (references/decision-tree.md,
# Q1.5); this script is what runs after the answer is "yes, a human is watching".
#
# Default is a PLAN. --apply writes the files and prints the dependency
# command; it never runs `go get` itself, because that edits go.mod and is your
# call (and a deliberate act in review).
#
# Usage:
#   ./scripts/new-agent-service.sh <client>            # print the plan
#   ./scripts/new-agent-service.sh <client> --apply    # write the scaffold
#   ./scripts/new-agent-service.sh <client> --apply --provider hubspot
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${TURVEZ_OPS_DIR:-$(cd "$HERE/.." && pwd)}"

CLIENT=""; APPLY=0; PROVIDER="hubspot"
while [ $# -gt 0 ]; do
  case "$1" in
    --apply) APPLY=1; shift ;;
    --provider) PROVIDER="${2:-}"; shift 2 ;;
    --provider=*) PROVIDER="${1#--provider=}"; shift ;;
    -*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) CLIENT="$1"; shift ;;
  esac
done

[ -n "$CLIENT" ] || { echo "usage: $0 <client> [--apply] [--provider <slug>]" >&2; exit 2; }
[[ "$CLIENT" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad client id: $CLIENT" >&2; exit 2; }

# The service is Go; an ops repo is data-only (no go.mod, docs/ops-repo.md).
# Say it here, where the plan is printed, not after a build fails.
if [ ! -f "$ROOT/go.mod" ]; then
  echo "note: $ROOT has no go.mod — an ops repo is data-only (docs/ops-repo.md)."
  echo "      run this from a Go checkout (the template or a service repo), with"
  echo "      TURVEZ_OPS_DIR pointing at the repo that holds clients/."
  echo
fi

CDIR="$ROOT/clients/$CLIENT"
[ -d "$CDIR" ] || { echo "no such client: $CDIR (scaffold it first: ./scripts/new-client.sh $CLIENT)" >&2; exit 1; }

ENGINE="$(sed -n 's/^engine:[[:space:]]*//p' "$CDIR/client.yaml" 2>/dev/null | head -1 | sed 's/#.*//; s/[[:space:]]*$//')"
[ -n "$ENGINE" ] || ENGINE="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["default"])' "$ROOT/engines.json" 2>/dev/null || echo windmill)"

SVC_DIR="$ROOT/cmd/$CLIENT-agent"
UP="$(echo "$CLIENT" | tr 'a-z' 'A-Z')"

echo "genkit capability for $CLIENT"
echo "  engine (unchanged)     : $ENGINE        # one engine per client; Genkit is not one"
echo "  service                : cmd/$CLIENT-agent/            (Go, HTTP + SSE)"
echo "  flow                   : bounded writer→critic loop, goai as the only model path"
echo "  tools                  : Nango proxy, Provider-Config-Key $PROVIDER (default — pass --provider), Connection-Id $CLIENT"
echo "  durable tail           : emit a run to $ENGINE, idempotency key from the request"
echo "  prompt record          : ${UP}_AGENT store, written before generation (docs/observability.md)"
echo

TMPL="$HERE/templates/agent-service/main.go.tmpl"
[ -f "$TMPL" ] || { echo "missing template: $TMPL" >&2; exit 1; }

if [ "$APPLY" -eq 0 ]; then
  echo "plan only. Files it would write:"
  echo "  $SVC_DIR/main.go        (from scripts/templates/agent-service/main.go.tmpl)"
  echo "  $SVC_DIR/README.md      (runbook: env, bounds, durable tail)"
  echo
  echo "then, in review:"
  echo "  go get github.com/firebase/genkit/go@latest"
  echo "  gofmt -l ./cmd/$CLIENT-agent && go build ./... && go test ./..."
  echo
  echo "re-run with --apply to write them."
  exit 0
fi

[ -e "$SVC_DIR" ] && { echo "already exists: $SVC_DIR" >&2; exit 1; }
mkdir -p "$SVC_DIR"
sed "s/TODO-CLIENT/$CLIENT/g; s/TODO-PROVIDER/$PROVIDER/g" "$TMPL" > "$SVC_DIR/main.go"
gofmt -w "$SVC_DIR/main.go"

cat > "$SVC_DIR/README.md" <<EOF
# $CLIENT agent (Genkit capability)

Not an engine. The loop lives in this request; anything durable is a run in the
\`$ENGINE\` engine (\`docs/genkit.md\`).

## Run

\`\`\`bash
go get github.com/firebase/genkit/go@latest   # once, in review
export GOAI_BASE_URL=http://<gateway>/v1 GOAI_API_KEY=... GOAI_MODEL=...
export NANGO_HOST=http://localhost:8088 NANGO_SECRET_KEY=...
go run ./cmd/$CLIENT-agent -addr 127.0.0.1:8099
\`\`\`

\`POST /agent/run {"content": "..."}\` returns the bounded verdict;
\`GET /agent/health\` proves the model gateway and the Nango proxy answer.

## Contracts

- Bounded: \`maxRounds=3\`, \`runTimeout=90s\`, 6000-rune input cap, cancel on disconnect.
- One LLM path: the Genkit model action wraps goai (\`internal/handlers/llm_complete.go\`
  shows the same client); no provider plugins, no second retry layer.
- Prompt record: \`-record\` JSONL; point it at the client's store before go-live.
- Tools: Nango proxy with \`Provider-Config-Key: $PROVIDER\` + \`Connection-Id: $CLIENT\` — never a token.
- Durable tail: emit a \`$ENGINE\` run with an idempotency key from the request.

## Remove

Delete \`cmd/$CLIENT-agent/\`, then \`go mod tidy\` drops Genkit from the build.
EOF

echo "wrote $SVC_DIR/main.go"
echo "wrote $SVC_DIR/README.md"
echo
echo "next (in review — this edits go.mod):"
echo "  go get github.com/firebase/genkit/go@latest"
echo "  gofmt -l ./cmd/$CLIENT-agent && go build ./... && go test ./..."
