#!/usr/bin/env bash
# nango-connect.sh — hand the operator a Nango link and the exact names to use.
#
# The agent never asks for a client's OAuth token or API key. It asks the
# operator to authorise the provider in Nango, and prints the identifiers so the
# connection lands under the names the automation already reads (docs/nango.md).
#
# Usage:
#   ./scripts/nango-connect.sh <client> --provider hubspot
#   ./scripts/nango-connect.sh <client> --provider hubspot --link --open
#   ./scripts/nango-connect.sh <client> --provider google-calendar --open
#   ./scripts/nango-connect.sh <client> --provider slack --subaccount billing
#
# --link creates a 30-minute Click-to-connect magic link (Nango
# POST /connect/sessions) tagged with the client; it needs NANGO_SECRET_KEY.
# The connection it creates gets Nango's own id (a UUID) — the tag is how you
# find it again. --open opens the link (or the Connect UI / dashboard when
# there is no link).
#
# Naming (enforced by clients/credentials_test.go):
#   integration key (Provider-Config-Key) : the provider slug, one per provider
#   connection id   (Connection-Id)       : the client slug, or <client>:<subaccount>
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"

NANGO_HOST="${NANGO_HOST:-http://localhost:8088}"
NANGO_CONNECT_URL="${NANGO_CONNECT_URL:-http://localhost:${NANGO_CONNECT_PORT:-8089}}"

CLIENT=""
PROVIDER=""
SUBACCOUNT=""
OPEN=0
LINK=0
while [ $# -gt 0 ]; do
  case "$1" in
    --provider) PROVIDER="${2:-}"; shift 2 ;;
    --provider=*) PROVIDER="${1#--provider=}"; shift ;;
    --subaccount) SUBACCOUNT="${2:-}"; shift 2 ;;
    --subaccount=*) SUBACCOUNT="${1#--subaccount=}"; shift ;;
    --open) OPEN=1; shift ;;
    --link) LINK=1; shift ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) CLIENT="$1"; shift ;;
  esac
done

[ -n "$CLIENT" ] || { echo "usage: nango-connect.sh <client> --provider <provider-slug> [--subaccount <name>] [--open]" >&2; exit 2; }
[ -n "$PROVIDER" ] || { echo "--provider is required (the Nango integration key, e.g. hubspot)" >&2; exit 2; }
[[ "$CLIENT" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad client id: $CLIENT (lowercase, digits, dashes)" >&2; exit 2; }
[[ "$PROVIDER" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "bad provider: $PROVIDER (lowercase provider slug, e.g. google-calendar)" >&2; exit 2; }

CONNECTION_ID="$CLIENT"
[ -n "$SUBACCOUNT" ] && CONNECTION_ID="$CLIENT:$SUBACCOUNT"

echo "== Nango connection for $CLIENT ($PROVIDER)"
echo
if curl -s -f -m 3 "$NANGO_HOST/health" >/dev/null 2>&1; then
  echo "nango: running at $NANGO_HOST"
else
  echo "nango: NOT running at $NANGO_HOST"
  echo "  start it first: NANGO_CLIENT_ID=<client> ./scripts/setup-nango.sh --up"
fi
echo
echo "1. Open the dashboard and add the integration IF IT IS MISSING:"
echo "     $NANGO_HOST"
echo
echo "   integration key (Provider-Config-Key) : $PROVIDER"
echo "   intended connection id                : $CONNECTION_ID"
echo
echo "   One integration per provider, shared by every client. The client is the"
echo "   CONNECTION, never the integration: two integrations for one provider means"
echo "   a token renewal fixes only one tenant."
echo
echo "2. Have the operator authorise the provider for this client:"
echo "     $NANGO_CONNECT_URL        # Connect UI, one-click per provider"
echo "   The operator clicks; the agent never sees the token."
echo
echo "   Connect UI does not let us pick the connection id: it makes one (a UUID)"
echo "   and copies the session tag turvez_client=$CLIENT onto it. A recipe that"
echo "   names Connection-Id: $CONNECTION_ID needs a connection created WITH that"
echo "   id; if it came from the UI instead, read the id it got:"
echo "     curl -s -H \"Authorization: Bearer \$NANGO_SECRET_KEY\" \\"
echo "       \"$NANGO_HOST/connections?tags[turvez_client]=$CLIENT\" \\"
echo "       | python3 -c 'import json,sys; print([c[\"connection_id\"] for c in json.load(sys.stdin)[\"connections\"] if c[\"provider_config_key\"]==\"$PROVIDER\"])'"
echo
echo "3. Verify the connection exists and is healthy (the id is the client"
echo "   slug only when it was created that way):"
echo "     curl -s -H \"Authorization: Bearer \$NANGO_SECRET_KEY\" \\"
echo "       \"$NANGO_HOST/connections/$CONNECTION_ID?provider_config_key=$PROVIDER\" \\"
echo "       | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[\"credentials\"][\"type\"], d.get(\"created_at\"))'"
echo
echo "4. The automation reads only these two names (never the token):"
echo "     NANGO_HOST=$NANGO_HOST"
echo "     NANGO_SECRET_KEY                                  # workspace secret / age"
echo
echo "   then reference it as:"
echo "     \"headers\": {\"Provider-Config-Key\": \"$PROVIDER\", \"Connection-Id\": \"$CONNECTION_ID\"},"
echo "     \"auth_secret\": \"NANGO_SECRET_KEY\""
echo

if [ "$LINK" -eq 1 ]; then
  if [ -z "${NANGO_SECRET_KEY:-}" ]; then
    echo "--link needs NANGO_SECRET_KEY exported (the environment API key)" >&2
    exit 2
  fi
  # A magic link the operator can click: one integration, tagged with the client
  # so the connection can be found again. The connection gets Nango's own id (a
  # UUID), not the client slug, so the follow-up command reads the id back by
  # tag. Never returns a provider token: the operator's browser does the OAuth
  # dance (Nango POST /connect/sessions, 30-minute session).
  body="{\"tags\":{\"turvez_client\":\"$CLIENT\"},\"allowed_integrations\":[\"$PROVIDER\"]}"
  resp="$(curl -s -X POST "$NANGO_HOST/connect/sessions" \
    -H "Authorization: Bearer $NANGO_SECRET_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" 2>/dev/null || true)"
  link="$(printf '%s' "$resp" | python3 -c 'import json,sys; print((json.load(sys.stdin).get("data") or {}).get("connect_link",""))' 2>/dev/null || true)"
  if [ -n "$link" ]; then
    echo "connect link (30 min, one integration):"
    echo "  $link"
    if [ "$OPEN" -eq 1 ]; then
      opener=""
      command -v open >/dev/null 2>&1 && opener=open
      command -v xdg-open >/dev/null 2>&1 && opener=xdg-open
      [ -n "$opener" ] && "$opener" "$link" >/dev/null 2>&1 || true
    fi
    echo
    echo "after authorising, read back the connection id it got (Connect UI does"
    echo "not let us choose it) — a recipe needs this id, not the client slug:"
    echo "  curl -s -H \"Authorization: Bearer \$NANGO_SECRET_KEY\" \\"
    echo "    \"$NANGO_HOST/connections?tags[turvez_client]=$CLIENT\" \\"
    echo "    | jq -r '.connections[] | select(.provider_config_key==\"$PROVIDER\") | .connection_id'"
    echo
    echo "then record it so the guard and the recipe agree:"
    echo "  clients/$CLIENT/nango/connections.json  ->  \"$PROVIDER\": \"<id>\""
  else
    echo "could not create a connect session — is Nango reachable and the key valid?" >&2
    echo "  response: ${resp:-<none>}" >&2
  fi
  echo
fi

if [ "$OPEN" -eq 1 ]; then
  opener=""
  command -v open >/dev/null 2>&1 && opener=open
  command -v xdg-open >/dev/null 2>&1 && opener=xdg-open
  if [ -n "$opener" ]; then
    "$opener" "$NANGO_CONNECT_URL" >/dev/null 2>&1 || "$opener" "$NANGO_HOST" >/dev/null 2>&1 || true
    echo "opened $NANGO_CONNECT_URL"
  else
    echo "no opener on this host; open the URL above manually"
  fi
fi

