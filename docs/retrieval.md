# Retrieval: answering from documents and from the client's own tools

Status: normative decision guide. It adds **no runtime dependency** to this
repo: retrieval is a per-client capability built as Go nodes (Windmill) or as a
service a dagnats recipe calls.

The question this answers is usually phrased as "can it answer questions about
our contracts / our ERP / our HubSpot?". That is not one feature. Split it
first.

| What holds the answer | Build | Do not build |
|---|---|---|
| A **system of record** (CRM, ERP, ticketing, wiki with an API) | a **tool call** through Nango: the fixed HTTP proxy in a recipe/flow, or an agent session (MCP) when an agent must choose tools | a local index of a copy that goes stale |
| **Documents the client owns** (PDFs, contracts, manuals) | **route-then-read** (below) | embeddings by default |
| A handful of pages that fit one prompt | no index: long context with a stable cached prefix | Bleve, a vector DB, a reindex pipeline |

The two expensive mistakes are a vector database for a 40-page FAQ, and a local
index of HubSpot that answers with last month's deal stage.

## Route, then read (the default shape)

The default is **not** "embed everything, fetch five chunks, stuff them in the
prompt". It is three stages:

```
[question]
   │
   ▼
1. LOCAL EXPANSION        glossary → OpenWN-PT → FastText (only behind an eval)
   │                       ms, $0, auditable
   ▼
2. BLEVE ROUTING          retrieve WHOLE documents / chapters, not chunks
   │                       the index is a router, not the answer
   ▼
3. LONG-CONTEXT READING   send the selected units whole, in order, within a
                          token budget; stable prefix for prompt caching
```

Why the unit changes:

- **Chunking is a workaround for a small reader window.** When the model can
  read a chapter, the chapter is the right unit. No answer split across chunk
  boundaries, no five fragments of the same page, and citations point at a
  section a human can open.
- **Lexical search only has to be right at document granularity.** Bleve's job
  is to get the reader into the right document/chapter; the model does the
  fine-grained reading.
- **It removes a layer.** No embedding model, no vector database, no chunk-level
  reranker by default.

This is still RAG. What changes is the **retrieval unit** and the **reader**.
Call it **route-then-read**; whether it replaces a vector database depends on
the corpus, not on the name.

## Verdict: does it replace vector databases and RAG?

**Yes for the common consultancy case; no as a universal replacement.**

| Case | Route-then-read? | Why |
|---|---|---|
| Bounded corpus (hundreds to a few thousand documents), terminology-rich (contracts, policies, manuals, ERP/legal/health) | **Replace chunked vector RAG; drop the separate vector DB** | exact terms, codes and clause numbers are the strongest signal; whole-unit reading beats chunk stitching; $0 for retrieval |
| Answer requires synthesising across many documents that do not fit one context | **Partial** — hierarchical/map-reduce, or vectors | the reader cannot see everything at once |
| Open-domain, very large, paraphrase-heavy, cross-lingual | **No** — keep semantic retrieval | lexical routing cannot recover a document whose vocabulary never matches the query |
| High QPS, tight latency/cost budget | **Usually no** | reading 30-60k tokens per query is slower and costlier than 5k of chunks, even with caching |
| You still want embeddings later | **Yes, without a new service** | Bleve has vector fields and kNN; add vectors to the same index when the eval demands it |

The honest limit: **route-then-read cannot recover a document that stage 1-2
never retrieved.** Expansion is the recall ceiling. If the eval shows paraphrase
misses, add vectors — inside Bleve, not as a second database — and score
hybrid.

## The ladder

Ask in this order. The first rung that passes the eval wins; do not skip a rung
because a newer technique is more interesting.

| # | Approach | Use when | Cost/query | Failure mode |
|---|---|---|---|---|
| 0 | Long context, whole corpus | corpus < ~100-150k tokens, changes slowly; gateway supports prompt caching | one big prompt, no infra | cost per call; context rot; citations only if the model returns ids |
| 1 | **Route-then-read**: Bleve selects whole documents/chapters, LLM reads them | bounded, terminology-rich corpus; citations matter | ~ms for routing + reading tokens | lexical recall; token budget; context rot |
| 2 | Tier 1 + curated glossary expansion | legal, accounting, health, legacy ERP: the domain has its own vocabulary | ~ms, $0, auditable | a bad glossary widens noise; log which term expanded |
| 3 | Tier 2 + OpenWN-PT (or a corporate thesaurus) | general Portuguese where the glossary is thin | ~ms, $0 | WordNet coverage is general, not domain |
| 4 | FastText query expansion | **only after an eval** shows tiers 1-3 miss | model size + cgo + load time | distributional neighbours are not synonyms; can blur "cancelamento"/"rescisão" |
| 5 | Embeddings / hybrid | eval shows lexical + synonyms still miss; corpus is large and paraphrastic | embedding + vectors + reindex | new infra, opaque ranking, cost |

Tiers 1-2 are the default for a consultancy delivery: deterministic, testable,
no model to host, and citations that point at a real offset.

## Documents: the pipeline in Go

1. **Ingest** (batch, never per query): extract text and keep `doc_id`,
   `title`, `updated_at`, `acl`, `lang`.
2. **Segment into reading units**: document → chapter → section. Prefer the
   chapter; if a chapter exceeds the reading budget, use its sections. Never
   split mid-sentence to hit a token count. Store `unit_id`, the heading path,
   `offset`, `tokens`.
3. **Index one record per unit** with
   [Bleve](https://github.com/blevesearch/bleve): BM25, a Portuguese analyzer
   (lowercase, stopwords, Snowball stemmer — Bleve ships language analyzers;
   confirm the package for the pinned version), a `keyword` field for codes/IDs
   (exact, boosted), title/heading boosts, and a synonym filter built from the
   glossary. The index is a build artifact: keep it on disk or in an object
   store, never in the run KV.
4. **Route**: expand the query with the glossary (query-time OR of synonyms),
   apply `acl`/date/type filters, search units, group by document, dedupe, then
   fit a **token budget** (not a fixed top-k). Order by score, then by document
   order so a chapter reads naturally.
5. **Read**: build the prompt in a prior step (observability rule). Stable
   prefix (system + rules + output schema), then the whole units with clear
   delimiters and ids, then the question. Ask for JSON:
   `{answer, citations: [{doc_id, unit_id, offset}]}`.
6. **Verify**: reject an answer whose citations are not in the selected set.
   That is a cheap hallucination gate and it costs no extra model call.

### Budget, not top-k

- Define `CONTEXT_BUDGET` as a fraction of the model window (start at 40-60%).
- Add units by relevance until the budget is reached; do not truncate a unit
  silently.
- If one unit alone exceeds the budget, fall back to its sections.
- If the answer needs more than the budget, use a map-reduce pass: summarise
  each unit against the question, then answer over the summaries. That is a
  second mode, not the default.

### Code sketch (Windmill Go node)

The node is a Go script (`language: go`), so the compiler checks the signature
(`docs/language-policy.md`):

```go
package inner

// RoutingUnit is a whole chapter/section, not a 500-token chunk.
type RoutingUnit struct {
	DocID   string
	UnitID  string
	Heading string
	Text    string
	Offset  int
	Tokens  int
	Score   float64
}

// main returns the units that fit budgetTokens; the LLM call is a separate step.
func main(question string, indexDir string, budgetTokens int) (interface{}, error) {
	// open indexDir, expand question with the glossary, search unit records,
	// group by document, then add whole units until budgetTokens is reached.
	return []RoutingUnit{}, nil
}
```

No Python/TS node: `docs/language-policy.md` applies. A WAC orchestrator may
only sequence these nodes.

### dagnats

The 12 handlers cannot open a Bleve index. Two valid shapes:

- **Small corpus**: `llm.complete` with the documents in the prompt (tier 0).
- **Larger corpus**: run the router as a separate Go service and call it with
  `http.request`; the recipe stays declarative. A new handler is a last resort
  (`AGENTS.md`), and retrieval is usually too client-specific to be a generic
  task.

## Why whole units can beat chunk RAG — and when they lose

| Whole-unit reading wins | Whole-unit reading loses |
|---|---|
| answers split across chunk boundaries | context rot: relevant text in the middle of a long prompt can be missed (Liu et al., "Lost in the Middle") |
| multi-paragraph reasoning inside one contract/manual | token cost and latency scale with the units sent |
| citations at a section a human can open | a single chapter can exceed the window |
| no embedding model or vector store to operate | lexical recall is still the ceiling |
| glossary terms stay visible in context | duplicates: ten near-identical chapters crowd out the right one |

Mitigations, not denials:

- keep the selected set small (budget, dedupe, drop near-duplicates);
- put the question **last** and the rules first;
- prefer chapter over whole document, section over chapter;
- measure answer accuracy, not token count.

**More context is not free accuracy.** Long context is a capability, not a
quality guarantee.

## Evaluated proposal: local SLM rewrite + cross-encoder reranking

A common upgrade is: rewrite the question with a local 3-8B SLM, search Bleve
for 20-50 snippets, rerank them with a local cross-encoder (BGE-reranker), and
send the top 5 to the cloud LLM. It is coherent, but it is **not a strict
upgrade** over route-then-read. Place each piece by what it actually fixes:

| Claim | Reality | Correct placement |
|---|---|---|
| SLM rewrite is milliseconds on CPU | a 3-8B model on CPU is seconds per rewrite and 5-8 GB of RAM; vLLM is GPU-first, Ollama runs on CPU more slowly | eval-gated, external service, cache by query hash; glossary first |
| a 1-3B model rewrites in tens of milliseconds | 1-3B is the right size for rewriting, but CPU generation is still hundreds of ms to a few seconds; quantization, prompt length and host decide | prefer 1-3B over 8B; keep a 1-3s timeout and the original-query fallback |
| a cross-encoder reranks 30 snippets in 50-100 ms | plausible for MiniLM-class (`ms-marco-MiniLM-L-6-v2`) with ONNX and short snippets; `bge-reranker-v2-m3` is several times heavier | pick by latency budget; measure on the host, not on a blog |
| run the reranker in a Python worker / ONNX inside the Go worker | a Python script node violates `docs/language-policy.md`; in-process ONNX adds cgo, native libs and memory to the worker | external inference service, called over HTTP by a Go node; separate binary if in-process |
| the reranker is ultra-light | a cross-encoder over 50 candidates is hundreds of ms to seconds on CPU, plus the model in memory | fine as a service; rerank **units/sections**, not snippets |
| top-5 snippets beat whole chapters | it reintroduces chunk boundaries and loses multi-paragraph context; the reranker only reorders what Bleve found | only when the token budget cannot fit the units; prefer unit-level rerank |
| selective prompt caching helps | with top-5 snippets there is no stable prefix to cache; caching is orthogonal to this stage | do not count it as a retrieval stage; measure hit rate |
| it replaces a vector database | it improves precision, not recall; Bleve still has to return the document | if paraphrase recall misses, add embeddings to Bleve's kNN index |

Corrected placement:

1. **Expand** — glossary first; an optional SLM rewrite behind an eval, with a
   timeout and a deterministic fallback. Cache the rewrite by query hash.
2. **Route** — Bleve returns **whole documents/chapters**, not 20-50 snippets.
3. **Rerank (optional)** — a cross-encoder scores **units** (chapter, or
   section when a chapter is too long), then the budget is filled in order.
4. **Read** — long context over the selected units; sections only when a unit
   exceeds the budget.
5. **Cache** — stable prefix; measure the hit rate.

Rules:

- Never make the deterministic path depend on an 8B model. If the inference
  service is down or slow, fall back to glossary + BM25 + whole-unit reading.
- A local model is an **external inference service**, not a script node: a Go
  node calls it over HTTP (`docs/language-policy.md`).
- The simplest semantic upgrade is often a small embedding model + Bleve kNN,
  not an 8B rewrite model. Compare them in the eval before adding either.
- "Zero token cost" is not zero infrastructure cost: account for the CPU/GPU,
  RAM, model weights, service uptime and the added latency.

### Execution topology in Windmill

Keep inference out of the worker and out of the request path's failure domain:

```text
flow: answer-question
  step 1 expand   Go node -> HTTP  inference service (Ollama/vLLM, 1-3B rewrite)
  step 2 route    Go node -> Bleve (top N units/sections, ACL filters)
  step 3 rerank   Go node -> HTTP  reranker (ONNX MiniLM or bge-reranker-v2-m3)
  step 4 read     llm.complete (stable prefix, whole units, token budget)
```

- One inference service per environment on the internal Docker network; never
  load a model per step execution. Health-check it and pin the model version.
- `1-3B` is enough for query rewriting; prefer it over 8B for latency. Expect
  hundreds of ms to a few seconds on CPU, not tens of ms.
- Reranker: MiniLM-class for latency, `bge-reranker-v2-m3` for quality; both
  are heavier than a BM25 pass. Measure on the target host.
- `top 30` candidates are **units/sections**, not arbitrary chunks. Top-5 is a
  budget outcome; do not fix it as a rule.
- A Python/FastEmbed service is infrastructure, not a script node; a Go node
  calls it over HTTP. In-process ONNX adds cgo/native libs to the worker and is
  not the default.

Fallback matrix:

| Failure | Fallback |
|---|---|
| rewrite timeout/error | route the original question through Bleve |
| reranker timeout/error | keep BM25 order and fill the token budget |
| inference service down | skip the semantic layer; route-then-read still answers |
| flow step fails after retries | flow error handler notifies; the run keeps its step outputs |

When it is worth it: the client needs paraphrase recall or multilingual query
coverage that the glossary cannot give, **and** the eval shows the gain beats
its latency/cost. When it is not: jargon-heavy corpora, strict latency, or an
operator who does not want another local service.

## Prompt caching: what it does and does not buy

- Route-then-read makes caching more likely to pay, because whole chapters
  repeat across questions (a policy manual, a contract set). But it is not
  automatic: if every query selects a different set, the hit rate is low.
- Cache the **stable prefix** (system, rules, output schema); put the question
  and the selected units last. Some gateways only cache exact prefixes.
- `llm.complete` here exposes `model`, `messages`, `base_url`, `max_tokens`,
  `temperature` and `timeout_ms` (`internal/handlers/llm_complete.go`) — there
  is **no `cache_control` field**. Caching is the gateway/provider's implicit
  behaviour. Measure the hit rate before promising a cost number.
- Do not sell "long-context + caching" as free retrieval. It is cheaper than
  uncached reading, not cheaper than not reading.

## Synonyms: glossary first, WordNet second, FastText last

The user-facing goal is "it understands cancelamento = rescisão". Three ways,
in increasing order of risk:

| Source | Size / license | Strength | Weakness |
|---|---|---|---|
| **Corporate glossary** (YAML in the client repo) | KBs, yours | exact, auditable, domain-correct | manual upkeep |
| **OpenWN-PT** (Portuguese WordNet) | tens of MB of data; check the data license | broad PT synonymy, deterministic graph | ~17k validated synsets; not legal/ERP-specific; needs a WordNet-format parser (e.g. `fluhus/gostuff/nlp/wordnet`) |
| **FastText** `cc.pt.300` | ~1.2 GB full, quantized ~300 MB; cgo bindings; check the **model** license — the MIT code license does not cover the pretrained vectors | subword morphology, distributional neighbours | neighbours are not synonyms; domain drift; heavy to load per worker |

The honest rule: **a table of the 200 terms the client actually uses beats a
300 MB model.** FastText is an experiment behind an eval, not a default. For
legal, health or accounting, prefer the glossary even when FastText wins a
generic benchmark: the glossary is reviewable by the client's specialist, and
"why did this match?" has a one-line answer.

OpenWN-PT is useful when the glossary is thin and the language is general
Portuguese. It is not a substitute for a corporate thesaurus, and it does not
know the client's internal codes.

## Tool data: Nango proxy or Nango MCP

Nango is the credential layer (`docs/nango.md`). It also exposes **Agent
Sessions**: an MCP server per session, with `mcp_url` + `session_token` from
`POST /sessions`. The tenant is selected by connection tags; the toolset can
pin/allow/deny catalog actions, custom actions, and tools from connected MCP
servers; `nango_tool_search` finds tools that are not pinned. Tool calls are
synchronous, capped at 60s and 5MB.

| Need | Use | Why |
|---|---|---|
| A recipe/flow knows the endpoint | **Nango HTTP proxy** | deterministic, one `http.request` node, testable |
| An interactive agent must discover and choose tools | **Nango Agent Session (MCP)** | scoped toolset, credentials never reach the model, per-tenant connection |
| Documents (PDFs, contracts) | **not Nango** | Nango brokers tools and credentials; it is not a document index. Index the documents yourself, or query the client's wiki/Drive API through a tool |

Traps:

- `nango_proxy` **bypasses allow/deny**; leave it off unless the agent genuinely
  needs arbitrary endpoints.
- The MCP tool list is a **snapshot at session creation**; a server that adds a
  tool needs a new session.
- A session token is a secret: keep the duration short, send it as bearer auth,
  never put it in a prompt or the run KV.
- Self-hosted Nango: confirm the pinned version supports agent sessions before
  promising it; the published docs use `api.nango.dev`.

**Agent loop gate:** an MCP-driven agent that chooses tools until done is a
platform decision, not a recipe (`skills/turvez/references/decision-tree.md`,
Q1.5). A deterministic "answer this question" flow uses fixed proxy calls, not
an MCP agent.

## Engine mapping

| Piece | Windmill | dagnats |
|---|---|---|
| Ingest/reindex | Go node on a schedule (`flow.yaml`), index in a volume/object store | `http.request` to the router service, or a manual batch job |
| Route | Go node (`main(question string, budgetTokens int)`) returns whole units | `http.request` to the service, or tier 0 via `llm.complete` |
| Prompt build | Go node or `transform.json`-equivalent step | `transform.json` (persisted before the model call) |
| Answer | flow step `llm.complete` | recipe step `llm.complete` |

One engine per client still applies: do not stand up a retrieval service as a
second orchestrator. It is a capability the client's existing engine calls.

## Evaluation (the ship gate)

Do not ship retrieval without a question set:

1. 30-50 real questions from the client, each with the document ids that answer
   it, plus 5-10 that should be refused (not in the corpus).
2. Measure:
   - `recall@unit` — did the right document/chapter appear?
   - citation validity — are the cited units in the selected set?
   - answer correctness (blind label);
   - tokens sent per query and cache hit rate;
   - p50/p95 latency and cost per query.
3. Compare against tier 0 (whole corpus), a chunked baseline, **and** a
   rewrite+rerank variant on the same set. Route-then-read must win on accuracy
   or cost, not on novelty.
4. If lexical routing misses, add vectors **in Bleve** and re-run; do not adopt
   a vector database as a separate service by reflex.
5. Re-run when the corpus or glossary changes. A retrieval system without an
   eval is a demo.

## Anti-patterns

- Fetching top-k chunks when the reader can read the chapter.
- Sending the whole corpus "because the model has a long context".
- No token budget, then silently truncating a chapter.
- Assuming prompt caching hits when every query selects different documents.
- A separate vector DB before the eval; Bleve can hold the vectors.
- An 8B rewrite plus a reranker as the default when a glossary and BM25 pass
  the eval.
- Reranking snippets when the unit is a chapter; that is how chunk RAG comes
  back through the side door.
- Embeddings first, eval never.
- Indexing a system of record instead of calling it.
- Letting the model decide access control; filter by `acl` at query time.
- Losing the offset, then asking for citations.
- Reindexing on every query.
- Putting raw client documents in the run KV (`docs/observability.md`).
- A 300 MB FastText model to expand five words.

## Sources (checked 2026-11-02)

- Bleve repository: https://github.com/blevesearch/bleve
- Bleve vectors and kNN: https://github.com/blevesearch/bleve/blob/master/docs/vectors.md
- Bleve docs (BM25, analyzers, synonym filter): https://blevesearch.com/docs/
- "Lost in the Middle: How Language Models Use Long Contexts": https://arxiv.org/abs/2307.03172
- Nango Agent Sessions: https://nango.dev/docs/guides/agent-sessions
- Nango MCP & Tools: https://nango.dev/platform/mcp/
- OpenWN-PT: https://github.com/own-pt/openwordnet-pt/
- Go WordNet parser: https://pkg.go.dev/github.com/fluhus/gostuff/nlp/wordnet
