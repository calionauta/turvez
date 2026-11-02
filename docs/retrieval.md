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
| **Documents the client owns** (PDFs, contracts, manuals) | an index + retrieval + `llm.complete` (below) | embeddings by default |
| A handful of pages that fit one prompt | no index: long context with a stable cached prefix | Bleve, a vector DB, a reindex pipeline |

The two expensive mistakes are a vector database for a 40-page FAQ, and a local
index of HubSpot that answers with last month's deal stage.

## The ladder

Ask in this order. The first rung that passes the eval wins; do not skip a rung
because a newer technique is more interesting.

| # | Approach | Use when | Cost/query | Failure mode |
|---|---|---|---|---|
| 0 | Long context, no retrieval | corpus < ~100-150k tokens, changes slowly; gateway supports prompt caching | one big prompt, no infra | cost per call; context rot; citations only if the model returns ids |
| 1 | Lexical BM25 in Go ([Bleve](https://github.com/blevesearch/bleve)) | documents with exact terms, codes, names, clause numbers | ~ms, $0 | misses paraphrase/synonym |
| 2 | Tier 1 + curated glossary expansion | legal, accounting, health, legacy ERP: the domain has its own vocabulary | ~ms, $0, auditable | a bad glossary widens noise; log which term expanded |
| 3 | Tier 2 + OpenWN-PT (or a corporate thesaurus) | general Portuguese where the glossary is thin | ~ms, $0 | WordNet coverage is general, not domain |
| 4 | FastText query expansion | **only after an eval** shows tiers 1-3 miss | model size + cgo + load time | distributional neighbours are not synonyms; can blur "cancelamento"/"rescisão" |
| 5 | Embeddings / hybrid | eval shows lexical+synonyms still miss; corpus is large and paraphrastic | embedding + vector store + reindex | new infra, opaque ranking, cost |

Tiers 1-2 are the default for a consultancy delivery: deterministic, testable,
no model to host, and citations that point at a real offset.

## Documents: the pipeline in Go

1. **Ingest** (batch, never per query): extract text and keep `doc_id`, `title`,
   `section`, `offset`, `updated_at`, `acl`. One document becomes many chunks.
2. **Chunk by structure**, not by character count: headings, clauses,
   paragraphs; 300-800 tokens, 10-15% overlap. Store the offset so an answer can
   cite the source instead of inventing one.
3. **Index** with [Bleve](https://github.com/blevesearch/bleve): BM25, a Portuguese analyzer (lowercase, stopwords,
   Snowball stemmer — Bleve ships language analyzers; confirm the package for
   the pinned version), a `keyword` field for codes/IDs (exact, boosted), and a
   synonym filter built from the glossary. The index is a build artifact: keep
   it on disk or in an object store, never in the run KV.
4. **Query**: expand the query with the glossary (query-time OR of synonyms),
   search, take top-k, then optionally rerank with a cross-encoder or the LLM —
   only if the eval says it pays.
5. **Prompt**: build it in a prior step (observability rule). Stable prefix
   (system + rules + output schema), then retrieved excerpts with ids, then the
   question. Ask for JSON: `{answer, citations: [{doc_id, offset}]}`.
6. **Verify**: reject an answer whose citations are not in the retrieved set.
   That is a cheap hallucination gate and it costs no extra model call.

### Code sketch (Windmill Go node)

The node is a Go script (`language: go`), so the compiler checks the signature
(`docs/language-policy.md`):

```go
package inner

// main returns the top-k chunks; the LLM call is a separate step.
// Parameters become the step's input schema and generated UI.
func main(question string, indexDir string, topK int) (interface{}, error) {
	// open indexDir, expand question with the glossary, build a boolean
	// query, return []struct{ DocID, Section string; Offset, Score int }
	return []any{}, nil
}
```

No Python/TS node: `docs/language-policy.md` applies. A WAC orchestrator may
only sequence these nodes.

### dagnats

The 12 handlers cannot open a Bleve index. Two valid shapes:

- **Small corpus**: `llm.complete` with the documents in the prompt (tier 0).
- **Larger corpus**: run the retriever as a separate Go service and call it with
  `http.request`; the recipe stays declarative. A new handler is a last resort
  (`AGENTS.md`), and retrieval is usually too client-specific to be a generic
  task.

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

## Prompt caching: what it does and does not buy

- Caching pays when a **stable prefix** repeats: system prompt, rules, output
  schema, and — in tier 0 — the corpus itself. Put the variable part (question,
  retrieved excerpts) last.
- It does **not** make per-query excerpts cheap. If excerpts change every query,
  they are not a cache hit. Do not sell "long-context + caching" as a retrieval
  strategy for a large corpus; those are two different modes.
- `llm.complete` here exposes `model`, `messages`, `base_url`, `max_tokens`,
  `temperature` and `timeout_ms` (`internal/handlers/llm_complete.go`) — there
  is **no `cache_control` field**. Caching is the gateway/provider's implicit
  behaviour. Verify it with the client's gateway before promising a cost number.

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
| Ingest/reindex | Go node on a schedule (`flow.yaml`), index in a volume/object store | `http.request` to the retriever service, or a manual batch job |
| Query | Go node (`main(question string, topK int)`) | `http.request` to the service, or tier 0 via `llm.complete` |
| Prompt build | Go node or `transform.json`-equivalent step | `transform.json` (persisted before the model call) |
| Answer | flow step `llm.complete` | recipe step `llm.complete` |

One engine per client still applies: do not stand up a retrieval service as a
second orchestrator. It is a capability the client's existing engine calls.

## Evaluation (the ship gate)

Do not ship retrieval without a question set:

1. 30-50 real questions from the client, each with the document ids that answer
   it, plus 5-10 that should be refused (not in the corpus).
2. Measure `recall@5` (did the right chunk appear), citation validity, answer
   correctness (blind label), p50/p95 latency, and cost per query.
3. Compare against tier 0 (long context) on the same set. If tier 0 wins on
   accuracy and the corpus fits, ship tier 0.
4. Re-run when the corpus or glossary changes. A retrieval system without an
   eval is a demo.

## Anti-patterns

- Embeddings first, eval never.
- Indexing a system of record instead of calling it.
- Letting the model decide access control; filter by `acl` at query time.
- Chunking by fixed character count, losing the offset, then asking for
  citations.
- Reindexing on every query.
- Putting raw client documents in the run KV (`docs/observability.md`).
- A 300 MB FastText model to expand five words.

## Sources (checked 2026-11-02)

- Nango Agent Sessions: https://nango.dev/docs/guides/agent-sessions
- Nango MCP & Tools: https://nango.dev/platform/mcp/
- Bleve repository: https://github.com/blevesearch/bleve
- Bleve docs (BM25, analyzers, synonym filter): https://blevesearch.com/docs/
- OpenWN-PT: https://github.com/own-pt/openwordnet-pt/
- Go WordNet parser: https://pkg.go.dev/github.com/fluhus/gostuff/nlp/wordnet
