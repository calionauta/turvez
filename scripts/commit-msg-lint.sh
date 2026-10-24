#!/usr/bin/env bash
# Conventional Commits check shared by the commit-msg hook and CI.
#
# Usage:
#   scripts/commit-msg-lint.sh --file <message-file>   # git hook
#   scripts/commit-msg-lint.sh --range <rev-range>     # CI (git log subjects)
set -euo pipefail

valid='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9._/-]+\))?!?: .+'

check() {
  local subject="$1"
  case "$subject" in
    "Merge "*|"Revert "*|"fixup! "*|"squash! "*|"amend! "*|"Initial commit"*) return 0 ;;
  esac
  printf '%s\n' "$subject" | grep -Eq "$valid"
}

usage() {
  echo "usage: $0 --file <message-file> | --range <rev-range>" >&2
  exit 2
}

[ $# -eq 2 ] || usage

case "$1" in
  --file)
    subject="$(head -n 1 "$2")"
    if check "$subject"; then exit 0; fi
    cat >&2 <<'MSG'
commit blocked: message must follow Conventional Commits.
  feat: add invoice approval
  fix(api)!: drop legacy webhook
  docs: pin ops repos to release tags

Allowed types: feat fix docs style refactor perf test build ci chore revert.
See AGENTS.md ("Commits and releases").
MSG
    exit 1
    ;;
  --range)
    failed=0
    while IFS= read -r subject; do
      if ! check "$subject"; then
        echo "invalid commit subject: $subject" >&2
        failed=1
      fi
    done < <(git log --format=%s "$2")
    if [ "$failed" -ne 0 ]; then
      echo "Conventional Commits required (see AGENTS.md)." >&2
      exit 1
    fi
    ;;
  *) usage ;;
esac
