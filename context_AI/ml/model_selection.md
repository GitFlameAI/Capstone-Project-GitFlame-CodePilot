# Open-Source Model Selection

## Candidate Table

| Model | Size | License | Current availability | Intended role | Short justification |
|---|---:|---|---|---|---|
| **Qwen3-Coder-30B-A3B-Instruct** | 30B total / 3B active, MoE | Apache 2.0 | Available through Hugging Face Inference Providers | **Primary candidate** | Specialized for repository-level coding and agent workflows; strong quality-to-inference-cost balance. |
| Qwen2.5-Coder-32B-Instruct | 32B, dense | Apache 2.0 | Available through Hugging Face Inference Providers | Fallback | Mature coding model with broad language support and predictable instruction following. |
| DeepSeek-Coder-V2-Lite-Instruct | 16B total / 2.4B active, MoE | DeepSeek Model License | Model is on Hugging Face Hub, but not currently available through Inference Providers | Original lightweight fallback | Lower active parameter count and suitable for cheaper local inference. |
| Cohere North Mini Code | 30B total / 3B active, MoE | Apache 2.0 | Not currently listed by Hugging Face Inference Providers | Alternative primary | Coding-focused MoE candidate with efficient inference and permissive licensing. |
| Poolside Laguna XS.2 | 33B total / 3B active, MoE | Apache 2.0 | Not currently listed by Hugging Face Inference Providers | Alternative primary | Designed for coding and agentic tasks with a small active parameter count. |
| Devstral Small 2 | 24B, dense | Apache 2.0 | Not currently listed by Hugging Face Inference Providers | Quality fallback | Coding-agent model with moderate size and simpler dense deployment. |
| Mellum2 12B Thinking | 12B total / 2.5B active, MoE | Apache 2.0 | Not currently listed by Hugging Face Inference Providers | Lightweight candidate | Smallest original candidate; intended for code generation, editing, debugging, and tool use. |
| Qwen3.6-35B-A3B | 35B total / 3B active, MoE | Apache 2.0 | Exact 35B checkpoint is not currently listed by Hugging Face Inference Providers | Additional main candidate | Newer repository-level and agentic coding candidate with only 3B active parameters. |

## Preliminary Selection

- **Primary:** `Qwen/Qwen3-Coder-30B-A3B-Instruct`
- **Fallback:** `Qwen/Qwen2.5-Coder-32B-Instruct`
- **Original lightweight fallback:** `DeepSeek-Coder-V2-Lite-Instruct`
- **Temporary hosted lightweight fallback:** `Qwen/Qwen2.5-Coder-7B-Instruct`, because DeepSeek-Coder-V2-Lite is not currently available through Hugging Face Inference Providers.

Qwen3-Coder-30B-A3B-Instruct remains the main candidate because it is coding-focused, uses only 3B active parameters, has an Apache 2.0 license, and can already be called through the selected Hugging Face integration.

## Testing Status

A single Qwen3-Coder request was completed to verify that the Hugging Face integration can return a valid `plan.md`. This is an integration demonstration, not a comparative benchmark.

Full model testing cannot be completed in the current sprint because the provided GPU container does not expose a working CUDA device, while most original candidates are unavailable through Hugging Face Inference Providers. Comparative evaluation of plan quality, latency, resource usage, and quantized checkpoints is therefore moved to the next sprint after the container is fixed.

