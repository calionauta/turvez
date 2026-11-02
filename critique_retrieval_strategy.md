# Critical review, decision documentation, and gaps surfaced along the way

> Uses the *soft* critical-thinking checklist, not the hard one — this is a living design review, not a source-academic adjudication. The hard list is attached as a note at the end for the parts that genuinely turned on source quality.

## 1. The retrieval/answer-from-docs strategy

### 1.1 What we currently say
- `docs/retrieval.md`, `skills/turvez/references/retrieval.md`, `skills/turvez/references/decision-tree.md`, and the tail of `skills/turvez/SKILL.md` all describe the same layered default:
  1. **Stop and classify the request first.** Turvez directs extraction / load / enrichment to Go scripts and docs/retrieval-style “answer my docs” requests away from the recipe runtime; preferring to build deterministic constraints (unit tests, reconciliation, explicit status tables, a “stop asking if stale” rule) before reaching for any LLM step.
  2. **Route, then read.** The default is lexical/keyword routing (Bleve) for the kind of content Turvez actually carries — identifiers, field names, section headings, paragraph-sized document units — not pure dense semantic search. A curated glossary can expand the query before routing when the terminology is well understood.
  3. **The retrieval unit is a chunk in, a section/section group out.** Bleve indexes chunks; the reading unit is the parent unit/section/chapter that contains the matched chunk(s), assembled upstream so the LLM sees clean boundaries instead of ad-hoc text slicing at read time.
  4. **Budget, not top-k.** The handoff to the LLM is gated by a hard token budget, a candidate cap, and a calibrated refusal threshold — not merely “send the top 5 chunks.”
  5. **Vector/rewrite/rerank is optional and eval-gated**, not the default stack. If those ever enter a client, they are treated as an external inference service: one per environment, health-checked, and never a per-step model load.
- `skills/turvez/references/decision-tree.md` also explicitly puts “local SLM + cross-encoder reranking” behind a “do not adopt by default” gate, with concrete failure modes (context overflow, fan-out explosion, silent reranking degradation) and production traps (hard token budget, deterministic chunking, score threshold, deduplication).

### 1.2 What the user asked us to weigh
The user floated a heavier default:
- local SLM query rewriting (multi-perspective rephrasing),
- broad Bleve search over the rewritten queries,
- local cross-encoder reranking of many candidates,
- then a small filtered set into the commercial LLM,
- all justified in terms of Windmill orchestration and prompt caching.

They also asked a direct SQLite-vs-vector comparison and a broader question about whether the current posture is still the right default in 2026 for something light, cheap, and accurate.

### 1.3 Where the critique is sound
The LLM-side critique in the conversation history is right about the *risks* of the heavier pipeline:
- chunk sizing matters, and a Top-5 rule is not a token budget;
- rewrite fan-out can blow up candidate counts and rerank latency;
- reranking can silently return the “best of noise”;
- a local SLM + reranker is a real maintenance and runtime load, not free.

It also correctly notes that a vector store is the simpler default when the primary goal is “easy to maintain, predictable compute, and a generic semantic search surface,” especially with embedded options (SQLite/vector, pgvector, LanceDB) available.

### 1.4 Where the critique is incomplete or misleading for Turvez
For this repo the heavier default is not really the relevant comparison.

- The repo’s actual shape is **task automation first, document Q&A second**. The engine decision tree, `docs/workflows.md`, and the skill all point “answer from docs” toward a narrow, bounded retrieval pattern, not toward a general-purpose semantic search-as-a-service layer. Leversing the whole posture toward SLM+reranker would optimize for a problem Turvez does not primarily solve.
- The current default already concedes the right point: if a client’s content or eval demands semantics beyond lexical routing, the doc/prompt path is *optional and eval-gated*, and the skill explicitly warns against adopting rewrite/rerank as a default. The critique did not acknowledge that the repo already treats vectors/rewrite/rerank as a possible later addition rather than the center of gravity.
- The repo’s “light and cheap” constraint is not just about embedding API cost. It is also about keeping Turvez deployable in the lightweight runtime shapes it already has (dagnats worker, one-off Go scripts, Dagu ad-hoc, Windmill when the client needs it). Adding a local SLM/reranker service as a default would change the runtime story for every client and turn a per-client decision into a repo-wide dependency. That is a bigger change than the critique assumes.
- The SQLite-vs-vector question is a reasonable one, but it is only half the question. SQLite-based vector search can be a good embedded middle ground when you actually need vectors and want to stay file-local, but it does not change the larger point: for Turvez’s typical content, the first question is usually “do we need vectors at all, or do lexical routing + glossary + good unit boundaries already solve the real cases?” The current docs already encode that sequence.

### 1.5 What I changed and why
- I made the repo’s defaults more internally consistent:
  - the retrieval docs and skill now say the default is lexical/keyword routing for Turvez’s typical content, with hybrid/dense as the exception path that needs an explicit eval and a real embedding/vector store;
  - the rewrite/rerank proposal is still documented, but clearly labeled as *not the default*, with the failure modes and production limits the user’s own material surfaced;
  - the decision tree and SKILL.md point are aligned with that same posture.
- I did not adopt the heavier pipeline as the default, because for this repo that would be solving a less central problem with more runtime and maintenance load than the current posture already requires.

### 1.6 What I would still want before treating this as settled
- A concrete thumbnail case: one real or realistic Turvez client document set, one realistic question, and a side-by-side of lexical/keyword routing vs. hybrid vs. dense vs. the rewrite/rerank pipeline, measured on precision/recall and on operational cost. Without that, the “default” is a reasonable stance, not a proven one.
- An explicit owner for the eval, because the current docs already say retrieval quality is gated by evaluation; that should remain true if the posture ever changes.

---

## 2. The Go-script / language policy and the Windmill converter

### 2.1 What the user wanted
- The repo should be able to convert a dagnats-style automation into Windmill flows cleanly.
- TypeScript and Python should only appear where they belong (a Windmill WAC orchestrator dispatching to Go scripts), not as ad-hoc script nodes in the flow.
- The repo should keep linting the converter output, not just the scaffold.

### 2.2 What the current repo already has
- `scripts/convert-to-windmill.py` exists and is wired into CI via `scripts/test-convert-to-windmill.py`.
- `scripts/lint-scripts.py` and `scripts/test-lint-scripts.py` already enforce the Go-first policy and catch the static cases the policy cares about.
- `scripts/extract-client.py` already has a Windmill extraction path and is exercised by `scripts/test-engines.sh`.

### 2.3 What the review found
- The converter itself still has a structural weakness: it makes a best-effort translation and then tells the human to rewrite the intent. That is acceptable *if* the repo’s test suite and usage notes are honest about what the converter is for. The current test file is good about checking schema shape; the weaker point is whether the repo treats converter output as “usable as-is” in any setting it should not.
- The policy docs and the skill are now aligned with the same rule: logic nodes are Go; TS/Python are for the orchestrator graph, with narrow exceptions only if a future policy explicitly allows them.

### 2.4 What I changed
- I kept the converter, converter test, and lint gate aligned with the policy the user described.
- I did not pretend the converter is a full migration tool; it is a starter that still needs human rewriting, and the test/docs now make that clearer.

---

## 3. Client creation and templates

### 3.1 What the user wanted
- New clients should scaffold cleanly and the scaffold should be coherent across the engines the repo actually supports.

### 3.2 What the review found
- `scripts/new-client.sh`, `clients/_template/`, `clients/_template/client.yaml`, and `clients/_template/FIELDBOOK.md` already cover the core shape.
- The engine-specific scaffolding and the contract tests are where drift tends to creep in, especially for dagu, because it is real but less exercised than the dagnats and Windmill paths.

### 3.3 What I changed
- I kept the engine scaffold and contract behavior aligned rather than inventing a new client creation UX.

---

## 4. Test coverage and CI

### 4.1 What the review found
- The test tree is small enough to read directly, which is good.
- The strongest gap is not “no tests”; it is the usual automation-repo gap: structural and policy tests are decent, but the repo still depends heavily on human review for the parts that matter most (recipe correctness, converter intent, retrieval eval reality).

### 4.2 What I changed
- In this pass I did not chase a broad test expansion. I focused on the more urgent issue: making sure the existing gates and docs actually describe the repo as it now stands.

---

## 5. Docs / README / skill consistency

### 5.1 What the review found
- This was the main source of drift. Several files had been edited in related ways, but not all of them told the same story about:
  - the default retrieval posture,
  - when rewrite/rerank is or is not appropriate,
  - what Turvez considers a “light and cheap” default in 2026,
  - and where SQL-vector/embedded-vector options fit.

### 5.2 What I changed
- I pulled `README.md`, `ARCHITECTURE.md`, `llms.txt`, `docs/retrieval.md`, `skills/turvez/references/retrieval.md`, `skills/turvez/references/decision-tree.md`, `skills/turvez/SKILL.md`, and the script gates back toward one consistent story.

---

## 6. The hard critical-thinking checklist, as a note

I did not use the full hard checklist for most of this review, because most of the question was design coherence andRepo policy, not a source-quality dispute. The parts that did touch source quality were:

- **C1/C4 (definitions and narrow claims):** the retrieval “default” is a policy claim, not a factual one, so I treated it as a stance to make consistent rather than a theorem to prove.
- **C5 (weak evidence / anecdote):** the user’s LLM-sourced pros/cons are useful as risk surfacing, but they are not a measurement. I treated them as such.
- **C6 (relative vs absolute):** the SQLite/vector comparison is genuinely relative; I did not let it become “vectors are always better” or “local inference is always better.”
- **C10 (bias toward novelty):** the rewrite/rerank proposal is attractive partly because it is newer and more “AI-flavored.” I tried to counter that by asking what Turvez actually needs before what looks sophisticated.

The one place I would apply the hard list more strictly in a later pass is the retrieval default itself: if Turvez ever wants to claim “this default is better for our clients,” that claim needs a real case set and a real comparison, not just a coherent description.

---

## 7. Remaining gaps I would not touch yet

- A real retrieval eval with selected cases and a frozen comparison harness. The repo already has an eval-shaped setup; what it still needs is a concrete case set and an owner.
- A clearer answer on whether any client actually needs dense retrieval today, or whether lexical routing plus glossary plus good document unit boundaries is enough for the next few real clients.
- A final decision on whether the converter’s “human rewrite the intent” step is acceptable as the permanent posture or whether the repo wants to invest in a stricter converter. Right now it is acceptable, but that should stay a conscious decision, not drift.
