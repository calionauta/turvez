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

The default is not "embed everything, fetch `k` chunks." It is four decisions:
the **routing unit**, the **query route**, the **rerank**, and the **reading
unit**. Doc rationale and sources: `docs/retrieval.md`.

```
[question]
   │
   ▼
1. CLASSIFY                 identifier (code/clause/id/quoted phrase)
   │                        vs natural-language query
   ├── identifier ──► BM25 lexical only  keyword/exact field, no fusion    └── natural lang ─► hybrid              BM25 + small dense, RRF over units,
                                               optional rerank 25-50
   ▼
2. EXPAND                  glossary → OpenWN-PT → FastText (only behind an eval);
                            optional SLM rewrite cache by query hash, fallback
   ▼
3. BLEVE ROUTING          whole units (section/chunk + parent), not 500-token chunks;
                            hybrid augments recall for paraphrase queries
   ▼
4. READ PARENT             re-expand the top chunks to their parent section/chapter,
                            read inside a token budget; stable prefix for caching;
                            drop to subsections when a parent exceeds the budget
Corrected placement:

1. **Expand** — glossary first; an optional SLM rewrite behind an eval, with a
timeout and a deterministic fallback. Cache the rewrite by query hash.
2. **Route** — Bleve returns whole units after dedupe and the candidate cap
(hybrid when the query is natural language; lexical-only for identifiers), not
20-50 snippets.
3. **Rerank (optional)** — a cross-encoder scores **units** (section, or its
parent when the section fits), then the budget is filled in reading order.
4. **Read** — long context over the selected **parent sections/chapters**; drop
to subsections when a reading unit exceeds the budget.
5. **Cache** — stable prefix; measure the hit rate.
```

**2026 default is not lexical-only and not vector-first.** For a bounded,
terminology-rich corpus it is **lexical for identifiers, hybrid for natural
language, optional cross-encoder rerank of units, and the parent as the reading
unit.** That is the practical shape of hybrid search + BM25 + small dense +
RRF + cross-encoder reranking + small-to-big / parent-document retrieval.

Why hybrid and not lexical-only or vector-only:

- BM25 is still a strong baseline; in a 2026 finance-domain benchmark
  (23,088 queries, 7,318 docs with text+tables) BM25 beats dense on Recall@5
  (0.644 vs 0.587) and wins on exact identifiers (rank 1 on 40/40 BM25 vs
  14/40 dense).
- Hybrid BM25+dense with RRF is the recommended minimum RAG baseline; hybrid
  + cross-encoder rerank is the best result there (Recall@5 ~0.816, MRR@3
  ~0.605). Dense-only underperforms both.
- Identifiers (clause numbers, codes, quoted phrases, exact names) route best
  with BM25 and a `keyword`/`exact` field; dense blurs and hides why it matched.
- Vectors are still useful when a query rephrases the corpus's vocabulary; they
  fix paraphrase recall, not the client's own terminology.

The **routing unit** is a chunk (~256-512 tokens, 50-100 overlap, recursive
chunking as a solid 2026 default) that remembers its **parent section/chapter**.
The **reading unit** is the parent, not the chunk and not the whole chapter.
This keeps chunk-level citations and section-level context. A single huge
"chapter" index unit is usually too coarse; a bare 300-500 token chunk is the
degraded mode, not the default reading unit. If the parent section exceeds the
reading budget, read its subsections.

**Verdict:** this replaces **chunked vector RAG and a separate vector DB** for
bounded, terminology-rich, citation-first corpora. It does **not** replace
semantic retrieval universally: lexical routing cannot recover a document whose
vocabulary never matches the query, reading a section/chapter is slower and
needier than 5k of chunks, and a corpus larger than one context needs
hierarchy or vectors. When the eval shows that gap, add vectors **inside Bleve**
(it has vector fields and kNN) — not a second database. The honest limit is
recall: route-then-read cannot recover a document it never routed to.

## The ladder (documents)

Cheapest first; stop at the first rung that passes the eval.

1. **Long context, whole corpus** — corpus < ~100-150k tokens and stable. Corpus
   in a stable prefix, question last. Confirm the gateway really caches;
   `llm.complete` has no `cache_control` field.
2. **Route-then-read** — Bleve BM25 over routing units (section/chunk + parent),
   Portuguese analyzer, `keyword`/`exact` field for codes, title/heading boosts.
   Return candidates within a token budget, not top-k.
3. **+ curated glossary** — a YAML map of the client's vocabulary
   (`cancelamento: [rescisão, distrato, encerramento]`). Deterministic,
   auditable, the right default for legal/accounting/health/legacy ERPs. Log
   which term expanded.
4. **+ OpenWN-PT / corporate thesaurus** — glossary thin, general Portuguese.
   ~17k validated synsets; not domain-specific.
5. **FastText** — only behind an eval. Full PT model ~1.2 GB (quantized ~300 MB),
   needs cgo, neighbours are not synonyms. A 200-term glossary beats it for a
   consultancy delivery.
6. **Embeddings / hybrid** — only when tiers 1-3 measurably miss; add to the
   Bleve index and score hybrid with BM25, not as a separate service.

The answer must cite `{doc_id, unit_id, offset}`; reject citations not in the
selected set.

Chunking defaults:

- Index ~256-512 tokens per chunk, ~50-100 token overlap, recursive chunking as
  a solid 2026 default.
- The reader reads the parent section/chapter of the top chunks, not the chunk
  alone. If the section exceeds the reading budget, read its subsections.
- A single huge "chapter" index unit is usually too coarse.
- 300-500 token chunks are the indexing unit for long documents, not the default
  reading unit.

## Build shape

- **Windmill**: Go nodes — `index_docs.go` (batch), `route_question.go`
  (`main(question string, budgetTokens int)`, returns parent units), `answer`
  via `llm.complete`. A scheduled flow reindexes; the index lives in a
  volume/object store, never run KV.
- **dagnats**: the 12 handlers cannot open an index. Small corpus → tier 0 via
  `llm.complete`. Larger corpus → a separate Go router called with
  `http.request`. A new handler is a last resort.
- Language policy applies: no Python/TS node; a WAC orchestrator only sequences
  Go nodes (`docs/language-policy.md`).

## Budget and reading

- Set `CONTEXT_BUDGET` at 40-60%; add **units** by relevance until full, never
  silently truncate.
- One oversized unit → use its subsections. More than the budget → map-reduce
  summaries, not a bigger prompt.
- Order by score, but keep each document's reading order; question last. Long
  context degrades for text in the middle ("Lost in the Middle"), so keep the
  selected set small and structured.

Hard limits, not top-k:

- Count tokens with the target tokenizer, reserve output + prefix, fail closed.
  Cap the reading unit at index time (e.g. 4k tokens); a bigger section is stored
  as subsections. 300-500 token chunks are the indexing unit for long docs, not
  the default reading unit.
- Classify the query first: identifiers → lexical only; natural language → hybrid.
- Candidate cap: 25-50 retained per query for hybrid, fewer for lexical; fan-out
  ≤3 rewrites × ≤20 units, dedupe by `doc_id:unit_id` (plus a content hash),
  ≤60 before reranking.
- Reranker: batch, per-call timeout, BM25 fallback. Calibrate the score
  threshold per model on the eval set; never copy 0.35. If nothing passes, the
  explicit "not found" answer wins and no cloud call is made.
- Bounded, not immune: measure worst-case tokens, reranker p95 and false-refusal
  rate.

## If someone proposes SLM rewrite + reranker

A local 3-8B rewrite plus a cross-encoder is a valid **eval-gated** layer, not a
default. Keep the unit.

- Bleve routes to whole document/section units; the reranker scores **units**
  (or sections when a section is too long), not 20-50 snippets.
- A rewrite model on CPU is **seconds and 5-8 GB**, not milliseconds; run it as
  an external service with a timeout, cache by query hash, and fall back to
  glossary + BM25 if it is down.
- Top-5 **snippets** reintroduce chunk boundaries; drop to sections when the
  reading budget cannot fit the unit. Top-5 **sections** is a budget outcome,
  not a rule.
- "Zero token cost" is not zero infrastructure cost. Compare against the cheapest
  semantic upgrade first: a small embedding model + Bleve kNN.
- A reranker in another language is an external service called by a Go node; the
  default is **Go/ONNX**, never a Python/TS script node
  (`docs/language-policy.md`).
- One inference service per environment (Ollama/vLLM for a 1-3B rewrite; ONNX
  MiniLM or bge-reranker-v2-m3 for reranking) on the internal network; never load
  a model per execution. 1-3B is right-sized for rewriting, but expect hundreds
  of ms to seconds on CPU, not tens of ms.
- Fallbacks: rewrite fails → route the original query; rerank fails → BM25 order;
  service down → route-then-read; LLM step fails → flow retry/alert.

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

- Fetching top-k chunks when the reader can read the parent section.
- Sending the whole corpus "because the context is long".
- No token budget, then silent truncation.
- Assuming caching hits when every query selects different documents.
- A separate vector DB before the eval; Bleve can hold the vectors.
- Embeddings before an eval.
- Indexing a CRM instead of querying it.
- The LLM deciding access control; filter by `acl` at query time.
- Losing the offset, then asking for citations.
- Raw client documents in the run KV.
- Preferring a 300 MB FastText model over a 200-term curated glossary for the
  client's own vocabulary.
- Treating lexical-only as the default for natural-language queries.
- Treating a chapter as the only index unit for long documents.
- Top-5 snippets as the reading unit when the parent section fits the budget.
- Preferring a 300 MB FastText model over a 200-term curated glossary for the
  client's own vocabulary.
