# Retrieval strategy critique — 2026 posture, Go + Windmill/dagnats/dagu

## Bottom line
The current posture is reasonable as a *default*, but it still overclaims on what
route-then-read replaces and still under-specifies the hybrid/index-unit split
where the biggest correctness gains are.

## What is now correct
- Lexical is still a strong baseline for bounded, terminology-rich corpora, and
  BM25 is often better than dense on exact-match identifiers even in 2026 finance
  benchmarks. Keeping lexical as the first cut for identifiers is not nostalgic;
  it is the right regress.
- Parent-as-reading-unit is the right structural move vs naive chunk-in-prompt
  RAG. Citations, context continuity and fewer boundary artifacts come from it.
- A hard token budget and a calibrated refusal are better operational ceilings
  than "top k".

## What still needs tightening
1. **Hybrid is not automatic, and it is not always better.** Hybrid BM25 + small
   dense + RRF can help paraphrase recall, but it also adds inference cost, an
   extra dependency, and ranking opacity. The default should say *lexical-first
   with a hybrid baseline when the eval asks for paraphrase recall*, not "hybrid
   is the new default everywhere".
2. **The clause about "vectors live in Bleve" is true but narrow.** Bleve has
   hybrid/vector support, but if the real gap is cross-lingual, open-domain or
   very large paraphrase-heavy corpora, the honest answer may be a dedicated
   semantic store, not "just add a vector field". Don't sell Bleve as universal.
3. **Query rewrite inside the router is risky as a default.** Even a 1B-class
   rewrite adds latency, a dependency and a fan-out multiplier. The right default
   is *no rewrite unless the eval shows lexical + synonyms under-recall*, and
   then one cached rewrite, not multiple.
4. **Reranking should be framed as optional and model-specific.** A MiniLM-class
   reranker on short units is not the same budget as `bge-reranker-v2-m3` on
   longer units. There is no universal 0.35 cutoff. This should stay explicit in
   docs and never be copied across models.
5. **Synonyms are the right first semantic signal for consultancy work.** A
   curated glossary beats distributional models for a client's own vocabulary in
   most delivery scenarios. FastText is a fallback, not a default. WordNet/
   OpenWN-PT is a fallback when the glossary is thin and the language is
   general, not domain-specific.

## Practical 2026 recommendation
- **Default stack**: Bleve (lexical + optional hybrid field), chunk indexing with
  parent mapping, glossary expansion, hard token budget, refusal cutoff.
- **Add hybrid only when the eval justifies paraphrase recall.**
- **Add a reranker only when candidate counts are large enough that top-k noise
  hurts and you can afford the rerank budget on the target CPU.**
- **Add query rewrite only when lexical + synonym + hybrid still under-recalls,
  and make it a single cached rewrite with a fallback to the original query.**
- **If the corpus is large/cross-lingual/open-domain, consider a dedicated
  semantic store, not "just Bleve harder".**

## Windmill/dagnats/dagu fit
- This is very doable in Go on Windmill as nodes, with the router/indexer as a
  scheduled or on-demand flow and the reader as `llm.complete`.
- In dagnats, you cannot open the index from the 12 handlers directly, so a
  separate Go router service over HTTP is the natural shape for anything beyond
  tiny corpora.
- Dagu is the wrong home for this; it is for recurring commands, not retrieval
  pipelines.

## Verdict
The strategy is now close to a good 2026 default, but should stop implying that
route-then-read *replaces* vector DBs universally. It replaces chunked vector RAG
for the bounded, terminology-rich case, and it keeps the door open for hybrid
and dedicated semantic stores when the corpus (not the trend) demands it.
