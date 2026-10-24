#!/usr/bin/env bash
# Install git hooks for this repo (opt-in, local only, no dependencies).
#
# pre-commit: fail the commit if a secret-looking value lands in a recipe.
# Same scan CI runs via `go test ./workflows/` — the hook just moves it left,
# before history is written (gogogo runs gitleaks here; this repo's scan is
# the zero-dependency equivalent).
#
# Usage: ./scripts/install-hooks.sh
set -euo pipefail

HOOK_DIR="$(git rev-parse --git-dir)/hooks"
cat > "$HOOK_DIR/pre-commit" <<'EOF'
#!/usr/bin/env bash
# turvez pre-commit: secret-value scan on recipes (fast, offline).
go test ./workflows/ -run TestRecipeStructure || { echo "commit blocked: secret scan failed"; exit 1; }
EOF
chmod +x "$HOOK_DIR/pre-commit"
echo "installed: $HOOK_DIR/pre-commit"
