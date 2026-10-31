#!/usr/bin/env bash
# Install repo-local git hooks (opt-in, local only, no dependencies).
#
# Git uses .githooks/ for this repo (core.hooksPath), so the hooks survive
# clones and coexist with a global core.hooksPath: each wrapper runs the
# global hook of the same name first, then the turvez check.
#
# pre-commit — recipe contract tests (template checkout only).
# commit-msg — Conventional Commits, so release-please can derive the version.
#              Merge, revert, and fixup messages are exempt.
#
# Usage: ./scripts/install-hooks.sh
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
HOOKS_DIR="$ROOT/.githooks"
mkdir -p "$HOOKS_DIR"

cat > "$HOOKS_DIR/pre-commit" <<'EOF'
#!/usr/bin/env bash
# turvez pre-commit: chain the global hook, then test the recipes.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
global="$(git config --global --get core.hooksPath || true)"
if [ -n "$global" ] && [ -x "$global/pre-commit" ]; then "$global/pre-commit" "$@"; fi
[ -f "$root/go.mod" ] || exit 0
cd "$root"
go test ./clients/ || { echo "commit blocked: clients contract failed"; exit 1; }
EOF

cat > "$HOOKS_DIR/commit-msg" <<'EOF'
#!/usr/bin/env bash
# turvez commit-msg: chain the global hook, then enforce Conventional Commits.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
global="$(git config --global --get core.hooksPath || true)"
if [ -n "$global" ] && [ -x "$global/commit-msg" ]; then "$global/commit-msg" "$@"; fi
lint="$root/scripts/commit-msg-lint.sh"
[ -x "$lint" ] || exit 0
exec "$lint" --file "$1"
EOF

chmod +x "$HOOKS_DIR/pre-commit" "$HOOKS_DIR/commit-msg"
git -C "$ROOT" config --local core.hooksPath .githooks

# Legacy cleanup: older versions of this script wrote .git/hooks/pre-commit.
# That path is not versioned and not used while core.hooksPath points at
# .githooks, but the global hook chains to it by absolute path, so a stale copy
# keeps running the old command (e.g. `go test ./workflows/` after the layout
# moved). Remove it only when it is ours — never delete another tool's hook.
LEGACY="$(git -C "$ROOT" rev-parse --git-dir)/hooks/pre-commit"
if [ -f "$LEGACY" ] && grep -q "turvez pre-commit" "$LEGACY" 2>/dev/null; then
  rm -f "$LEGACY"
  echo "removed stale legacy hook: $LEGACY"
fi

echo "installed: core.hooksPath=.githooks"
echo "  pre-commit: global hook + recipe tests (template checkout only)"
echo "  commit-msg: global hook + Conventional Commits"