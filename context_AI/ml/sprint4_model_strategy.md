# Sprint 4 Model Strategy - Karim

## Verified vLLM endpoint

Sprint 4 uses the university OpenAI-compatible vLLM endpoint as the primary Agent Engine model
source:

```text
AGENT_MODEL=laguna
OPENAI_BASE_URL=https://gpu-1.devops-playground.innopolis.university/v1
OPENAI_API_KEY=<provided by deployment operator>
MODEL_CONTEXT_LIMIT=32768
```

Do not commit the API key. It must be supplied only through local `.env`, VM secrets, or CI/runtime
secret storage.

Verified on July 5, 2026:

| Check | Result |
| --- | --- |
| `GET /v1/models` | HTTP 200 |
| Listed model id | `laguna` |
| Model root | `poolside/Laguna-XS.2` |
| Serving stack | `vllm` |
| Advertised context | `32768` tokens |
| Agent Engine client readiness | `ready=True` |
| Agent Engine HTTP `/ready` | `{"status":"ready","model":"laguna","version":"3.0.0"}` |
| Streaming chat smoke test | Returned assistant content `ok` with usage metadata |
| `POST /v1/plans/generate` smoke test | `status=completed`, `model=laguna`, `plan_markdown` returned |
| `POST /v1/files/generate` smoke test | `status=completed`, generated-files contract returned |

The endpoint is compatible with the current `OpenAICompatibleClient` streaming path used by the
Agent Engine. Very small non-streaming `max_tokens` probes can spend all generated tokens on the
model reasoning field, so Sprint 4 smoke tests should use either `/ready` or the Agent Engine
streaming completion path.

The Agent Engine plan-generation smoke test used one synthetic issue and one supplied repository
file. It returned a valid implementation plan, one relevant file, usage metadata, and one read-only
tool call. The generated-files smoke test returned a validated generated-files response for the same
synthetic task. These checks prove integration compatibility; final product acceptance should still
use the real GitFlame issue/config/repository flow.

## Agent Engine method improvement

Compared with the previous hosted Qwen/Hugging Face router setup, Sprint 4 moves Agent Engine
runtime verification to a real university vLLM endpoint. The deployment now has concrete values for
`AGENT_MODEL`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `MODEL_CONTEXT_LIMIT`, and `/ready` verifies
that the configured model id is actually listed by the model server before the backend starts real
issue-to-plan or approved-plan-to-code-generation work.

## Recommendation model decision

Sprint 4 keeps recommendations on the dedicated recommendation service instead of moving them to
the Agent Engine model in this iteration.

Decision:

- `laguna` is the primary model for Agent Engine planning and generated-files contracts.
- The recommendation service remains a separate model/service boundary for Sprint 4, using the
  existing structured recommendation prompt, JSON Schema, and post-validation pipeline.
- A later unification can replace the Ollama-backed recommendation client with an
  OpenAI-compatible client, but that is not required for the Sprint 4 weekly demo.

Rationale:

- The Agent Engine needs a larger coding-agent model with tool use, RAG evidence handling, and
  code-generation capability; the verified `laguna` endpoint satisfies that requirement.
- Recommendations are a bounded repository-review task with a stable JSON contract and existing
  validation artifacts. Keeping it separate reduces Sprint 4 integration risk while Arthur connects
  backend recommendation flow to a real service.
- Separate runtime knobs let the team scale or disable recommendations without blocking
  issue-to-plan and code-generation readiness.

## Final Sprint 4 recommendation categories

The supported recommendation categories remain intentionally small and evidence-backed:

| Category | Supported meaning |
| --- | --- |
| `security` | Secrets, injection risks, unsafe auth/session handling, and insecure defaults. |
| `performance` | Avoidable hot-path cost, inefficient queries/loops, and expensive repeated work. |
| `maintainability` | Fragile structure, unclear ownership, hard-to-test code, and risky complexity. |
| `architecture` | Cross-module contract problems, misplaced responsibilities, and integration design issues. |
| `code_duplication` | Repeated logic that should be shared because it affects correctness or maintenance. |

The `.yml` config can request a subset of these categories. The recommendation service rejects any
model output outside this list and rejects hallucinated files or line numbers.

## Report links

- Model selection: `context_AI/ml/model_selection.md`
- Sprint 4 model strategy: `context_AI/ml/sprint4_model_strategy.md`
- Agent Engine report: `recommendations/agent_engine_report.md`
- Recommendation model comparison: `recommendations/model_comparison.md`
- Recommendation prompt: `recommendations/recommendation_prompt.md`
- Recommendation schema: `recommendations/recommendation_schema.json`
- Runtime env example: `.env.example`
