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

The default is **not** "embed everything, fetch `k` chunks, stuff them in the
prompt". It is four decisions, in order: the **routing unit** you index, how
you answer a **natural-language query** vs an **identifier query**, whether
you **rerank**, and what the reader actually reads. Call the result
**route-then-read**: the index is a router, not the answer, and the reader
is what produces the answer.

The 2026 default is neither "lexical only" nor "vectors first." For a
bounded, terminology-rich corpus it is **lexical for identifiers, hybrid
(BM25 + a small dense model) for natural-language queries**, an optional
cross-encoder rerank of the candidates, and **reading the parent** of the
chunk that matched rather than the chunk alone.

```
[question]
   │
   ▼
1. CLASSIFY                 identifier (code, clause, id, quoted phrase)
   │                        vs natural-language query
   │
   ├── identifier ──► BLEVE BM25          exact term / keyword field
   │                                            (no dense, no fusion)
   │
   └── natural language ─► HYBRID          BM25 + small dense embeddings,
                                                RRF over units, optional rerank
                                                (25-50 candidates)
   │
   ▼
2. EXPAND                  glossary → OpenWN-PT → FastText (only behind an eval)
   │                        apply synonym OR on the lexical side before search;
   │                        optional SLM rewrite cache by query hash, with fallback
   ▼
3. BLEVE ROUTING          retrieve chunks (~256-512 tokens) that remember
   │                        their parent; hybrid augments recall for
                          paraphrase-heavy queries
   ▼
4. LONG-CONTEXT READING   re-expand to the parent section/chapter,
                          send selected units within a token budget,
                          stable prefix for prompt caching
```

Why **hybrid** rather than lexical-only or vector-only:

- **BM25 is still a strong baseline.** In a 2026 finance-domain benchmark
  (23,088 queries over 7,318 documents with text and tables), BM25 beats dense
  retrieval on Recall@5 (0.644 vs 0.587), and wins on exact-match identifiers
  (rank 1 on 40/40 BM25 vs 14/40 dense).
- **Hybrid is the recommended baseline.** Bi-orthogonal RRF over BM25 + dense
  lifts Recall@5 to 0.695, and hybrid + a cross-encoder rerank reaches 0.816
  with MRR@3 around 0.605. The authors recommend hybrid as the minimum RAG
  baseline; dense-only underperforms it.
- **Dense-only underrecalls exact terms and is opaque.** Exact clause numbers,
  codes, quoted phrases and names surface first with BM25; embeddings blur and
  hide why a document was scored.
- **Vectors are still useful** when a query rephrases the corpus's vocabulary.
  They augment recall; they do not replace a reviewer's vocabulary for the
  client's own terms.

Why the **routing unit** is a section/chunk that remembers its parent, not a
whole chapter and not a bare 500-token chunk:

- **Chunking is the workaround for a small reader window.** When the model can
  afford a section or chapter, that bigger unit is what you read. The chunk is
  the **indexing unit** only; the **reading unit** is its parent.
- **BM25 only has to be right coarse enough to route.** Bleve's job is to get
  you to the right section; the model does the fine-grained reading.
- **Section-level indexing lets you re-expand to the parent.** You keep chunk
  boundaries (so citations are precise) **and** the surrounding section context
  (so the reader does not lose the argument split across chunks). This is the
  practical shape of small-to-big / parent-document retrieval.
- **A 300-500 token chunk is the indexing unit for long documents; the chapter
  is the reading unit when it fits the budget.** Do not confuse the two or treat
  either as "the unit".

This is still RAG. What changed between this and the 2024 default is the
**routing shape** (lexical for identifiers, hybrid for natural language,
cross-encoder rerank of units, parent-as-reading-unit) and the **defaults**
(lexical-first-with-hybrid-baseline instead of lexical-until-it-fails and
vector-database-first). Whether it replaces a separate vector database still
depends on the corpus, not on the name.

### Lexical vs hybrid vs dense — when each route wins

| Query type | Best route | Why | Risk if you pick wrong |
|---|---|---|---|
| Identifier: article/contract/clause number, code, exact quoted phrase, named entity, exact product spec | **BM25 lexical on a keyword/exact field** | exact match is faster, cheaper, auditable and almost always rank 1 | dense can miss the literal string; RRF can dilute an exact hit |
| Natural language, paraphrase, "does our policy cover X" type queries | **Hybrid BM25 + small dense, RRF over units, optional rerank** | paraphrasing and vocabulary mismatch are exactly what dense adds | lexical-only under-recalls; dense-only loses exact identifiers |
| Mixed (identifier + intent) | **Hybrid** | keeps the exact hit and adds paraphrase recall | |
| Very small, static corpus that fits one prompt | **Long context, no index** | no routing needed | routing adds work |
| Large, paraphrase-heavy, cross-lingual, open-domain | **Semantic dense (vector DB or heavy embedding model)** | lexical and small hybrid cannot recover the right document | lexical-first will miss |

The practical default on the router: **if the query looks like an identifier,
answer with lexical only; otherwise run hybrid and let the rerank (if you have
one) and the parent reading absorb the rest.**

### Where the query rewrite / reranker fit

If someone proposes adding a **query rewrite step** before the route, treat it
as a measured improvement, not a default. A local Small Language Model
(rewriting the question in one more semantic form) plus a cross-encoder rerank
can improve recall on a paraphrase-heavy corpus, but it also adds: a local
inference dependency, CPU cost, a timeout, a cache by query hash, and a
fan-out you have to bound. In the Go/Windmill shape this runtime favors, the
practical compromise is usually **one rewrite, cached**, with the rest of the
funnel kept small. Multiple rewrites are a precision tool for a corpus that
needs it, not a starting default.

The more important fork is hybrid vs lexical-first on the router itself: in Go,
a single query against the same Bleve index can target both the lexical and
vector fields, and RRF / cutoff logic stays in the router, not in the index.

### The retrieval unit: chunk in, section out

This is the biggest point of confusion and the one most likely to be wrong:

- What you **index** for a long document is a **chunk of ~256-512 tokens**
  (recursive chunking with ~50-100 token overlap is a solid 2026 default;
  whole-page or semantic chunking are alternatives in narrower cases).
- What you **retrieve** is a candidate list of those chunks, sorted and
  deduped.
- What you **read** is the **parent section/chapter** of the top candidate(s),
  not the chunk alone. The chunk localises; the section explains.

That means the index stores `doc_id`, `unit_id`, `heading_path`,
`parent_section_id`, `offset`, `tokens`, and the chunk text. The router
returns chunks; the reader expands them to parents. This is why "top 5
chunks" is the wrong answer but "5 sections that contain the matched chunks"
is the right one: the chunks find the signal, the section supplies the context.

It is also why a **chapter as the only index unit is usually too coarse**: if
your corpus is 60 pages of contracts, indexing a single huge "chapter" record
loses the localisation that made route-then-read useful in the first place, and
your citations point at something a human still has to hunt through.

Defaults to start from (tune with the eval):

| Limit | Starting default | Where enforced |
|---|---|---|
| Indexing unit | ~256-512 tokens, 50-100 token overlap; recursive chunking | indexer |
| Reading unit | parent section/chapter of the matched chunk; if the section/chapter exceeds the reading budget, use its subsections | router + reader |
| Retrieval candidates | 25-50 retained per query, hybrid if natural-language | router |
| Rerank candidates | 25-50, batch, timeout, BM25-order fallback | reranker (optional) |
| Context budget | 40-60% of the window, minus output and prefix reserve | reader, target tokenizer |
| Query rewrites | 0 or 1, cache by query hash, 1-3s timeout | orchestrator |
| Fan-out cap | ≤1 rewrite × ≤50 units each → ≤50 candidates before rerank | router |
| Refusal | cutoff calibrated per model on the eval set; no cloud call when nothing passes | orchestrator |
| Eval | 30-50 questions + 5-10 refusals, per-query-type labels | ship gate |

Do not treat `30` or `50` as universal. In a 2026 local measurement on a
~560M cross-encoder, 25 candidates took ~71 ms, 50 ~138 ms, 100 ~271 ms, and
200 ~540 ms — the knee is around 50. If you keep a rewrite step at all, one
rewrite is usually the practical cap in the Go/Windmill shape this runtime favors,
because each rewrite multiplies both candidates and token budget.

### Budget, not top-k

- Define `CONTEXT_BUDGET` as a fraction of the model window (start at 40-60%).
- Add **reading units** by relevance until the budget is reached; do not
  truncate a unit silently.
- If the relevant chunk's parent unit alone exceeds the budget, fall back to
  its subsections.
- If the answer needs more than the budget, use a map-reduce pass: summarise
  each unit against the question, then answer over the summaries. That is a
  second mode, not the default.

The budget is a **hard limit**, not a target. Count with the tokenizer of the
model that will read the prompt (or a conservative estimate plus a character
cap), reserve space for the answer and the system prefix, and fail closed:

```go
const (
	contextBudget = 24000 // window minus output reserve and prompt overhead
	maxUnitTokens = 4000  // bigger units are split at index time
)

// fillBudget fills the context budget with whole reading units, not chunks.
// parentUnits come from expanding the top chunks to their parent sections.
func fillBudget(parentUnits []Unit, budget int) []Unit {
	out := make([]Unit, 0, len(parentUnits))
	used := 0
	for _, u := range parentUnits {
		if u.Tokens > maxUnitTokens || used+u.Tokens > budget {
			continue
		}
		out = append(out, u)
		used += u.Tokens
	}
	return out
}
```

`maxUnitTokens` is enforced at **index time**: a section larger than that is
stored as subsections, not as one 10k-token record. That is the structural fix
for "one result is 10,000 tokens"; the budget is the runtime backstop.

Bounding the funnel before the cloud call:

1. **Identify the query type first.** Identifiers go lexical; natural language
goes hybrid. Do not route everything through dense because the corpus is
"semantic enough."
2. **Keep the candidate count bounded.** 25-50 per query, hybrid only when the
query is natural language. Four rewrites × top 50 = 200 candidates is how the
local reranker times out.
3. **Reranker budget.** Batch the candidates, set a per-call timeout, and keep a
BM25-order fallback. Measure p95 on the target CPU; a MiniLM-class model on
short units is not the same as `bge-reranker-v2-m3` on 50 long ones.
4. **Calibrated threshold, not a magic number.** Cross-encoder scores are not
comparable across models (`bge` logits vs MiniLM sigmoid), and 0.35 is not a
universal cutoff. Calibrate on the eval set for a precision target; if no unit
passes, return the explicit "not found in the corpus" answer without calling
the cloud model. Log the scores so a false refusal is diagnosable.

**Bounded, not immune.** Tokenizers differ from your estimate, PDFs contain
tables that blow up a section, the reranker can OOM, and a threshold that is
right for one model is wrong for another. The design bounds the failure; it
does not eliminate it. Measure worst-case tokens, reranker p95 and refusal
rate.

### Production limits (starting points)

The tiers are a cost/correctness ladder, not a menu. The 2026 version of this
ladder makes three clear points versus earlier defaults:

1. **Split the ladder by query type, not just by cost.** Identifiers stay
   lexical (BM25 on a keyword/exact field). Natural-language, paraphrase, and
   vocabulary-mismatch queries get a hybrid route at the same cost tier: the same
   index, a small dense embedding model over the same indexing units, RRF, optional
   cross-encoder rerank of units, and the same parent-as-reading-unit shape.
2. **Query rewrite is not a default rung.** _When the eval justifies it_, a local
   Small Language Model rewriting the question can help on a paraphrase-heavy
   corpus, but it also adds a local inference dependency, CPU cost, a timeout, a
   cache by query hash, and a fan-out you have to bound. In the Go/Windmill shape
   this runtime favors, the practical compromise is usually one rewrite, cached,
   with a fallback to the original query.
3. **Editing the index is a normal stop, not a novelty.** If the term mismatch
   is predictable and expensive, build it into the index (lexical field, synonym
   filter, hybrid field) rather than asking the orchestrator to solve it at
   query time every call.

These defaults are repeated across the repo. If a file disagrees with the
table below, the file is wrong — this guide is the normative copy.

| Limit | Default | Where enforced |
|---|---|---|
| Reading unit | chapter; its sections when the chapter exceeds ~4k tokens | indexer |
| Context budget | 40-60% of the window, minus output and prefix reserve | router, target tokenizer |
| Query rewrites | 0 or 1, cache by query hash, 1-3s timeout | orchestrator |
| Candidates | 25-50 retained per query for hybrid, fewer for lexical | router |
| Reranker | batch, per-call timeout, BM25 fallback | inference service + node |
| Refusal | cutoff calibrated per model; no cloud call when nothing passes | orchestrator |
| Eval | 30-50 questions + 5-10 refusals | ship gate |

A fixed threshold such as 0.35 is not a default: cross-encoder scores are
model-specific (logits vs sigmoid) and query-dependent. Calibrate it on the
refusal set and log the score of every dropped unit.

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
| run the reranker inside the Go worker via in-process ONNX | in-process ONNX adds cgo, native libs and memory to the worker; a Python script node violates `docs/language-policy.md` | external **Go/ONNX** inference service, called over HTTP by a Go node; separate binary if in-process |
| the reranker is ultra-light | a cross-encoder over 50 candidates is hundreds of ms to seconds on CPU, plus the model in memory | fine as a service; rerank **units/sections**, not snippets |
| top-5 snippets beat whole chapters | it reintroduces chunk boundaries and loses multi-paragraph context; the reranker only reorders what Bleve found | only when the token budget cannot fit the units; prefer unit-level rerank |
| selective prompt caching helps | with top-5 snippets there is no stable prefix to cache; caching is orthogonal to this stage | do not count it as a retrieval stage; measure hit rate |
| it replaces a vector database | it improves precision, not recall; Bleve still has to return the document | if paraphrase recall misses, add embeddings to Bleve's kNN index |

Corrected placement:1. **Expand** — glossary first; an optional SLM rewrite behind an eval, with a
timeout and a deterministic fallback (route the original question through Bleve).
Cache the rewrite by query hash. If you add a rewrite step at all, keep it to one
cached rewrite, not a multi-rewrite fan-out.
2. **Route** — Bleve returns candidate chunks after dedupe and the candidate cap;
   the reader then expands them to their parent units
(hybrid when the query is natural language; lexical-only for identifiers), not
20-50 snippets.
3. **Rerank (optional)** — a cross-encoder scores **units** (section, or its
parent when the section fits), then the budget is filled in reading order.
4. **Read** — long context over the selected **parent sections/chapters**; drop
to subsections when a reading unit exceeds the budget.
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
  step 2 route    Go node -> Bleve (top N units/sections, ACL filters, hybrid if NL)
  step 3 rerank   Go node -> HTTP  reranker (ONNX MiniLM or bge-reranker-v2-m3)
  step 4 read     llm.complete (stable prefix, parent sections, token budget)
```

- One inference service per environment on the internal Docker network; never
  load a model per step execution. Health-check it and pin the model version.
- `1-3B` is enough for query rewriting; prefer it over 8B for latency. Expect
  hundreds of ms to a few seconds on CPU, not tens of ms.
- Reranker: MiniLM-class for latency, `bge-reranker-v2-m3` for quality; both
  are heavier than a BM25 pass. Measure on the target host.- The candidate set is ≤1 rewrite × ≤50 units, deduped, then ≤50 units before
reranking, if you use a rewrite step at all. `top 30` is a starting point; top-5 is
a budget outcome, not a rule. The literal ≤3 rewrites version is legacy wording and
should not be used in new docs.
- A **Go/ONNX** inference service is the default: infrastructure, not a script
  node, called over HTTP by a Go node. Python/FastEmbed is an exception only
  for a model the Go path does not have. In-process ONNX adds cgo/native libs
  to the worker and is not the default.
- The reader reads **parent sections**, not chunks. If a section exceeds the
  budget, read its subsections.

Fallback matrix:

| Failure | Fallback |
|---|---|
| rewrite timeout/error | route the original question through Bleve |
| reranker timeout/error | keep BM25 order and fill the token budget |
| inference service down | skip the semantic layer; route-then-read still answers |
| no unit clears the calibrated cutoff | return the explicit "not found" answer; no cloud call |
| the budget cannot fit even one unit | return "not found"; never truncate mid-sentence |
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
| Route | Go node (`main(question string, budgetTokens int)`) returns the top chunks expanded to their parent units, within budget | `http.request` to the service, or tier 0 via `llm.complete` |
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
   - tokens sent per query, **worst-case tokens**, and cache hit rate;
   - false-refusal rate on answerable questions, and correct refusal on the
     5-10 that must be refused;
   - reranker p95 and p50/p95 end-to-end latency and cost per query.
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
- Treating "top 5" as a safety limit; the limit is tokens, not items.
- A fixed 0.35 reranker threshold copied across models; calibrate it.
- Unbounded rewrite fan-out (4 x top 50) before the reranker.
- No explicit "not found" path when every score is low.
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

## Source note

Same facts are mirrored in `docs/retrieval.md` and `skills/turvez/references/retrieval.md`;
the script mirrors the docs, not a pasted substack. If the LLM source below conflicts with
this file, this file wins.
