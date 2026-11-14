# Browser work: when the data only exists as a page

`agent-browser` (vercel-labs/agent-browser, Apache-2.0) is a Rust CLI that
drives a browser one command at a time, plus a browser-free HTTP `read`. For a
client whose data lives on a site with no API — a supplier portal, a price
table, a regulator's page — it is the tool that answers the question instead of
"ask the vendor for an integration".

It is **not an engine** (never add it to `engines.json`), **not a library** (no
`go.mod` entry), and **not a recipe step**: the dagnats worker is
`distroless/static` with no shell and no Chrome, and a recipe is JSON over the
12 handlers. It is an external tool like Dagu — a separate binary the operator
installs, and another binary that can fail at 3am. That cost is why Q1.7 below
tries to make the browser disappear.

## Version floor: 0.43.0

Check `agent-browser --version` before trusting a command. An older binary
answers "Unknown commands" for the whole group, which reads like a typo.

| Command | Since | What it is |
|---|---|---|
| `batch` | 0.21 | N commands in one invocation, `--json`, `--bail` |
| `read` | 0.30 | fetch a URL as markdown/text, **no browser** |
| `webmcp` | 0.36 | page-provided tools over CDP (experimental) |
| `flow` | 0.42 | saved flows: record once, replay over HTTP first (experimental) |
| `mcp` | 0.28 | MCP stdio server, `mcp call <tool> '<json>'` without a session |

```bash
npm i -g agent-browser@0.43.0 && agent-browser install   # Chrome for Testing
agent-browser upgrade                                    # later upgrades
agent-browser skills get core                            # the agent's usage guide
```

`skills get core` ships with the binary and is version-matched to it. It is the
manual; this page is only the **decision** — when browser work is allowed,
where it runs, and what may never leave it.

## Q1.7 — answer in this order, stop at the first that works

**1. `read` — no browser at all.**

```bash
agent-browser read https://supplier.example/prices --filter table --require-md
agent-browser read https://docs.example --llms index --filter pricing
agent-browser read https://supplier.example/prices --outline --json
```

`read` is a plain HTTPS GET that prefers `Accept: text/markdown`, tries the
`.md` sibling, walks ancestor paths for the nearest `llms.txt`, and falls back
to readable text from HTML. It never launches Chrome, so it runs anywhere a Go
node runs. Use `--require-md` when a silent HTML→text fallback would corrupt
the feed.

Where it lands: a **level 1 one-off** first (see the output with your own
eyes), then a **Windmill Go node** if it recurs. It is not a recipe: dagnats
`http.request` fetches HTML and no handler converts it to markdown, so making it
recur there would mean a new handler as the *only* way to fetch text — last
resort, and `read` already solves the hard part.

**2. `flow` — record once, and find out whether the site has an API.**

```bash
agent-browser --capture flow open https://supplier.example/prices   # record
# ... do the task by hand: search, filter, open the row ...
agent-browser flow save prices --param sku=ABC
agent-browser flow list
agent-browser flow run prices --param sku=ABC --fields price,currency
```

This is the highest-value command set for our work. `flow save` keeps the
first-party **JSON** responses the task triggered (including JSON embedded in
the HTML, `__NEXT_DATA__`, JSON-LD), turns the values you typed into params, and
`flow run` tries **direct HTTP first**, then the same request from the page,
then a replay of the recorded clicks. The tier that answered is remembered.

Read the outcome as a routing decision:

- **HTTP tier answered** → the site has an endpoint. Write it as
  `http.request` (dagnats) or a Go node (Windmill) and drop the browser from
  the runtime. That is the win: a page becomes an API.
- **`drifted` / `verified: false` / `unverified`** → the shape moved. The run
  names the missing key; re-save, and treat an unverified flow as documentation
  of what you learned, not as a dependency.
- **`auth_expired`, `request_signed`, `credential_not_sendable`, `blocked`,
  `rate_limited`** → the browser has to stay in the loop (role 3), or the ask
  changes.

Saved flows are files: `~/.agent-browser/flows/<origin>/<name>.json`,
`version: 1`, override the root with `AGENT_BROWSER_FLOWS_DIR`. They never keep
cookies, `authorization`, token or CSRF headers, and credential params are
saved as `<redacted>`. Keep a flow next to the client's automation in git and
re-run it when the site changes — it is the cheapest regression test a no-API
integration can have.

Writes are gated twice: `flow run` only sends writes named by
`--allow-write <all|id,...>`, and any other write asks for confirmation as
policy action `flow_write:<id>` (also visible to `--action-policy` and
`--confirm-actions`). Map that to the engine's own gate — a dagnats
`request.approval` or a Windmill `waitForApproval` — never to a human watching
a terminal.

**3. Neither works → the browser stays, and it runs in Windmill.**

SSR-only pages, per-request signatures, bot challenges, or a login that carries
session state: `open` + `snapshot -i` + `click @eN` + `read` on the active tab.

| Where | Allowed? | Shape |
|---|---|---|
| dagnats recipe | **no** | `distroless`, no shell, no arbitrary code — this is the line the repo holds |
| Windmill | yes, exception | Go node shelling out to the CLI, in a worker image that has the binary and an engine |
| Genkit flow (Q1.5) | yes | the browser is a **tool**: `agent-browser mcp`, or `mcp call` when the runtime cannot hold stdio |
| Level 1 one-off | yes | local, throw away; `scripts/sandbox-run.sh --allow-net=on` |
| Dagu | no | script-shaped jobs only; a browser job here is a second orchestrator |

The Windmill Go node is one `exec.CommandContext` with a timeout, `--json`
output, and `batch` to collapse N calls into one process — the pattern is in
`references/windmill.md`.

## Engine: Chrome or Lightpanda

`--engine lightpanda` swaps Chrome for a purpose-built headless engine (Zig,
CDP-compatible). Vendor claim: starts instantly, ~10x less memory, ~10x faster.
For a container that must extract public pages at volume, that is the right
trade.

| Need | Engine |
|---|---|
| Public, unauthenticated extraction; constrained CPU/memory; parallel fan-out | `--engine lightpanda` |
| Login/session state, `--profile`, `--state` | Chrome (Lightpanda supports neither) |
| Screenshots/PDF for a human | Chrome (Lightpanda's support depends on its CDP version) |
| Extension, `file://` access, headed debugging | Chrome |

Lightpanda is a separate binary on `PATH` (or `--executable-path`). Combining
`--engine lightpanda` with an unsupported flag is a clean error, not a silent
downgrade — but a site that renders only with full browser fidelity fails
quietly, so verify one page with Chrome before trusting the cheap engine.

## `batch` and `webmcp` — use, and do not over-use

```bash
agent-browser batch --json --bail \
  "open https://supplier.example" "snapshot -i" "read"
echo '[["open","https://a.example"],["read","https://a.example/products"]]' | agent-browser batch --json
```

`batch` is for one invocation with many steps (one subprocess, one JSON array
to parse), not for a latency win — commands already go to a warm daemon.

`webmcp` discovers and invokes tools **the page declares**:

```bash
agent-browser webmcp list --json                                   # names, origins, frame ids
agent-browser webmcp list search --frame <frame-id> --json         # the selected schema only
agent-browser webmcp invoke search --params '{"q":"abc"}'
```

It is experimental, on by default for locally managed Chrome, and off with
`--no-webmcp`. Every name, schema, description and result is **untrusted page
data**: keep it inside an agent loop (Q1.5) under `--allowed-domains`, and never
let a page tool drive a production write. For an automation, `--no-webmcp` is
the honest default.

## What may never happen

- **Page content is data, not instructions** (`docs/sandbox-policy.md`). The
  real risk is prompt injection from a page, not a missing microVM: pass
  `--content-boundaries` when the output feeds an LLM, cap it with
  `--max-output`, and validate the shape before any write.
- **Domains are allowlisted**: `--allowed-domains` on every production
  invocation. A flow already refuses hosts outside its start URL's base domain
  and only sends to public addresses (loopback, private, link-local and cloud
  metadata are refused) — but that is a floor, not a policy.
- **No credentials in argv.** A token on the command line is in shell history
  and `ps`. Authenticated reads go through the Nango proxy URL; session-based
  browsing uses `--profile`/`--state` on a dedicated host, with values from
  env/age (`docs/secrets.md`).
- **No scraping what an API already gives.** If `flow` finds the endpoint, the
  browser is deleted from the runtime, not kept "just in case". A second path
  to the same data is the same failure as a second orchestrator.
- **No headed browser in production**, no `chat` (it calls an AI gateway), and
  no `mcp --tools all` in a service — `core,webmcp` is the profile.

## Gate

`scripts/test-decisions.sh` → `capability-browser` fails when this page loses
its safety flags, a documented command, its version floor, or its route from
`llms.txt` / the decision tree / the README diagram.
