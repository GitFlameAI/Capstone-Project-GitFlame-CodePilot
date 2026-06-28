# Issue-to-Plan Model Benchmark

This suite compares open-source models on the same five GitFlame CodePilot issues and repository context.

## Test Cases

| ID | Scenario | Attached files |
|---|---|---:|
| `issue_01` | PostgreSQL persistence for issue sessions | 4 |
| `issue_02` | ML client timeout and retry handling | 3 |
| `issue_03` | `.yml` RAG snippet limits | 4 |
| `issue_04` | Asynchronous frontend plan generation | 4 |
| `issue_05` | Recommendation retention and cleanup | 6 |

Each JSON file in `cases/` contains the issue, attached repository files, expected relevant files, and required plan points.

`issues_for_review.md` contains the same issues in a compact human-readable form.

## Run

Start an OpenAI-compatible model server, then run:

```bash
export MODEL_API_URL=http://localhost:8000/v1
export MODEL_API_KEY=local

python3 context_AI/ml/experiments/autogen/run_benchmark.py \
  --model MODEL_ID \
  --model-slug MODEL_SLUG
```

For Hugging Face Inference Providers:

```bash
export HF_TOKEN=hf_your_token
export MODEL_API_URL=https://router.huggingface.co/v1
```

The token requires permission to call Inference Providers. Do not commit it to the repository.

Use the same quantization, temperature, token limit, prompt, and five cases for every model.

## Outputs

- `responses/<model>/issue_*_input.md`: exact issue and attached file contents.
- `responses/<model>/issue_*_response.md`: raw model answer.
- `responses/<model>/issue_*_metrics.json`: automatic metrics per issue.
- `results/<model>_automatic.csv`: machine-readable per-issue results.
- `results/<model>_aggregate.json`: model-level aggregate.
- `results/model_comparison.csv`: final report table.
- `results/manual_evaluation.csv`: rubric for human scoring.
- `results/infrastructure_diagnostics.csv`: current container readiness checks.

Model specifications in `models.json` and `model_comparison.csv` are preliminary. Verify the exact checkpoint ID and its model card before downloading or reporting final hardware requirements.

## Metrics

| Metric | Type | Meaning |
|---|---|---|
| Format validity | Automatic | All required headings exist in order |
| File precision | Automatic | Mentioned attached files that are expected |
| File recall | Automatic | Expected files mentioned by the plan |
| Hallucinated files | Automatic | Referenced paths absent from supplied context |
| Latency | Automatic | End-to-end request time |
| Output tokens/s | Automatic | Completion tokens divided by request time |
| Completeness | Manual | Coverage of required plan points, 1-5 |
| Technical feasibility | Manual | Compatibility with current architecture, 1-5 |
| Tests quality | Manual | Specificity and adequacy of verification, 1-5 |

## Current Infrastructure Blocker

The Jupyter container is reachable, but CUDA is not usable. Current diagnostics:

```text
NVIDIA_VISIBLE_DEVICES=1
nvidia-smi: Failed to initialize NVML: Unknown Error
torch.cuda.is_available(): False
CUDA driver error: CUDA_ERROR_NO_DEVICE
```

The container must be restarted or recreated with a valid NVIDIA runtime/device mapping before 12B-35B inference can run. After the fix, verify both commands inside the container:

```bash
nvidia-smi
python3 -c "import torch; print(torch.cuda.is_available(), torch.cuda.get_device_name(0))"
```
