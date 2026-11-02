# Integrating Nango with Turvez

Nango ([github.com/nangohq/nango](https://github.com/nangohq/nango)) is the **Auth & Integration Layer** for Turvez. It manages OAuth 2.0 flows, token storage, automatic token refreshing, and rate-limiting across 250+ SaaS integrations (HubSpot, Salesforce, Slack, Google Workspace, QuickBooks, Notion, Linear, Stripe, etc.).

Turvez is the orchestration spine (Windmill or dagnats); **Nango is the credential broker**.

---

## 1. The Problem Nango Solves

Without Nango, automations store static API keys or access tokens directly in Windmill variables or encrypted `secrets/<client>/env.age`:

```
[Client SaaS] <---- (Raw Access Token) ---- [Windmill / Dagnats Worker]
                                                    ^
                                            (Expired in 1h!)
```

### Why this breaks in production:
1. **Token Expiration & Refresh Rotation:** Modern SaaS APIs use OAuth 2.0 with short-lived access tokens (1–24 hours) and single-use rotating refresh tokens (RFC 6749 / OAuth BCP). If a script attempts to refresh tokens in a stateless worker, concurrent runs trigger race conditions that permanently revoke the refresh token (`invalid_grant`).
2. **Onboarding Friction:** Asking a client to create a custom developer app, generate redirect URIs, execute OAuth callbacks via curl, and transmit secrets over email/chat is insecure and prone to failure.
3. **Secret Sprawl:** Tokens are duplicated across multiple scripts, flows, and age files without centralized visibility or revocation.
4. **Maintenance Burden:** A human operator must manually regenerate and re-encrypt tokens whenever an authorization expires.

### How Nango fixes this:
```
[Client SaaS] <--- (Auto-refreshed Token) --- [Nango Proxy] <--- (Connection-Id) --- [Windmill / Dagnats]
      ^                                             ^
 (OAuth Dance)                                (Distributed Lock
      |                                        & Token Store)
 [Client User]
```

- **Zero Token Handling in Flows:** Scripts and workflows never see or store upstream OAuth secrets or refresh tokens.
- **Unified Proxy:** Flows make HTTP requests to the Nango Request Proxy (`/proxy/...`), passing only `Connection-Id` and `Provider-Config-Key`. Nango injects valid credentials and handles upstream rate-limiting.
- **Client Connect UI:** Clients authorize their SaaS accounts via a pre-built, hosted OAuth modal without handling keys.
- **Distributed Token Refresh:** Nango centrally refreshes tokens before they expire using distributed locks to prevent rotation race conditions.

---

## 2. Multi-Tenancy & Topology

Turvez enforces strict client boundaries (`<client>`). Nango integrates seamlessly with this spine:

### Tenancy Mapping
- **`Connection-Id` = `<client>`** (e.g. `acme`, `initech`).
- For clients with multiple accounts of the same provider: `<client>:<subaccount>` (e.g. `acme:us-sales`, `acme:eu-sales`).
- **`Provider-Config-Key` = `<provider>`** (e.g. `hubspot`, `slack`, `google-sheets`, `salesforce`).

### Deployment Topology
Nango runs as a central service or per-cluster instance:

| Component | Shared Runtime (`runtime: shared`) | Dedicated Runtime (`runtime: dedicated`) |
|---|---|---|
| **Nango Instance** | Single self-hosted Nango (or Nango Cloud) shared by consultancy clients | Dedicated Nango container in client's VPC |
| **Data Isolation** | Partitioned strictly by `Connection-Id` (`<client>`) | Separate database and encryption key |
| **Secret in Turvez** | Shared `NANGO_SECRET_KEY` in Windmill/dagnats env | Client-specific `NANGO_SECRET_KEY` |

#### Self-Hosted Docker Compose (`docker-compose.nango.yml` in this repo)
The repo ships the real file; this is its shape (official image, `SERVER_PORT`
3003 + Connect UI 3009 inside the container, published on 8088/8089):

```yaml
services:
  nango:
    image: nangohq/nango-server:hosted
    ports:
      - "8088:3003"   # API + proxy
      - "8089:3009"   # Connect UI
    environment:
      - NANGO_ENCRYPTION_KEY=${NANGO_ENCRYPTION_KEY}   # 32 hex chars, no default
      - NANGO_DB_HOST=nango-db
      - NANGO_DB_USER=nango
      - NANGO_DB_PASSWORD=${NANGO_DB_PASSWORD}
      - NANGO_DB_NAME=nango
      - SERVER_PORT=3003
      - CONNECT_UI_PORT=3009
      - NANGO_SERVER_URL=${NANGO_SERVER_URL:-http://localhost:8088}
    depends_on:
      - nango-db
```

`./scripts/setup-nango.sh --up` validates the two required env vars, starts the
stack and waits for `/health`. A default encryption key is deliberately not
provided: it protects every stored OAuth token.

---

## 3. Using Nango in Windmill

Windmill is the default engine (`engine: windmill`). The call goes in a **Go
node** — script nodes are Go in this repo (`docs/language-policy.md`), and the
proxy is plain HTTP, so no Nango SDK is involved. The only non-Go file allowed is
a thin WAC orchestrator that dispatches to this node with `taskScript()`.

There are two ways to invoke SaaS APIs via Nango:

### Method A: The Nango Request Proxy (Recommended)
The proxy forwards requests to the SaaS endpoint, automatically attaching refreshed OAuth headers.

#### Go node (`language: go`):
```go
package inner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// main returns one page of HubSpot contacts routed through Nango.
// The parameter becomes the step's input schema and generated UI.
func main(client string) (interface{}, error) {
	host := os.Getenv("NANGO_HOST")       // workspace variable
	key := os.Getenv("NANGO_SECRET_KEY")  // workspace secret

	url := fmt.Sprintf("%s/proxy/crm/v3/objects/contacts?limit=10", host)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Provider-Config-Key", "hubspot")
	req.Header.Set("Connection-Id", client) // the Turvez client slug

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("nango proxy: %s", resp.Status)
	}
	var out any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
```

### Method B: Fetching Fresh Credentials Directly
If an external Go SDK requires the raw token (e.g. `google.golang.org/api`),
fetch it from Nango just-in-time. Never cache it and never write it anywhere:

```go
func nangoToken(provider, client string) (string, error) {
	host := os.Getenv("NANGO_HOST")
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/connection/%s?provider_config_key=%s", host, client, provider), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("NANGO_SECRET_KEY"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nango connection: %s", resp.Status)
	}
	var body struct {
		Credentials struct {
			AccessToken string `json:"access_token"`
		} `json:"credentials"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Credentials.AccessToken, nil
}
```

---

## 4. Using Nango in Dagnats

In the dagnats engine (`engine: dagnats`), recipes are written in declarative JSON using the 12 generic handlers.

Because Nango provides an HTTP Request Proxy, **no new Go handler is needed**. The standard `http.request` task routes directly through Nango. The URL must be reachable **from the worker**, so it is the host address of the Nango instance, not a compose service name from a different project:

```json
{
  "id": "fetch_hubspot_contacts",
  "task": "http.request",
  "input": {
    "url": "http://localhost:8088/proxy/crm/v3/objects/contacts?limit=10",
    "method": "GET",
    "auth_secret": "NANGO_SECRET_KEY",
    "headers": {
      "Provider-Config-Key": "hubspot",
      "Connection-Id": "acme"
    }
  }
}
```

When the runner and Nango are containers, put them on one compose network and
use `http://nango:3003`, or reach the host (`host.docker.internal:8088` on Docker
Desktop, the Tailscale IP on a server).

### How dagnats executes this:
1. `auth_secret: "NANGO_SECRET_KEY"` resolves from the client's `secrets/<client>/env.age` or worker environment.
2. The worker sends the HTTP request to the Nango container.
3. Nango inspects `Connection-Id: acme` and `Provider-Config-Key: hubspot`, retrieves/refreshes the OAuth token for `acme`, injects `Authorization: Bearer <upstream_token>`, and calls HubSpot.
4. The output returned to dagnats is the raw HubSpot response payload.

---

## 4.1. Using Nango in Dagu

In the Dagu engine (`engine: dagu`), recurrent script-shaped jobs run as DAGs (`automation/dagu/*.yaml`). Instead of hardcoding tokens or maintaining token refresh scripts:

```yaml
steps:
  - name: sync_hubspot
    command: >
      curl -s -f -H "Authorization: Bearer $NANGO_SECRET_KEY"
      -H "Provider-Config-Key: hubspot"
      -H "Connection-Id: acme"
      "$NANGO_HOST/proxy/crm/v3/objects/contacts?limit=50"
      -o /tmp/contacts.json
```

---

## 5. Division of Responsibilities: What Goes Where

| Secret / Config | Storage | Why |
|---|---|---|
| **SaaS OAuth credentials** (HubSpot, Slack, Google, Salesforce) | **Nango** | Token refresh rotation, OAuth callback dance, rate-limits |
| **SaaS provider API keys** (a HubSpot private-app token, a Stripe secret key, a client's own API bearer token) | **Nango**, as an API-key connection | One encrypted store, one place to rotate, `Connection-Id` per tenant. The runner never holds the value. |
| **Nango Secret Key** (`NANGO_SECRET_KEY`) | **Windmill Variable / `secrets/<client>/env.age`** | Platform-level secret used by workers to authenticate with Nango |
| **Webhook signing secrets** (e.g. `SLACK_SIGNING_SECRET`) | **Windmill Variable / `secrets/<client>/env.age`** | Static secret required locally to verify inbound payload HMAC signatures |
| **Incoming webhook URLs** (`SLACK_WEBHOOK_URL`) | **Windmill Variable / `secrets/<client>/env.age`** | A fixed endpoint we own, not a revocable grant; nothing to refresh |
| **Internal databases / private APIs** | **Windmill Variable / `secrets/<client>/env.age`** | Private infrastructure not exposed as public SaaS |

The test is one question: *can the client revoke or rotate it in a vendor
dashboard?* If yes it belongs in Nango — we should not be paged for their
rotation. If it is our own plumbing (webhook URL, DB password, the Nango key
itself), it stays in the store the runner reads.

### 5.1 Naming: one integration per provider, one connection per client

| Thing | Value | Example |
|---|---|---|
| Integration key (`Provider-Config-Key`) | provider slug, lowercase, **no client prefix** | `hubspot`, `google-calendar`, `slack` |
| Connection id (`Connection-Id`) | client slug, `:<subaccount>` when it has several accounts | `acme`, `acme:billing` |

Two integrations for one provider (`acme-hubspot`, `beta-hubspot`) means a scope
change or a renewal is applied N times and gets missed N-1 times. The client is
the **connection**, never the integration.

### 5.2 Enforcement (not convention)

`clients/credentials_test.go` fails the build on all three ways a copy-pasted
integration leaks across tenants:

1. a provider credential in `env/*.env.example` (`ACME_HUBSPOT_TOKEN`) — the
   message names the provider and the `nango-connect.sh` command that replaces it;
2. a `Connection-Id` that is not the client the file lives under;
3. a `Provider-Config-Key` with a client prefix, or not a lowercase slug.

`./scripts/nango-connect.sh <client> --provider <slug> [--open]` prints the
dashboard link, the two names to type, the verification `curl`, and the exact
header block to paste — so the agent asks for an authorisation instead of a token.

---

## 6. Runbook: Onboarding a New Client Integration

When a new client (`acme`) requires integration with a SaaS platform (e.g. HubSpot):

### Step 1: Configure the Integration in Nango
1. In the Nango UI (or `nango.yaml`), enable the provider (`hubspot`).
2. Enter your master OAuth Client ID and Client Secret (or let the client provide their developer app keys if required by enterprise policy).
3. Set the scopes needed by the automation (e.g. `crm.objects.contacts.read`).

### Step 2: Authorize the Client Connection
The agent prints the identifiers; the operator clicks. `./scripts/nango-connect.sh acme --provider hubspot --open` gives the dashboard link, the connection id to use (`acme`) and the verification command — never ask for the token itself.
Send the client a secure Connect link generated via Nango's API or UI:
```bash
curl -X POST "$NANGO_HOST/connect/sessions" \
  -H "Authorization: Bearer $NANGO_SECRET_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "connection_id": "acme",
    "provider_config_key": "hubspot"
  }'
```
The client visits the URL, completes the OAuth consent screen with their provider, and Nango marks connection `acme` as `active`.

### Step 3: Verify the Connection via CLI
```bash
curl -s -H "Authorization: Bearer $NANGO_SECRET_KEY" \
  "$NANGO_HOST/connection/acme?provider_config_key=hubspot" | jq .
```
Verify that `credentials.access_token` and `status: "SUCCESS"` are present.

### Step 4: Configure the Turvez Client
In `env/acme.env.example`:
```bash
# Nango is used for SaaS integrations (HubSpot).
# No raw HUBSPOT_TOKEN is needed!
NANGO_HOST=http://localhost:8088
NANGO_SECRET_KEY=
```

### Step 5: Test the Proxy Call
Execute a test probe using the Nango proxy:
```bash
curl -i -H "Authorization: Bearer $NANGO_SECRET_KEY" \
  -H "Provider-Config-Key: hubspot" \
  -H "Connection-Id: acme" \
  "$NANGO_HOST/proxy/crm/v3/objects/contacts?limit=1"
```
If this returns `200 OK`, write the Windmill flow or dagnats recipe referencing `Connection-Id: acme`.

---

## 7. Reliability and Failure Handling

- **Proxy Down / Network Partitions:** The `on_failure` step in Windmill flows or dagnats recipes alerts the operator if Nango is unreachable.
- **Revoked Connections:** If a client revokes authorization in their SaaS settings, Nango returns HTTP 401 with error code `invalid_token`. Workflows catch this and route to a notification step asking the client to re-authenticate via the Connect UI.
- **DLQ & Replay:** Because requests go through the proxy with stable connection keys, replaying a failed run from the DLQ automatically uses the refreshed credentials without manual intervention.

---

## 8. Scaling Strategy: Centralized (Shared) vs. Multi-Instance (Dedicated)

Turvez scales across two primary dimensions:
1. **Engine scale:** Expanding recipes and flows across clients using Windmill, dagnats, or Dagu.
2. **Infrastructure scale:** Evolving from a single centralized consultancy stack (`runtime: shared`) to dedicated client instances (`runtime: dedicated`).

Here is how Nango scales coherently with that architecture:

### 8.1 The Two-Tier Topology

```
                  ┌────────────────────────────────────────────────────────┐
                  │                 TIER 1: SHARED (SMBs)                  │
                  │   Consultancy Server (runtime: shared)                 │
                  │                                                        │
                  │   ┌───────────────┐     ┌───────────────────────────┐  │
                  │   │ Windmill /    │     │   Nango Central (:8088)   │  │
                  │   │ Dagnats /     │────▶│                           │  │
                  │   │ Dagu          │     │  • acme:hubspot           │  │
                  │   └───────────────┘     │  • beta:slack             │  │
                  │                         │  • gamma:google-sheets    │  │
                  │                         └─────────────┬─────────────┘  │
                  └───────────────────────────────────────┼────────────────┘
                                                          │ (Proxy calls with
                                                          │  auto-refreshed OAuth)
                                                          ▼
                                                   [External SaaS]
                                                          ▲
                  ┌───────────────────────────────────────┼────────────────┐
                  │                                       │                │
                  │                TIER 2: DEDICATED (Enterprise)          │
                  │   Client VPC / Isolated Server (runtime: dedicated)    │
                  │                                                        │
                  │   ┌───────────────┐     ┌───────────────────────────┐  │
                  │   │ Windmill /    │     │   Nango Dedicated (:8088) │  │
                  │   │ Dagnats       │────▶│                           │  │
                  │   │ Dedicated     │     │  • enterprise:salesforce  │  │
                  │   └───────────────┘     │  • enterprise:quickbooks  │  │
                  │                         └───────────────────────────┘  │
                  └────────────────────────────────────────────────────────┘
```

| Dimension | Tier 1: Shared Centralized (`runtime: shared`) | Tier 2: Dedicated Isolated (`runtime: dedicated`) |
|---|---|---|
| **Target Client** | SMBs, startups, standard consulting engagements (Profile B) | Enterprise, financial, healthcare (SOC2, HIPAA, GDPR, Profile A) |
| **Nango Deployment** | Single instance on consultancy server (`docker-compose.nango.yml`) | Dedicated Nango container in client's VPS/VPC |
| **Tenancy Boundary** | Partitioned strictly by `Connection-Id: <client>` | Complete physical isolation (dedicated DB + container) |
| **Master OAuth Apps** | Consultancy creates 1 OAuth App in HubSpot/Slack/Google; all clients authorize through it | Client registers their own Developer App in their SaaS organization |
| **Encryption Key** | Consultancy holds `NANGO_ENCRYPTION_KEY` | Client / dedicated host holds independent encryption key |
| **Operating Cost** | Near zero (1 container, 1 DB, 1 backup) | Higher ($N$ containers and Postgres DBs to monitor) |

---

### 8.2 Best Configurations Nango Offers for Turvez

1. **Master OAuth App for SMBs (Frictionless Onboarding):**
   - In Tier 1, register ONE master OAuth App in each provider (e.g. "Turvez Consulting Integrations").
   - Clients do **not** need developer accounts or GCP/HubSpot console access. You send a Connect session link, they click "Authorize", and Nango stores the token under `Connection-Id: <client>`.
2. **Per-Client OAuth App Override:**
   - If an SMB client wants their brand on the consent screen, Nango allows overriding the OAuth App credentials per integration or environment without needing a new Nango instance.
3. **Webhook Forwarding:**
   - Nango receives inbound webhooks from HubSpot/Slack/Stripe and forwards them with `connectionId` attached to the Turvez trigger endpoint (`/hooks/<client>/...` in dagnats or `/api/w/<client>/webhooks/...` in Windmill).
4. **Proxy Caching & Rate-Limit Backoff:**
   - Nango centrally serializes token refresh requests. When 10 parallel Windmill flow steps run simultaneously, Nango refreshes the token **once** and serves all 10 requests without triggering provider rate limits or refresh token race conditions.

---

### 8.3 Migration Lifecycle: Moving a Client from Shared to Dedicated

Because Turvez recipes and scripts use environment variables and the client slug:
- In a Go node or dagnats input: `os.Getenv("NANGO_HOST") + "/proxy/..."`
- Header: `Connection-Id: <client>`

**Zero workflow code changes are required** when moving a client to a dedicated host! Only environment configuration moves.

#### The 4-Step Extraction Procedure:
1. **Provision Dedicated Nango:** On the client's new server, run:
   ```bash
   ./scripts/setup-nango.sh --up
   ```
2. **Authorize Dedicated Connections:** Generate Connect sessions on the new Nango instance for `connection_id = <client>` (using client's own OAuth Developer App if required).
3. **Point Automation to New Nango:** In the new server's environment or Windmill workspace:
   ```bash
   export NANGO_HOST="http://<client-dedicated-nango>:8088"
   export NANGO_SECRET_KEY="<new-dedicated-secret-key>"
   ```
4. **Purge Shared Connection (Old Server):** Remove the connection from the shared Nango to prevent lingering tokens:
   ```bash
   curl -X DELETE -H "Authorization: Bearer $SHARED_NANGO_SECRET_KEY" \
     "$SHARED_NANGO_HOST/connection/<client>?provider_config_key=<provider>"
   ```
   Now the client is 100% isolated, and [`scripts/verify-isolation.sh`](file:///Users/cali/Development/dagnats-automation/scripts/verify-isolation.sh) verifies that the client is coherent.

