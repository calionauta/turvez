#!/usr/bin/env bash
# Install git hooks for this repo (opt-in, local only, no dependencies).
#
# pre-commit: fail the commit if a secret-looking value lands in a recipe,
# or a real client lands in the public template (ops repos are exempt —
# WORKFLOWS_DIR set skips the template-only guard).
# Same scan CI runs via `go test ./workflows/` — the hook just moves it left,
# before history is written (gogogo runs gitleaks here; this repo's scan is
# the zero-dependency equivalent).
#
# Usage: ./scripts/install-hooks.sh
set -euo pipefail

HOOK_DIR="$(git rev-parse --git-dir)/hooks"
cat > "$HOOK_DIR/pre-commit" <<'EOF'
#!/usr/bin/env bash
# turvez pre-commit: contract tests on recipes (fast, offline).
go test ./workflows/ || { echo "commit blocked: workflows contract failed"; exit 1; }
EOF
chmod +x "$HOOK_DIR/pre-commit"
echo "installed: $HOOK_DIR/pre-commit"
