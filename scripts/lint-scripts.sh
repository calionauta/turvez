#!/usr/bin/env bash
# lint-scripts.sh — local gate for Windmill automation scripts and bindings.
#
# Enforces the language policy (docs/language-policy.md): script nodes are Go.
# TypeScript and Python are allowed only for a workflow-as-code orchestrator
# that dispatches to Go scripts with taskScript()/taskFlow().
#
# Catches what `wmill lint` cannot see: syntax and type errors, a missing
# `func main`, signature/`input_transforms` mismatches, and policy violations.
#
# Usage:
#   ./scripts/lint-scripts.sh
#   TURVEZ_OPS_DIR=../turvez-ops ./scripts/lint-scripts.sh

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="${TURVEZ_OPS_DIR:-$(cd "$HERE/.." && pwd)}"

python3 "$HERE/lint-scripts.py" "$@"
