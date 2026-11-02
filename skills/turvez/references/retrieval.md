# Retrieval: answer from documents or from the client's tools

Use when the client asks for "a chatbot over our knowledge base", "search our
contracts", "answer questions about our ERP/CRM", or any request that sounds
like RAG. Full rationale and sources: `docs/retrieval.md`.

## Split the request first

| The answer lives in | Do | Do not |
|---|---|---|
| A system of record (CRM, ERP, ticketing, wiki with an API) | call it through **Nango** — fixed HTTP proxy in a recipe/flow | index a copy that goes stale |
| Documents the client owns (PDFs, contracts, manuals) | **route-then-read** (below) | reach for embeddings by default |
| A few pages that fit one prompt | no index: long context, stable prefix | build a pipeline |

A system of record is a **freshness** problem, not a retrieval problem. Nango
also exposes **Agent Sessions (MCP)** for an agent that must discover and choose
tools; that is an agent loop, not a recipe (decision-tree Q1.5). Nango is not a
document index — it brokers tools and credentials.

## Route, then read

The default is not "embed everything, fetch five chunks". It is:

1. **Expand** the query locally — glossary first, OpenWN-PT second, FastText
   only behind an eval.
2. **Route with [Bleve](https://github.com/blevesearch/bleve)** — retrieve
   **whole documents/chapters**, not 500-token chunks. The index is a router,
   not the answer.
3. **Read with the LLM** — send the selected units whole, ordered, within a
   **token budget**, in long context; keep a stable prefix for caching.

Why: chunking is a workaround for a small reader window. When the model can read
the chapter, the chapter is the right unit — no answers split across boundaries,
no five fragments of the same page, citations at a section a human can open, and
lexical search only has to be right at document granularity. It also removes the
embedding model and vector store from the default stack.

**Verdict:** this replaces **chunked vector RAG and a separate vector DB** for
bounded, terminology-rich, citation-first corpora (contracts, policies, manuals,
legal/health/ERP). It does **not** replace semantic retrieval universally:
lexical routing cannot recover a document whose vocabulary never matches the
query, reading 30-60k tokens is slower and costlier per query, and a corpus
larger than the context needs hierarchy/map-reduce. When the eval shows that
gap, add vectors **inside Bleve** (it has vector fields and kNN) — not a second
database.

## The ladder (documents)

Cheapest first; stop at the first rung that passes the eval.

1. **Long context, whole corpus** — corpus < ~100-150k tokens and stable. Put
   the corpus in a stable prefix and the question last. Verify that the client's
   gateway really caches; `llm.complete` has no `cache_control` field.
2. **Route-then-read** — Bleve BM25 over whole units (chapter, or section when
   a chapter exceeds the budget), Portuguese analyzer, `keyword` field for
   codes, title/heading boosts. Return units within a token budget, not top-k.
3. **+ curated glossary** — a YAML map of the client's own vocabulary
   (`cancelamento: [rescisão, distrato, encerramento]`). Deterministic,
   auditable, and the right default for legal, accounting, health and legacy
   ERPs. Log which term expanded.
4. **+ OpenWN-PT / corporate thesaurus** — when the glossary is thin and the
   language is general Portuguese. ~17k validated synsets; not domain-specific.
5. **FastText** — only behind an eval. The full PT model is ~1.2 GB (quantized
   ~300 MB), needs cgo, and nearest neighbours are not synonyms. A 200-term
   glossary beats it for a consultancy delivery.
6. **Embeddings / hybrid** — only when tiers 1-3 measurably miss; add them to
   the Bleve index and score hybrid with BM25, not as a separate service.

The answer must cite `{doc_id, unit_id, offset}`; reject citations that were not
in the selected set.

## Build shape

- **Windmill**: Go nodes — `index_docs.go` (batch), `route_question.go`
  (`main(question string, budgetTokens int)`, returns whole units), `answer` via
  `llm.complete`. A scheduled flow reindexes; the index is a
  volume/object-store artifact, never run KV.
- **dagnats**: the 12 handlers cannot open an index. Small corpus → tier 0 via
  `llm.complete`. Larger corpus → a separate Go router called with
  `http.request`. A new handler is a last resort.
- Language policy applies: no Python/TS node; a WAC orchestrator only sequences
  Go nodes (`docs/language-policy.md`).

## Budget and reading

- Set `CONTEXT_BUDGET` at 40-60% of the model window; add units by relevance
  until it is full, and never silently truncate a unit.
- One oversized unit → use its sections. More than the budget → map-reduce
  summaries, not a bigger prompt.
- Order units by score but keep each document's reading order; put the question
  last. Long context degrades for text in the middle ("Lost in the Middle"), so
  keep the selected set small and structured.

## If someone proposes SLM rewrite + reranker

A local 3-8B rewrite plus a cross-encoder is a valid **eval-gated** layer, not
a default. Keep the unit:

- Bleve routes to whole documents/chapters; the reranker scores **units** (or
  sections when a chapter is too long), not 20-50 snippets.
- A rewrite model on CPU is **seconds and 5-8 GB**, not milliseconds; run it as
  an external service with a timeout, cache by query hash, and fall back to
  glossary + BM25 if it is down.
- Top-5 snippets reintroduce chunk boundaries; only drop to sections when the
  reading budget cannot fit the unit.
- "Zero token cost" is not zero infrastructure cost. Compare against the
  cheapest semantic upgrade: a small embedding model + Bleve kNN.
- A reranker in another language is an external service called by a Go node;
  the default is **Go/ONNX**, and never a Python/TS script node
  (`docs/language-policy.md`).
- One inference service per environment (Ollama/vLLM for a 1-3B rewrite; ONNX
  MiniLM or bge-reranker-v2-m3 for reranking) on the internal network; never
  load a model per execution. 1-3B is right-sized for rewriting, but expect
  hundreds of ms to seconds on CPU, not tens of ms.
- Fallbacks: rewrite fails → route the original query; rerank fails → BM25
  order; service down → route-then-read; LLM step fails → flow retry/alert.
- 30 candidates are units/sections, not arbitrary chunks; top-5 is a budget
  outcome, not a rule.

## Nango MCP: when it is the right answer

| Need | Use |
|---|---|
| A recipe/flow knows the endpoint | Nango **HTTP proxy** (`docs/nango.md`) |
| An agent must search/choose tools across connected SaaS | Nango **Agent Session** → `mcp_url` + `session_token`; scope by connection tags and toolset |
| PDFs/contracts | index them yourself; Nango is not a document store |

Traps: `nango_proxy` bypasses allow/deny (leave it off); the tool list is a
snapshot at session creation; 60s/5MB caps; the session token is a secret. On
self-hosted Nango, confirm the pinned version supports agent sessions.

## Ship gate

Do not promise "RAG" without 30-50 real questions with expected document ids
(plus 5-10 that must be refused). Measure `recall@unit`, citation validity,
answer correctness, tokens/query, cache hit rate, latency and cost. Compare with
tier 0 **and** a chunked baseline; route-then-read must win on accuracy or cost.
Re-run when the corpus or glossary changes. Without this, it is a demo.

## Anti-patterns

- Fetching top-k chunks when the reader can read the chapter.
- Sending the whole corpus "because the context is long".
- No token budget, then silent truncation.
- Assuming caching hits when every query selects different documents.
- A separate vector DB before the eval; Bleve can hold the vectors.
- Embeddings before an eval.
- Indexing a CRM instead of querying it.
- The LLM deciding access control; filter by `acl` at query time.
- Losing the offset, then asking for citations.
- Raw client documents in the run KV.
