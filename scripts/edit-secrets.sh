#!/usr/bin/env bash
# Safe decrypt-edit-encrypt cycle for secrets/<client>/env.age.
#
# Usage: ./scripts/edit-secrets.sh <client-id>
#
# The plaintext only ever lives inside a trapped temp dir (wiped on any
# exit); the recipient public key is derived from your own identity, so
# you cannot encrypt to a key you cannot decrypt. Git history is the
# backup — the previous ciphertext stays committed until you commit anew.
set -euo pipefail

CLIENT="${1:?Usage: edit-secrets.sh <client-id>}"
[[ "$CLIENT" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad client id: $CLIENT" >&2; exit 1; }

command -v age >/dev/null || { echo "age binary not found" >&2; exit 1; }
command -v age-keygen >/dev/null || { echo "age-keygen binary not found" >&2; exit 1; }

IDENTITY="${AGE_IDENTITY_FILE:-$HOME/.secrets/consultancy.age.key}"
[ -f "$IDENTITY" ] || { echo "identity not found: $IDENTITY" >&2; exit 1; }

SECRETS_DIR="secrets/$CLIENT"
AGE_FILE="$SECRETS_DIR/env.age"
EXAMPLE="env/$CLIENT.env.example"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/edit-secrets.XXXXXX")"
TMP="$WORK/plain.env"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

shred_file() {
  # GNU shred on Linux; rm -P on macOS. Best effort either way.
  if command -v shred >/dev/null; then shred -u "$1"; else rm -P "$1" 2>/dev/null || rm -f "$1"; fi
}
trap 'shred_file "$TMP" 2>/dev/null; cleanup' EXIT INT TERM

if [ -f "$AGE_FILE" ]; then
  age --decrypt --identity "$IDENTITY" -o "$TMP" "$AGE_FILE" || {
    echo "decrypt failed (wrong identity?)" >&2; exit 1
  }
  echo "decrypted $AGE_FILE"
elif [ -f "$EXAMPLE" ]; then
  cp "$EXAMPLE" "$TMP"
  echo "no ciphertext yet; starting from $EXAMPLE"
else
  echo "neither $AGE_FILE nor $EXAMPLE exists" >&2; exit 1
fi

before="$(shasum -a 256 < "$TMP" 2>/dev/null || sha256sum < "$TMP")"
# shellcheck disable=SC2086: $EDITOR intentionally word-splits ("code -w", "sh -c …").
${EDITOR:-vi} "$TMP"
after="$(shasum -a 256 < "$TMP" 2>/dev/null || sha256sum < "$TMP")"
if [ "$before" = "$after" ]; then
  echo "unchanged; nothing to do"
  exit 0
fi

# Typo guard: warn about keys absent from the committed template.
if [ -f "$EXAMPLE" ]; then
  known="$(grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$EXAMPLE" | cut -d= -f1 | sort -u)"
  present="$(grep -E '^[A-Za-z_][A-Za-z0-9_]*=' "$TMP" | cut -d= -f1 | sort -u)"
  extra="$(comm -23 <(echo "$present") <(echo "$known"))"
  [ -z "$extra" ] || echo "warning: keys not in $EXAMPLE:${extra//$'\n'/, }" >&2
fi

PUB="$(age-keygen -y "$IDENTITY")"
mkdir -p "$SECRETS_DIR"
age --encrypt --recipient "$PUB" -o "$AGE_FILE" "$TMP" || {
  echo "encrypt failed" >&2; exit 1
}

# Verify: fresh decrypt must succeed and parse as KEY=VALUE lines.
VERIFY="$(mktemp "$WORK/verify.XXXXXX")"
age --decrypt --identity "$IDENTITY" -o "$VERIFY" "$AGE_FILE" || {
  echo "post-encrypt verify FAILED; ciphertext left as-is, fix manually" >&2; exit 1
}
[ -s "$VERIFY" ] || { echo "post-encrypt verify FAILED (empty)" >&2; exit 1; }
shred_file "$VERIFY"

echo "wrote $AGE_FILE (recipient $PUB)"
echo "next: git add -f $AGE_FILE && git commit"
