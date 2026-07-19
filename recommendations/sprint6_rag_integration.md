# Sprint 6 CodeRAG integration

## Implemented boundary

Agent Engine already owns an `HttpRagClient` and exposes it to the planning model through the
read-only `search_repository` tool. Sprint 6 completes the other side of that boundary in
[`GitFlame-CodeRAG`](https://github.com/GitFlameAI/GitFlame-CodeRAG/tree/sprint-6/http-rag-service):

- `GET /health` verifies CodeRAG database availability;
- `POST /search` accepts `query`, `top_k`, `repository_id`, `commit_sha`, `include`, and `exclude`;
- Bearer authentication is enabled when `RAG_API_KEY` is configured;
- every result has exactly `path`, `start_line`, `end_line`, `score`, and `content`;
- a missing repository or revision returns an empty result without generated evidence.

The CodeRAG service applies the retrieval threshold and hard context budgets before returning
snippets: requested `top_k`, maximum files, maximum chunks per file, token budget, and overlapping
range deduplication. This is score-threshold plus top-k selection, not LLM sampling `top_p`.

## Runtime configuration

Set the same private RAG key on both services. Do not reuse the model-provider key.

```dotenv
# Agent Engine
RAG_BASE_URL=http://coderag:8004
RAG_API_KEY=<private-rag-service-key>
MODEL_CONTEXT_LIMIT=32768

# CodeRAG
RAG_API_KEY=<private-rag-service-key>
RAG_MAX_CONTEXT_FILES=20
RAG_MAX_CHUNKS_PER_FILE=3
RAG_MAX_CONTEXT_TOKENS=12000
```

`MODEL_CONTEXT_LIMIT=32768` matches the context currently advertised by `laguna`. The repository
revision must be indexed in CodeRAG before Agent Engine searches it. Search does not clone or index
a repository on demand.

## Manual verification

1. Check CodeRAG directly:

   ```bash
   curl --fail "$RAG_BASE_URL/health"
   ```

2. Query a known indexed revision:

   ```bash
   curl --fail "$RAG_BASE_URL/search" \
     -H 'Content-Type: application/json' \
     -H "Authorization: Bearer $RAG_API_KEY" \
     -d '{
       "query": "where is authentication validated?",
       "top_k": 5,
       "filters": {
         "repository_id": "owner/repository",
         "commit_sha": "indexed-commit-sha",
         "include": ["**/*"],
         "exclude": ["node_modules/**", ".git/**"]
       }
     }'
   ```

3. Run an Agent Engine issue-to-plan request and confirm its trace contains a
   `search_repository` call and returned snippets from the same revision.
4. Repeat with an unknown revision and confirm the result is empty rather than fabricated.

For a 10-20 issue validation sample, record Recall@5, Precision@5, empty-result rate, p50/p95
latency, and admitted files/chunks/tokens. Calibrate the raw threshold on this sample; raw BM25/RRF
scores are not probabilities.

## Automated checks

```bash
cd recommendations
uv run pytest tests/test_agent_rag.py tests/test_agent_tools.py
```

The contract tests verify authentication, request filters, strict response validation, honest empty
results, and the 32K Laguna context limit.
