#!/usr/bin/env bash
# Run an untrusted script inside a restricted local container (free sandbox).
#
# Defaults are deny-by-default: no network, read-only fs, nobody user,
# capped CPU/mem/pids. Use --allow-net=on only when the script must fetch
# (still no secrets in env unless you pass --secret-env).
#
# Usage:
#   ./scripts/sandbox-run.sh [--image=python:3.12-slim] [--allow-net=off|on]
#                            [--timeout=30] [--secret-env=FILE] <script> [args...]
#   ./scripts/sandbox-run.sh --allow-net=off report.py -- --date 2026-10-24
#
# Contract: repo is mounted read-only at /work; script writes to ./sandbox-out/
# (mounted at /out). Read the script before running — this contains the blast
# radius, it does not audit malicious logic.
set -euo pipefail

IMAGE="python:3.12-slim"
ALLOW_NET="off"
TIMEOUT="30"
SECRET_ENV=""
SCRIPT=""
SCRIPT_ARGS=()

while [ $# -gt 0 ]; do
  case "$1" in
    --image=*) IMAGE="${1#--image=}"; shift ;;
    --allow-net=*) ALLOW_NET="${1#--allow-net=}"; shift ;;
    --timeout=*) TIMEOUT="${1#--timeout=}"; shift ;;
    --secret-env=*) SECRET_ENV="${1#--secret-env=}"; shift ;;
    --) shift; while [ $# -gt 0 ]; do SCRIPT_ARGS+=("$1"); shift; done ;;
    --*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) if [ -z "$SCRIPT" ]; then SCRIPT="$1"; else SCRIPT_ARGS+=("$1"); fi; shift ;;
  esac
done

if [ -z "$SCRIPT" ]; then
  echo "usage: $0 [--image=..] [--allow-net=off|on] [--timeout=30] <script> [args...]" >&2
  exit 2
fi

command -v docker >/dev/null || { echo "docker not found" >&2; exit 1; }
[ -f "$SCRIPT" ] || { echo "script not found: $SCRIPT" >&2; exit 1; }

OUTDIR="$(pwd)/sandbox-out"
mkdir -p "$OUTDIR"
WORKDIR="$(pwd)"
# The repo is the only host tree visible inside (mounted at /work:ro).
# Scripts outside it are invisible by design — copy them in first.
case "$(cd "$(dirname "$SCRIPT")" && pwd)/" in
  "$WORKDIR"/*) ;;
  *) echo "script must live under $WORKDIR (sandbox only mounts the repo at /work:ro). Copy it in first." >&2; exit 2 ;;
esac
SCRIPT_ABS="$(cd "$(dirname "$SCRIPT")" && pwd)/$(basename "$SCRIPT")"
# Container-side path: /work/<relpath>
SCRIPT_REL="${SCRIPT_ABS#$WORKDIR/}"

# macOS has no `timeout(1)`; use gtimeout when present (brew coreutils),
# else rely on docker --stop-timeout + handler timeouts.
TIMEOUT_BIN=""
command -v timeout >/dev/null && TIMEOUT_BIN="timeout ${TIMEOUT}s"
command -v gtimeout >/dev/null && TIMEOUT_BIN="gtimeout ${TIMEOUT}s"

NET_FLAG="--network=none"
[ "$ALLOW_NET" = "on" ] && NET_FLAG="--network=bridge"

echo "== sandbox: image=$IMAGE net=$ALLOW_NET timeout=${TIMEOUT}s"
echo "== script: $SCRIPT_ABS | out: $OUTDIR/"
# shellcheck disable=SC2086
if [ -n "$SECRET_ENV" ]; then
  [ -f "$SECRET_ENV" ] || { echo "secret env not found: $SECRET_ENV" >&2; exit 1; }
  echo "WARNING: mounting secrets into sandbox ($SECRET_ENV). Prefer names-only + staging." >&2
  # shellcheck disable=SC2086
  $TIMEOUT_BIN docker run --rm \
    $NET_FLAG \
    --read-only \
    --user 65534:65534 \
    --memory=512m --cpus=1.0 --pids-limit=128 \
    --cap-drop=ALL --security-opt=no-new-privileges \
    -v "$WORKDIR:/work:ro" \
    -v "$OUTDIR:/out:rw" \
    --tmpfs /tmp:rw,noexec,nosuid,size=64m \
    --workdir /work \
    --env-file "$SECRET_ENV" \
    "$IMAGE" python3 "/work/$SCRIPT_REL" ${SCRIPT_ARGS[@]+"${SCRIPT_ARGS[@]}"}
else
  # shellcheck disable=SC2086
  $TIMEOUT_BIN docker run --rm \
    $NET_FLAG \
    --read-only \
    --user 65534:65534 \
    --memory=512m --cpus=1.0 --pids-limit=128 \
    --cap-drop=ALL --security-opt=no-new-privileges \
    -v "$WORKDIR:/work:ro" \
    -v "$OUTDIR:/out:rw" \
    --tmpfs /tmp:rw,noexec,nosuid,size=64m \
    --workdir /work \
    "$IMAGE" python3 "/work/$SCRIPT_REL" ${SCRIPT_ARGS[@]+"${SCRIPT_ARGS[@]}"}
fi
echo "== done — outputs in $OUTDIR/"
