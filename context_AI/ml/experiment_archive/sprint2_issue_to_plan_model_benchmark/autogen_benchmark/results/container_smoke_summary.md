# Container Autogen Smoke Benchmark

| Model | Status | Loader | Quantization | Format valid | Latency, s | Tokens/s | Peak VRAM, GB | Notes |
| --- | --- | --- | --- | --- | ---: | ---: | ---: | --- |
| poolside/Laguna-XS.2 | ok | AutoModelForCausalLM | bnb4 | True | 57.805 | 11.054 | 61.881 |  |
| CohereLabs/North-Mini-Code-1.0 | ok | AutoModelForCausalLM | bnb4 | False | 81.148 | 15.774 | 56.771 | 8/9 required headings |
| mistralai/Devstral-Small-2-24B-Instruct-2512 | ok | AutoModelForImageTextToText | native | True | 29.034 | 22.663 | 46.878 |  |
| JetBrains/Mellum2-12B-A2.5B-Thinking | ok | AutoModelForCausalLM | bnb4 | False | 57.924 | 22.098 | 23.276 | 4/9 required headings |
| Qwen/Qwen3.6-35B-A3B | ok | AutoModelForCausalLM | bnb4 | False | 129.355 | 9.895 | 63.924 | 8/9 required headings |
