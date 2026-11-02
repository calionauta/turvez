# Retrieval: answer from documents or from the client's tools

Use when the client asks for "a chatbot over our knowledge base", "search our
contracts", "answer questions about our ERP/CRM", or any request that sounds
like RAG. Full rationale and sources: `docs/retrieval.md`.

## Split the request first

| The answer lives in | Do | Do not |
|---|---|---|
| A system of record (CRM, ERP, ticketing, wiki with an API) | call it through **Nango** — fixed HTTP proxy in a recipe/flow | index a copy that goes stale |
| Documents the client owns (PDFs, contracts, manuals) | index + retrieve + `llm.complete` | reach for embeddings by default |
| A few pages that fit one prompt | no index: long context, stable prefix | build a pipeline |

A system of record is a **freshness** problem, not a retrieval problem. Nango
also exposes **Agent Sessions (MCP)** for an agent that must discover and choose
tools; that is an agent loop, not a recipe (decision-tree Q1.5). Nango is not a
document index — it brokers tools and credentials.

## The ladder (documents)

Cheapest first; stop at the first rung that passes the eval.

1. **Long context** — corpus < ~100-150k tokens and stable. Put the corpus in a
   stable prefix and the question last. Verify that the client's gateway really
   caches; `llm.complete` has no `cache_control` field.
2. **[Bleve](https://github.com/blevesearch/bleve) BM25 (Go)** — exact terms, codes, clause numbers, names. A
   Portuguese analyzer (lowercase, stopwords, Snowball) + a `keyword` field for
   codes. Chunk by heading/clause, 300-800 tokens, keep `doc_id` + offset.
3. **+ curated glossary** — a YAML map of the client's own vocabulary
   (`cancelamento: [rescisão, distrato, encerramento]`). Deterministic,
   auditable, and the right default for legal, accounting, health and legacy
   ERPs. Log which term expanded.
4. **+ OpenWN-PT / corporate thesaurus** — when the glossary is thin and the
   language is general Portuguese. ~17k validated synsets; not domain-specific.
5. **FastText** — only behind an eval. The full PT model is ~1.2 GB (quantized
   ~300 MB), needs cgo, and nearest neighbours are not synonyms. A 200-term
   glossary beats it for a consultancy delivery.
6. **Embeddings / hybrid** — only when tiers 1-3 measurably miss. Hybrid with
   BM25, not a replacement. New infra, reindex cost.

The answer must cite `{doc_id, offset}`; reject citations that were not in the
retrieved set.

## Build shape

- **Windmill**: Go nodes — `index_docs.go` (batch), `search_docs.go`
  (`main(question string, topK int)`), `answer` via `llm.complete`. A scheduled
  flow reindexes; the index is a volume/object-store artifact, never run KV.
- **dagnats**: the 12 handlers cannot open an index. Small corpus → tier 0 via
  `llm.complete`. Larger corpus → a separate Go retriever called with
  `http.request`. A new handler is a last resort.
- Language policy applies: no Python/TS node; a WAC orchestrator only sequences
  Go nodes (`docs/language-policy.md`).

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
(plus 5-10 that must be refused). Measure `recall@5`, citation validity, answer
correctness, latency and cost/query. Compare with tier 0 on the same set. Re-run
when the corpus or glossary changes. Without this, it is a demo.

## Anti-patterns

- Embeddings before an eval.
- Indexing a CRM instead of querying it.
- The LLM deciding access control; filter by `acl` at query time.
- Losing the offset, then asking the model for citations.
- Raw client documents in the run KV.
