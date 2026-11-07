#!/usr/bin/env bash
# connect-remotes.sh — record the Windmill and Nango endpoints this checkout
# uses, without ever echoing a secret.
#
# The operator is the only one who knows whether the workspace is Windmill
# Cloud, the client's own server, or ours. The repo records that answer
# (client.yaml) and refuses to infer it; this script gets it into the shell.
#
# Writes ~/.secrets/turvez-remotes.env (chmod 600, outside the repo) with the
# names the tools actually read:
#   BASE_INTERNAL_URL, WM_TOKEN, WM_WORKSPACE   (wmill CLI)
#   NANGO_HOST, NANGO_SECRET_KEY                (recipes, scripts/nango-*)
#
# Usage:
#   ./scripts/connect-remotes.sh            # prompt, validate, save
#   ./scripts/connect-remotes.sh --nango-key # prompt only for the NANGO secret key
#   ./scripts/connect-remotes.sh --client acme # per-client file
#   ./scripts/connect-remotes.sh --check    # re-validate what is saved
#   ./scripts/connect-remotes.sh --print    # which names are set (no values)
#
# After it:
#   set -a; . ~/.secrets/turvez-remotes.env; set +a
#
# Validation is read-only: one authenticated GET per remote. Nothing is
# created, pushed or run.
set -euo pipefail

SECRETS_DIR="${TURVEZ_SECRETS_DIR:-$HOME/.secrets}"
CLIENT=""
ARGS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --client=*) CLIENT="${1#--client=}"; shift ;;
    --client) CLIENT="${2:-}"; shift 2 ;;
    *) ARGS+=("$1"); shift ;;
  esac
done
FILE="$SECRETS_DIR/turvez-remotes.env"
[ -n "$CLIENT" ] && FILE="$SECRETS_DIR/turvez-remotes-$CLIENT.env"

action="${ARGS[0]:-prompt}"
case "$action" in
  prompt|--check|--print|--nango-key|--help|-h) ;;
  *) echo "connect-remotes: unknown argument '$action' (want --check | --print | --nango-key)" >&2; exit 2 ;;
esac

say() { printf '%s\n' "$*"; }
fail() { printf 'connect-remotes: %s\n' "$*" >&2; exit 1; }

shquote() { printf '%q' "$1"; }

normalize_url() {
  # Accept a bare host from the operator input; the tool they type in a browser
  # usually has a scheme, but scripts and pasted values often do not.
  case "$1" in
    http://*|https://*) printf '%s' "$1" ;;
    "") printf '' ;;
    *) printf 'https://%s' "$1" ;;
  esac
}

valid_url() {
  case "$1" in
    http://*|https://*) return 0 ;;
    *) return 1 ;;
  esac
}

http_code() { # url, token
  local url="$1" token="$2" code=""
  if [ -n "$token" ]; then
    code="$(curl -s -o /dev/null -w '%{http_code}' -m 12 \
      -H "Authorization: Bearer $token" -H 'Accept: application/json' "$url" 2>/dev/null || true)"
  else
    code="$(curl -s -o /dev/null -w '%{http_code}' -m 12 -H 'Accept: application/json' "$url" 2>/dev/null || true)"
  fi
  printf '%s' "${code:-000}"
}

check_windmill() { # base, token
  local code
  code="$(http_code "${1%/}/api/users/whoami" "$2")"
  case "$code" in
    200) return 0 ;;
    401|403) echo "windmill: token rejected (HTTP $code)" >&2; return 1 ;;
  esac
  # Older/adjusted deployments: prove the host answers, and say the token was
  # not verified rather than claiming success.
  code="$(http_code "${1%/}/api/version" "")"
  case "$code" in
    200) echo "windmill: host answers, token NOT verified (no /api/users/whoami)" >&2; return 0 ;;
  esac
  echo "windmill: no answer at ${1%/} (HTTP $code)" >&2
  return 1
}

check_nango() { # host, key
  local out code
  out="$(curl -s -w '\n%{http_code}' -m 12 -H "Authorization: Bearer $2" -H 'Accept: application/json' "${1%/}/integrations" 2>/dev/null || true)"
  code="${out##*$'\n'}"
  [ -z "$code" ] && code=000
  case "$code" in
    200) return 0 ;;
    401|403) echo "nango: secret key rejected (HTTP $code)" >&2; return 1 ;;
    400)
      if printf '%s' "$out" | grep -q 'missing_environment'; then
        echo "nango: secret key is not environment-scoped; use the Environment API key (not the account-level key)" >&2
      else
        echo "nango: request rejected (HTTP 400); check host and key" >&2
      fi
      return 1
      ;;
  esac
  echo "nango: no answer at ${1%/} (HTTP $code)" >&2
  return 1
}

prompt_value() { # label, current, secret?
  local label="$1" current="${2:-}" secret="${3:-no}" answer=""
  if [ "$secret" = "yes" ]; then
    if [ -n "$current" ]; then
      printf '%s [set, Enter keeps it]: ' "$label" >&2
    else
      printf '%s (empty to skip): ' "$label" >&2
    fi
    IFS= read -r -s answer || true
    echo >&2
  else
    if [ -n "$current" ]; then
      printf '%s [%s]: ' "$label" "$current" >&2
    else
      printf '%s: ' "$label" >&2
    fi
    IFS= read -r answer || true
  fi
  [ -z "$answer" ] && answer="$current"
  printf '%s' "$answer"
}

load_saved() {
  [ -f "$FILE" ] || return 0
  # Only the known names; the file is ours and 600.
  set -a
  # shellcheck disable=SC1090
  . "$FILE"
  set +a
}

do_print() {
  load_saved
  say "file: $FILE"
  for v in BASE_INTERNAL_URL WM_TOKEN WM_WORKSPACE NANGO_HOST NANGO_SECRET_KEY; do
    eval "val=\${$v:-}"
    if [ -n "$val" ]; then say "  $v = set"; else say "  $v = unset"; fi
  done
}

do_check() {
  load_saved
  local rc=0
  [ -n "${BASE_INTERNAL_URL:-}" ] || { echo "windmill: BASE_INTERNAL_URL unset" >&2; rc=1; }
  [ -n "${NANGO_HOST:-}" ] || { echo "nango: NANGO_HOST unset" >&2; rc=1; }
  [ -n "${BASE_INTERNAL_URL:-}" ] && { check_windmill "$BASE_INTERNAL_URL" "${WM_TOKEN:-}" || rc=1; }
  [ -n "${NANGO_HOST:-}" ] && { check_nango "$NANGO_HOST" "${NANGO_SECRET_KEY:-}" || rc=1; }
  [ "$rc" -eq 0 ] && say "remotes: both answer"
  return "$rc"
}

do_prompt() {
  load_saved
  say "Windmill (Cloud or self-hosted — the answer is recorded per client in client.yaml)"
  local wm_url wm_tok wm_ws
  # Bare hostnames are accepted; a scheme is normalized before validation.
  wm_url="$(prompt_value 'base URL (e.g. app.windmill.dev)' "${BASE_INTERNAL_URL:-}")"
  wm_tok="$(prompt_value 'API token' "${WM_TOKEN:-}" yes)"
  wm_ws="$(prompt_value 'workspace (one per client)' "${WM_WORKSPACE:-}")"
  say ""
  say "Nango (Cloud or self-hosted; the broker is per client by default)"
  local ng_url ng_key
  ng_url="$(prompt_value 'host (e.g. api.nango.dev)' "${NANGO_HOST:-}")"
  ng_key="$(prompt_value 'secret key' "${NANGO_SECRET_KEY:-}" yes)"

  local rc=0
  if [ -n "$wm_url" ]; then
    wm_url="$(normalize_url "$wm_url")"
    valid_url "$wm_url" || fail "windmill base URL must start with http:// or https://"
    check_windmill "$wm_url" "$wm_tok" || rc=1
  fi
  if [ -n "$ng_url" ]; then
    ng_url="$(normalize_url "$ng_url")"
    valid_url "$ng_url" || fail "nango host must start with http:// or https://"
    check_nango "$ng_url" "$ng_key" || rc=1
  fi
  [ "$rc" -eq 0 ] || fail "a remote did not validate; nothing was written"

  umask 077
  mkdir -p "$SECRETS_DIR"
  chmod 700 "$SECRETS_DIR" 2>/dev/null || true
  {
    echo "# turvez remotes — generated $(date -u +%Y-%m-%dT%H:%M:%SZ). Never commit."
    echo "# set -a; . $FILE; set +a"
    echo "export BASE_INTERNAL_URL=$(shquote "$wm_url")"
    echo "export WM_TOKEN=$(shquote "$wm_tok")"
    echo "export WM_WORKSPACE=$(shquote "$wm_ws")"
    echo "export NANGO_HOST=$(shquote "$ng_url")"
    echo "export NANGO_SECRET_KEY=$(shquote "$ng_key")"
  } > "$FILE"
  chmod 600 "$FILE"
  say ""
  say "saved: $FILE (600). Load it with:"
  say "  set -a; . $FILE; set +a"
}

do_nango_key() {
  load_saved
  local ng_url ng_key
  ng_url="$(normalize_url "${NANGO_HOST:-}")"
  [ -n "$ng_url" ] || fail "set NANGO_HOST first with $(basename "$0")"
  ng_key="$(prompt_value 'NANGO_SECRET_KEY (environment API key)' "${NANGO_SECRET_KEY:-}" yes)"
  [ -n "$ng_key" ] || fail "empty secret key"
  check_nango "$ng_url" "$ng_key" || fail "nango key did not validate; nothing was written"
  umask 077
  {
    echo "# turvez remotes — updated $(date -u +%Y-%m-%dT%H:%M:%SZ). Never commit."
    echo "# set -a; . $FILE; set +a"
    echo "export BASE_INTERNAL_URL=$(shquote "${BASE_INTERNAL_URL:-}")"
    echo "export WM_TOKEN=$(shquote "${WM_TOKEN:-}")"
    echo "export WM_WORKSPACE=$(shquote "${WM_WORKSPACE:-}")"
    echo "export NANGO_HOST=$(shquote "$ng_url")"
    echo "export NANGO_SECRET_KEY=$(shquote "$ng_key")"
  } > "$FILE"
  chmod 600 "$FILE"
  say "saved: $FILE (600). NANGO_SECRET_KEY updated; the other values were preserved."
}

case "$action" in
  --help|-h) sed -n '3,20p' "$0" | sed 's/^# \{0,1\}//' ;;
  --print) do_print ;;
  --check) do_check ;;
  --nango-key) do_nango_key ;;
  *) do_prompt ;;
esac
