# Nango Integration Reference for Turvez Agents

When an automation requires talking to third-party SaaS APIs (HubSpot, Slack, Google, Salesforce, Notion, Jira, QuickBooks, Stripe, etc.), **use Nango**. Do not put raw OAuth tokens, refresh tokens, or client secrets into Windmill variables, `.env` files, or dagnats recipes.

Full architecture guide: `docs/nango.md`.

---

## 1. Golden Rules for Agents

1. **Never store raw OAuth tokens.** OAuth access tokens expire; refresh tokens rotate. Writing them to `env/<client>.env` guarantees production outages.
2. **Never implement token refresh logic in scripts.** Token rotation requires distributed locking. Concurrent executions will race and invalidate the connection.
3. **Use the Nango Request Proxy by default.** Route HTTP calls through `${NANGO_HOST}/proxy/<path>` with `Connection-Id: <client>` and `Provider-Config-Key: <provider>`.
4. **Tenancy:** The `Connection-Id` must always be the client name (`<client>`). If a client has multiple accounts, use `<client>:<subaccount>`.
5. **Only one secret in the runner:** The only secret Windmill or dagnats needs is `NANGO_SECRET_KEY`.

---

## 2. Windmill Patterns (Go nodes)

Windmill workspace should declare:
- Variable: `NANGO_HOST` (e.g. `http://localhost:8088` locally, `https://api.nango.dev` on Cloud)
- Secret Variable / Resource: `NANGO_SECRET_KEY`

Script nodes are Go (`docs/language-policy.md`). The proxy is plain HTTP, so no
Nango SDK is needed — and the TS/Python SDKs are not an option for a node.

### Pattern A: Go node through the Nango proxy

```go
package inner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// main fetches contacts through Nango. The step's input schema comes from this
// signature; the secret is read in the runner, never passed as an argument.
func main(client string) (interface{}, error) {
	host := os.Getenv("NANGO_HOST")
	key := os.Getenv("NANGO_SECRET_KEY")

	url := fmt.Sprintf("%s/proxy/crm/v3/objects/contacts?limit=20", host)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Provider-Config-Key", "hubspot")
	req.Header.Set("Connection-Id", client)

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

Use an `http.Client` with a timeout in production:
`&http.Client{Timeout: 30 * time.Second}`.

### Pattern B: fetching a raw token for a Go SDK client

Only when a Go library insists on a token (e.g. `google.golang.org/api`). Fetch it
per run; never persist it.

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

### Pattern C: a WAC orchestrator that calls the Go node

If the flow needs a loop or a fan-out that `flow.yaml` expresses badly, the thin
TypeScript/Python graph may dispatch to the Go node by path. It must not call the
Nango API itself:

```typescript
import { taskScript, workflow } from "windmill-client";

const fetchContacts = taskScript("f/acme/nango_fetch_contacts");

export const main = workflow(async (client: string) => {
  return await fetchContacts({ client });
});
```


---

## 3. Dagnats Pattern

Use the built-in `http.request` task. No custom Go handler is required.

```json
{
  "id": "get_contacts",
  "task": "http.request",
  "input": {
    "url": "http://localhost:8088/proxy/crm/v3/objects/contacts",
    "method": "GET",
    "auth_secret": "NANGO_SECRET_KEY",
    "headers": {
      "Provider-Config-Key": "hubspot",
      "Connection-Id": "TODO-CLIENT"
    }
  }
}
```

---

## 4. Dagu Pattern

For level 2 recurrent jobs in Dagu (`automation/dagu/*.yaml`):

```yaml
steps:
  - name: sync_hubspot
    command: >
      curl -s -f -H "Authorization: Bearer $NANGO_SECRET_KEY"
      -H "Provider-Config-Key: hubspot"
      -H "Connection-Id: TODO-CLIENT"
      "$NANGO_HOST/proxy/crm/v3/objects/contacts?limit=50"
      -o /tmp/contacts.json
```

---

## 5. Checking & Bootstrapping Nango

Before authoring flows that use Nango, check if Nango is running:

```bash
./scripts/setup-nango.sh            # or curl -s -f "$NANGO_HOST/health"
```

If not running, spin up the stack:
```bash
./scripts/setup-nango.sh --up       # runs docker compose -f docker-compose.nango.yml up -d
```
Default port: `8088` (API + proxy at `http://localhost:8088`; Connect UI on 8089), non-conflicting with Windmill (:8000) or dagnats (:8080). Inside a compose network the service is `http://nango:3003`.

---

## 6. Error Handling

When calling the Nango proxy:
- **HTTP 401 with `invalid_token`**: Upstream authorization was revoked by the user or expired permanently. Flow should route to notification (`notify.slack`) informing the operator that the client needs to re-authorize via Nango Connect UI.
- **HTTP 429**: Rate limit hit upstream. Nango automatically retries standard rate limits, but if forwarded, retry with exponential backoff.
- **HTTP 502 / 503**: Nango cannot reach the upstream service or Nango service is restarting. Step retries handle this automatically.

---

## 7. Scaling Strategy: Shared vs Dedicated Nango

1. **Shared Consultancy Nango (Default, `runtime: shared`):**
   - One central Nango instance on your server (`:8088`).
   - Register ONE master OAuth Developer App in HubSpot/Slack/Google for the consultancy; all SMB clients authorize through it.
   - Tenancy: `Connection-Id: <client>`.
   - Security: AES-256 encrypted at rest; clients never have direct access to `NANGO_SECRET_KEY` or the Nango API.
2. **Dedicated Client Nango (`runtime: dedicated`):**
   - For enterprise clients with data residency or compliance requirements.
   - Run dedicated `./scripts/setup-nango.sh --up` on the client's host.
   - Zero workflow code changes! Workflows only read `NANGO_HOST` and `Connection-Id: <client>`.
   - When extracting a client to its own server: `./scripts/extract-client.py <client>` generates the exact cutover and old-server connection cleanup commands.
