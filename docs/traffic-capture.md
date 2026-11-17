# Traffic capture: finding the endpoint, and where the deep path stops

Q1.7 tries to make the browser disappear. This page is the **detour** for when
it will not: the app has no API, the page renders server-side, and records come
from a phone. Two questions come up every time, and they have different answers:

1. **"There is an endpoint, I just cannot see it"** → capture the traffic. This
   is `agent-browser flow`'s job, already in `docs/browser.md`. mitmproxy is the
   faster probe when the target is a **mobile app or a SPA whose bundle you do
   not control**.
2. **"There is no endpoint — it is in the binary"** → this page says **no**, and
   why, and what to do instead. That is the REA / Frida / Ghidra / binwalk road.

The short version:

```
capture traffic  →  the endpoint is there?  ── yes ──→  http.request / Go node · record gone
(mitmproxy / flow)                            │
                                              no → signed? session? challenge?
                                                    │
                                                    ├─ the ask changes   →  stop, re-ask Q1.7
                                                    └─ the browser stays →  Windmill Go node
no traffic to capture, logic lives in the client binary  →  NOT an automation (below)
```

## 1. mitmproxy as a discovery probe, not a mechanism

`agent-browser flow` records by driving a browser it installed. That is the
right tool for a site. It is the wrong tool for a **mobile app**, a SPA behind a
login you cannot script, or any target where the client is not yours to launch —
and for the specific question *"does this app talk JSON to a first-party
host?"*, ten minutes of capture answers it before you invest in selectors.

```bash
mitmdump -w ~/capture.flows --set confdir=~/.mitmproxy   # a scratch dir, never prod
# do the task by hand in the real app once
mitmdump -nr ~/capture.flows -w ~/capture.har            # convert to HAR
```

Read the HAR with the same routing decision `flow` gives you — it is the same
question, so it is the same answer:

| What the HAR shows | Decision |
|---|---|
| A JSON request to a first-party host, with a bearer token you can obtain | It is an API. Write `http.request` and **delete the capture**. |
| A JSON request whose signature/header is computed in the client | `request_signed`. Not `http.request` — see §3. |
| Only HTML/GraphQL-RPC/first-party XHR that is not replayable out of session | The browser stays (Q1.7 role 3). |
| A third party's SaaS, not the client's own system | Not ours to automate. Stop; this is a licence/ToS question for the client, not a technical one. |

Two facts decide most cases before any command runs:

- **A captured request is evidence, not a dependency.** It tells you an endpoint
  exists; it never becomes the thing production calls. The `http.request` step
  is written from the captured **shape** (URL, method, headers by name, body
  keys) with the secret as a name (`docs/secrets.md`), never from the capture
  itself.
- **The browser's capture tiers are safer than a proxy.** `flow` keeps the
  first-party JSON and drops cookies, `authorization`, token and CSRF headers.
  A `.har` from mitmproxy is the opposite: a plaintext file of a real session,
  with real credentials, on disk. Treat it as a secret (`chmod 600`, under
  `~/.secrets/`, never in git), and delete it after the shape is written down.

**When this probe is worth running** (the "1.5" step in Q1.7):

- the target is an app with no web UI worth scripting;
- the page needs a login whose flow you cannot reliably re-save;
- someone claims "there is no API" and you want the cheapest possible answer;
- the client already hands you a device/session — capture is then free.

Not worth it when `agent-browser read` already answers (no browser, no proxy,
no certificate), or when `flow` records fine. Installing a CA the first time
costs more than clicking through the page.

## 2. The deep path: REA, Frida, Ghidra, binwalk — **we do not do this**

There is a real toolchain here and it is genuinely good. Naming it exactly, so
nobody rediscovers it and thinks it is new:

| Tool | Licence | What it actually does |
|---|---|---|
| **REA** (`reamcp.dev`, `npx rea-agents`) | MIT | MCP server that gives an agent: saved network captures (HAR/SAZ, source locations), runtime observation, JavaScript/Electron analysis, APK + adb, firmware extraction handoff (binwalk/unblob), and hooks to native analysis engines |
| **mitmproxy** | MIT | proxy that records plaintext traffic; `mitmdump -w` writes a flow file, converted to HAR |
| **Frida** | wxWindows/LGPL | injects into a running process and hooks functions — reads the value *before* it is encrypted or signed |
| **Ghidra / Hopper / IDA** | Apache-2.0 / commercial | static analysis: decompile the client, read the routine that builds the request |
| **binwalk / unblob** | MIT | unpack a firmware image, then hand the result to the decompiler |

The pipeline the posts describe — `capture → map request to source → Frida the
hook → Ghidra the routine → take firmware apart` — works. The question is not
whether it works. It is whether the output has a home in **this** repo.

### Why it does not belong here

| The pipeline's premise | What turvez actually is |
|---|---|
| "Generate code that replicates the endpoint" | There is exactly one way to call an API: `http.request` or a Windmill Go node. Generated client code has **nowhere to live**, and `docs/language-policy.md` is why |
| "The payload is signed by the app" | `http.request` sends static headers plus one named secret. No per-request HMAC, no cookie jar, no token refresh. The capability does not exist — and this is a decision, not a gap |
| "Investigate until it works" | The dagnats worker is `distroless/static`: no shell, no Python, no Frida, no Chrome. The investigation cannot run where the automation runs |
| "A new provider/handler when you need one" | A handler is `internal/handlers/`, typed I/O, table tests, human review, **last resort**. Deep RE would become the justification per client |
| "The app is the interface" | The app belongs to a third party. Their ToS, their access controls, their rate limits. `DISCLAIMER.md` in REA itself refuses auth bypass, technical protection measures and unauthorised targets |

The failure mode is concrete: an interception CA and a hooked process in the
path of a client's real data, kept alive by a mapping from endpoint to
decompiled source that dies at the next app release. That is a server to
monitor at 3am, with a legal question attached, in exchange for a job that a
`flow` re-save would have fixed for free.

**This is not "those tools are bad".** It is: *turvez automates what has a
stable contract; it does not manufacture one by attacking the client.*

### What to do instead

| The situation | The answer |
|---|---|
| The endpoint exists, hidden behind the page | mitmproxy probe (§1) or `flow`, then `http.request` |
| The endpoint exists but signs every request | The browser **is** the mechanism. It stays as a Windmill Go node (Q1.7 role 3). Do not fight the signature |
| The logic lives in a phone app with no replayable HTTP | **Not an automation.** Say so in the fieldbook, name the reason, and propose the alternative the client controls: an official integration, an export, a data-sharing agreement — or a browser flow against their *web* client if one exists |
| The client owns the app and wants it driven | That is not a no-API integration, it is a client-side test harness. Different work, different repo |
| Firmware, a device, a proprietary protocol | Out of scope by construction. `docs/sandbox-policy.md` level 1 at most, and it is research, not a client automation |

The honest sentence to a client is: *"we can automate this while the interface
stays as it is today; the moment the vendor changes it, this is a re-record, not
a rebuild — and if they sign their requests, the browser stays in the loop."*

## 3. The refusal, in the shape of the engine

Q1.7 role 3 is where a signed or challenged target lands, and the doc says it
plainly (`docs/browser.md`, "What may never happen"). What the capture adds is
the **diagnosis** that puts you there. `flow run` already returns these codes —
read them as the end of the road, not as a prompt to go deeper:

| Code | Means | Goes to |
|---|---|---|
| `request_signed` | a header is computed in the client | browser stays; do not emulate the signature |
| `auth_expired` | the session ended | the session owner re-authorises; then re-run |
| `credential_not_sendable` | the header cannot be replayed out of session | browser stays |
| `blocked` / `rate_limited` | bot challenge or quota | the ask changes; do not evade |
| `drifted` / `verified: false` | the shape moved | re-save; treat as documentation of what you learned |

None of these codes is a bug to fix with a deeper tool. They are the router
telling you which of the three Q1.7 answers applies.

## Gate

`scripts/test-decisions.sh` → `capability-traffic` fails when this page loses
its refusals, its REA/mitmproxy §2 table, the "not an automation" row, or its
route from `llms.txt` / Q1.7 / the README diagram. `docs/browser.md` stays the
contract for `agent-browser`; this page is only reachable *through* it.
