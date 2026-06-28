# Autogen Experiments Summary

## Scope

Sprint 2 container tests focused on model feasibility for the issue-to-plan autogen flow.
All candidates were executed on the same test case: `issue_02`.

The model input included:

- issue title and body;
- repository context files;
- `autogen_prompt.md`;
- `plan_format.md`.

The model was expected to return a Markdown `plan.md` only.

## Container Runtime

- GPU: NVIDIA A100 80GB PCIe.
- CUDA: available.
- PyTorch: 2.5.1+cu121.
- Transformers: 5.12.1.
- Quantized loading: `bitsandbytes` 4-bit where supported.

## Results

| Model | Status | Format Valid | Latency, s | Tokens/s | Peak VRAM, GB | Notes |
| --- | --- | --- | ---: | ---: | ---: | --- |
| `poolside/Laguna-XS.2` | ok | yes | 57.414 | 11.130 | 61.881 | Valid plan format, high VRAM usage. |
| `CohereLabs/North-Mini-Code-1.0` | ok | no | 80.060 | 15.988 | 56.771 | Good reasoning, but did not strictly follow all required sections. |
| `mistralai/Devstral-Small-2-24B-Instruct-2512` | ok | yes | 29.070 | 22.635 | 46.879 | Best smoke result: valid format, fastest latency, lower VRAM than 30B+ MoE candidates. |
| `JetBrains/Mellum2-12B-A2.5B-Thinking` | ok | no | 58.013 | 22.064 | 23.276 | Lowest VRAM, but Thinking output breaks strict `plan.md` format. |
| `Qwen/Qwen3.6-35B-A3B` | ok | no | 126.333 | 10.132 | 63.924 | Runs successfully, but torch fallback is slow and output starts with reasoning before the plan. |

## Preliminary Conclusion

For Sprint 2, `Devstral-Small-2-24B-Instruct-2512` is the strongest practical candidate from the container smoke test because it produced a valid `plan.md` with the best latency and moderate VRAM usage.
`Laguna-XS.2` also produced a valid plan, but required more VRAM and had slower generation.

`North Mini Code`, `Mellum2 Thinking`, and `Qwen3.6-35B-A3B` require prompt tuning or model-specific decoding controls before they can be used as strict issue-to-plan generators.

## Remaining Manual Work

The file `manual_evaluation.csv` is prepared for human scoring across five issues.
It should be completed manually for report-level quality metrics:

- file relevance;
- completeness;
- hallucinations;
- technical feasibility;
- tests quality.

## Related Files

- `container_smoke_results.csv`
- `container_smoke_results.json`
- `container_smoke_summary.md`
- `manual_evaluation.csv`
- `run_container_smoke.py`
