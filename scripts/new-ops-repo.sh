#!/usr/bin/env bash
# Scaffold a private data-only ops repo (turvez-ops).
#
# Usage:
#   ./scripts/new-ops-repo.sh <path> [--github-private]
#
# What it creates (no Go code — engine stays in this template checkout):
#   <path>/TURVEZ_PIN                  # pins template SHA + dagnats version
#   <path>/AGENTS.md                   # ops-specific agent rules
#   <path>/README.md                   # what lives here, operator loop
#   <path>/.github/workflows/contract.yml  # CI: template tests @TURVEZ_PIN vs this repo
#   <path>/workflows/personal/         # your own namespace (from template)
#   <path>/workflows/ops/              # platform overlay, watchdog included
#   <path>/env/personal.env.example    # secret NAMES template
#   <path>/secrets/README.md           # ciphertext-only reminder
#   <path>/docker-compose.override.yml # CLIENT_ID + age mounts
#   <path>/.gitignore                  # env/*.env, *.age.key
#
# Real clients are added later with ./scripts/new-client.sh <client>
# (run inside the ops repo, or with TURVEZ_OPS_DIR set).
set -euo pipefail

[ -n "${1:-}" ] || { echo "Usage: new-ops-repo.sh <path> [--github-private] (local-only unless flagged)" >&2; exit 1; }
DEST="$1"
GH=0
[ "${2:-}" = "--github-private" ] && GH=1

# Fail fast: --github-private pushes to GitHub at the end; without gh the
# scaffold would be left half-handed with a confusing auth error.
if [ "$GH" -eq 1 ] && ! command -v gh >/dev/null; then
  echo "gh CLI not found (needed for --github-private)" >&2
  echo "install: https://cli.github.com/ (brew install gh), then re-run; or drop the flag for a local-only scaffold" >&2
  exit 1
fi

TEMPLATE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
[ -f "$TEMPLATE_DIR/go.mod" ] || { echo "run from the turvez template checkout" >&2; exit 1; }
[ -e "$DEST" ] && { echo "already exists: $DEST" >&2; exit 1; }

REF="$(git -C "$TEMPLATE_DIR" rev-parse HEAD 2>/dev/null || echo "unknown")"
DAGNATS_V="v0.0.30"

mkdir -p "$DEST/workflows/personal" "$DEST/workflows/ops" "$DEST/env" "$DEST/secrets"

cat > "$DEST/TURVEZ_PIN" <<EOF
# Pinned engine source. Worker image builds from this turvez ref only.
TURVEZ_REF=$REF
DAGNATS_VERSION=$DAGNATS_V
EOF

cat > "$DEST/.gitignore" <<'EOF'
# Real values — never commit
env/*.env
.env
*.age.key
*identity*.txt
# Editor / OS noise
.DS_Store
.idea/
.vscode/
*.swp
# Local agent memory
.engram/
EOF

cat > "$DEST/AGENTS.md" <<'EOF'
# AGENTS.md — turvez-ops (private data repo)

## Repo type

Data-only. No Go code. Engine = turvez template at `TURVEZ_PIN`.
Validate from the template checkout (go needs its module context):
`OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/workflows go test ./workflows/`.

## Layout

- `workflows/<client>/README.md` — fieldbook (copy from template `workflows/_templates/FIELDBOOK.md`).
- `workflows/<client>/*.json` — recipes, names `<client>/…`, triggers `<client>-…`.
- `env/<client>.env.example` — secret NAMES (commit); `env/<client>.env` — values (never).
- `secrets/<client>/env.age` — ciphertext (commit with -f); `secrets/<client>/.recipients` — age pubkeys.

## Rules

- Secrets by NAME in JSON. Never values.
- `TURVEZ_CHECKOUT` env points at the template clone (validate/register/smoke run from there).
- Never push this repo to a public remote.

## Commands

```bash
export TURVEZ_CHECKOUT=~/Development/turvez TURVEZ_OPS_DIR=$PWD
OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/workflows go test ./workflows/
OPS=$PWD; cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$OPS/workflows ./scripts/register-workflows.sh <client>
$TURVEZ_CHECKOUT/scripts/new-client.sh <client>  # scaffold (TURVEZ_OPS_DIR supported)
```
EOF

cat > "$DEST/README.md" <<'EOF'
# turvez-ops — private client automations

Data repo. Engine lives in the public `turvez` template pinned at `TURVEZ_PIN`.
Git here is source of truth; dagnats KV is a deploy target.

```bash
export TURVEZ_CHECKOUT=~/Development/turvez TURVEZ_OPS_DIR=$PWD
$TURVEZ_CHECKOUT/scripts/setup.sh                 # check prereqs
$TURVEZ_CHECKOUT/scripts/new-client.sh acme       # scaffold workflows/acme/
cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$TURVEZ_OPS_DIR/workflows go test ./workflows/
cd $TURVEZ_CHECKOUT && WORKFLOWS_DIR=$TURVEZ_OPS_DIR/workflows ./scripts/register-workflows.sh acme
```
EOF

# Seed personal + ops overlays from the template (examples, safe to commit).
# ops/dlq-watchdog.json ships enabled: fresh ops repos keep the silent-DLQ
# layer (AGENTS.md) from day one; disable per client via trigger disable.
cp "$TEMPLATE_DIR/workflows/personal/README.md" "$DEST/workflows/personal/README.md" 2>/dev/null || true
cp "$TEMPLATE_DIR/workflows/personal/ping.json" "$DEST/workflows/personal/ping.json" 2>/dev/null || true
cp "$TEMPLATE_DIR/workflows/ops/README.md" "$DEST/workflows/ops/README.md" 2>/dev/null || true
cp "$TEMPLATE_DIR/workflows/ops/dlq-watchdog.json" "$DEST/workflows/ops/dlq-watchdog.json" 2>/dev/null || true
cp "$TEMPLATE_DIR/env/personal.env.example" "$DEST/env/personal.env.example" 2>/dev/null || true

mkdir -p "$DEST/.github/workflows"
cat > "$DEST/.github/workflows/contract.yml" <<'EOF'
# Ops contract CI: template tests (pinned by TURVEZ_PIN) against this repo.
# Single source of truth = TURVEZ_PIN file; no SHA mirrored anywhere else.
name: contract
on: [push, pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { path: ops }
      - uses: actions/checkout@v4
        with: { repository: calionauta/turvez, path: turvez }
      - uses: actions/setup-go@v5
        with: { go-version: '1.27' }
      - run: go install github.com/danmestas/dagnats/cmd/dagnats@v0.0.30
      - name: pin template to TURVEZ_PIN
        run: |
          REF="$(awk -F= '/^TURVEZ_REF=/{print $2}' ops/TURVEZ_PIN)"
          git -C turvez fetch origin "$REF" && git -C turvez checkout "$REF"
      - run: WORKFLOWS_DIR=$GITHUB_WORKSPACE/ops/workflows go test ./workflows/ -timeout 300s
        working-directory: turvez
      - run: for f in $GITHUB_WORKSPACE/ops/workflows/*/*.json; do dagnats workflow validate "$f"; done
EOF

cat > "$DEST/secrets/README.md" <<'EOF'
# secrets/ — ciphertext only, never values
Per-client: `secrets/<client>/env.age` (commit with `git add -f`) +
`.recipients` (age public keys). Edit via template:
`CLIENT=acme make edit-secrets` with `TURVEZ_OPS_DIR=$PWD`, or
`$TURVEZ_CHECKOUT/scripts/edit-secrets.sh acme` (supports TURVEZ_OPS_DIR).
EOF

cat > "$DEST/docker-compose.override.yml" <<'EOF'
# Stage 1/2: one worker serves all clients. Stage 3: set CLIENT_ID=<client>.
services:
  worker:
    environment:
      CLIENT_ID: ${CLIENT_ID:-}
    env_file:
      - path: .env
        required: false
    # volumes:
    #   - ~/.secrets:/run/secrets:ro
EOF

echo "ops repo scaffolded at $DEST (TURVEZ_REF=$REF)"
echo "next:"
echo "  cd $DEST && git init && git add -A && git commit -m 'ops init (turvez $REF)'"
if [ "$GH" -eq 1 ]; then
  (cd "$DEST" && gh repo create --private --source=. --push) && echo "pushed private to GitHub"
else
  echo "  cd $DEST && gh repo create --private --source=. --push   # optional"
fi
echo "  export TURVEZ_CHECKOUT=$TEMPLATE_DIR TURVEZ_OPS_DIR=$DEST"
