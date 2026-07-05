# Recommendation Model Comparison

## Sprint 4 Recommendation Strategy

Sprint 4 keeps recommendations on the dedicated recommendation service instead of merging them into
the Agent Engine `laguna` runtime.

`laguna` is the verified primary model for issue-to-plan and approved-plan-to-generated-files work.
The recommendation workflow stays separate because it already has a stable prompt, JSON Schema,
category allowlist, file/line validation, and explicit failure contract. This lets the team connect
real recommendations in the backend without making the Sprint 4 demo depend on a second migration
from Ollama structured outputs to OpenAI-compatible structured outputs.

The final supported recommendation categories are:

- `security`
- `performance`
- `maintainability`
- `architecture`
- `code_duplication`

The service accepts only these categories from `.yml` and rejects any model output outside the
allowlist.

## Decision

**Primary Sprint 1 candidate: `Qwen2.5-Coder-1.5B-Instruct`.**

It is the best initial deployment balance: code-specialized instruction tuning, Apache-2.0 license,
32K context, approximately 986 MB in the tested Ollama quantization, and practical local inference
on the available Apple M3 Pro with 18 GB unified memory. Qwen 1.5B matched the measured Qwen 7B
finding F1 while requiring less than half the latency and under one third of the loaded memory.

The benchmark results below must be interpreted as a comparison of quantized Ollama variants, not
as a reproduction of full-precision vendor benchmarks.

## Source-based Comparison

| Candidate | Code-analysis quality expectation | Context | Tested quantized size | License | Hardware / speed expectation | Sprint 1 suitability |
| --- | --- | ---: | ---: | --- | --- | --- |
| Qwen2.5-Coder-1.5B-Instruct | Modern code-specific model trained for generation, reasoning, and fixing; likely strongest small-model balance | 32K | 986 MB | Apache-2.0 | Fastest practical candidate after DeepSeek 1.3B; runs comfortably on M3 Pro and CPU VM | **Selected** |
| Qwen2.5-Coder-7B-Instruct | Better reasoning capacity and likely higher recall than 1.5B | 32K | 4.7 GB | Apache-2.0 | Higher latency and memory; still practical quantized on 18 GB M3 Pro | Strong quality reference |
| DeepSeek-Coder-1.3B-Instruct | Code-specialized older small model trained on code and natural language | 16K | 776 MB | DeepSeek model license | Low memory and fast, but weaker instruction/JSON reliability is expected | Small-model baseline |
| CodeLlama-7B-Instruct | Established code discussion/generation model, but older and not optimized for strict structured code review | 16K | about 3.8 GB | Llama 2 community license | 7B latency and memory without the newer Qwen advantages | Legacy comparison |
| Rule-based mock baseline | Excellent determinism for known patterns; cannot reason across files or generate robust semantic advice | N/A | negligible | Project-owned rules | Very fast and cheap | Comparison only; rejected as runtime fallback |

## Measured Local Benchmark

Measured on June 13, 2026, on an Apple M3 Pro with 18 GB unified memory. The table is generated
from `experiments/results/model_benchmark.json` after running:

```bash
uv run python experiments/run_benchmark.py
```

| Model | Schema valid | File/line valid | Precision | Recall | F1 | Mean latency | Output tokens/s | Max loaded memory |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Qwen2.5-Coder-1.5B-Instruct | 100% | 100% | **0.500** | **0.333** | **0.400** | **5.28 s** | 96.97 | **1.89 GB** |
| Qwen2.5-Coder-7B-Instruct | 100% | 100% | **0.500** | **0.333** | **0.400** | 11.95 s | 26.89 | 6.30 GB |
| DeepSeek-Coder-1.3B-Instruct | 100% | 66.7% | 0.250 | 0.167 | 0.200 | 3.12 s | **106.59** | 4.55 GB |
| CodeLlama-7B-Instruct | 100% | 66.7% | 0.000 | 0.000 | 0.000 | 14.53 s | 26.37 | 13.48 GB |

## Measured Interpretation

- Qwen 1.5B returned valid responses for every case, correctly identified the hardcoded API token,
  and tied Qwen 7B on automatic F1. Its second automatic match was a semantically incorrect
  duplication explanation, which demonstrates why manual review is required.
- Qwen 7B correctly identified the hardcoded token and SQL injection. It also produced one useful
  cross-file duplication finding that the exact-file metric counted as a false positive.
- DeepSeek produced valid output for two of three cases but violated the category filter in the
  remaining case. Its suggestions included duplicated and inaccurate claims.
- CodeLlama failed one case because it violated the category filter and all of its accepted findings
  were false positives. Its maximum loaded memory was also the highest in this run.
- The selected Qwen 1.5B baseline is suitable for demonstrating the full ML integration, but its
  measured recall is not sufficient for production repository review without further work.

## Evaluation Interpretation

- Finding-level precision, recall, and F1 match recommendations by category, file, and a line
  tolerance of three lines against manual labels.
- Schema-valid rate measures whether the model produced the fixed JSON contract.
- File/line-valid rate rejects hallucinated locations.
- Category coverage shows which requested recommendation types were produced.
- Latency and output token throughput represent the available M3 Pro environment.
- Suggestion usefulness is reviewed manually because lexical similarity is not a reliable measure
  of remediation quality.
- `precision@k`, `recall@k`, `nDCG@k`, and `MRR` are reserved for the future RAG retrieval stage.
- This is a small Sprint 1 experiment with six labels. Exact file matching penalizes a valid
  duplication finding when the model references the other duplicated file, while line tolerance
  can still match a nearby but semantically different issue. Manual review remains necessary.

## References

- [Qwen2.5-Coder-1.5B-Instruct model card](https://huggingface.co/Qwen/Qwen2.5-Coder-1.5B-Instruct)
- [Qwen2.5-Coder-7B-Instruct model card](https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct)
- [DeepSeek-Coder-1.3B-Instruct model card](https://huggingface.co/deepseek-ai/deepseek-coder-1.3b-instruct)
- [CodeLlama-7B-Instruct model card](https://huggingface.co/codellama/CodeLlama-7b-Instruct-hf)
- [Ollama Qwen2.5-Coder variants](https://ollama.com/library/qwen2.5-coder)
- [Ollama DeepSeek-Coder variants](https://ollama.com/library/deepseek-coder)
- [Ollama CodeLlama variants](https://ollama.com/library/codellama)
