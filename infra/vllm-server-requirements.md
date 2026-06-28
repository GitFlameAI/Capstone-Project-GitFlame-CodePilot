# vLLM Server Requirements

## Requirements

| Section | Requirement |
|---|---|
| GPU | Preferred: A100 80GB. Minimum: L40S 48GB. |
| GPU check | `nvidia-smi` must work inside the environment. |
| CUDA check | `python3 -c "import torch; print(torch.cuda.is_available())"` must return `True`. |
| RAM | 64GB minimum, 128GB preferred. |
| Disk | 150GB minimum, 250GB preferred for model cache. |
| Installation | Python 3.10+, CUDA-compatible PyTorch, vLLM, transformers, accelerate, bitsandbytes, huggingface_hub, git, curl, wget, tmux or screen. |
| Open ports | `9000` for primary model, `9001` for fallback model. |
| API contract | vLLM must expose OpenAI-compatible endpoints: `http://<server-ip>:9000/v1` and `http://<server-ip>:9001/v1`. |

## Team Access To Environment

We need:

```text
SSH or terminal access
permission to run long-running processes
access to model cache directory
ability to set HF_TOKEN
ability to restart vLLM process
```

## Expected Output

Please provide:

```text
server IP / hostname
SSH or terminal access
GPU name, VRAM, RAM, disk
model cache path
open ports
API key if auth is enabled
```
