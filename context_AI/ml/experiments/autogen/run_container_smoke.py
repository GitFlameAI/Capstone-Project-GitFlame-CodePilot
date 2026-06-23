#!/usr/bin/env python3
"""Run a small autogen smoke benchmark inside a remote Jupyter container.

The script stores compact metrics only. It intentionally does not write
per-issue `*_response.md` files.
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import time
import uuid
from pathlib import Path

import requests
import websocket


ROOT = Path(__file__).resolve().parents[4]
SUITE = Path(__file__).resolve().parent

MODELS = {
    "laguna_xs2": "poolside/Laguna-XS.2",
    "north_mini_code": "CohereLabs/North-Mini-Code-1.0",
    "devstral_small_2": "mistralai/Devstral-Small-2-24B-Instruct-2512",
    "mellum2_thinking": "JetBrains/Mellum2-12B-A2.5B-Thinking",
    "qwen36_35b_a3b": "Qwen/Qwen3.6-35B-A3B",
}

REQUIRED_HEADINGS = [
    "# Implementation Plan",
    "## Issue Summary",
    "## Goal",
    "## Relevant Files",
    "## Proposed Changes",
    "## Implementation Steps",
    "## Expected Files to Change",
    "## Tests and Verification",
    "## Risks and Open Questions",
]


def load_env_file(path: Path) -> dict[str, str]:
    if not path.exists():
        return {}
    values = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key] = value.strip().strip('"').strip("'")
    return values


def build_prompt(case_id: str) -> tuple[str, str]:
    case = json.loads((SUITE / "cases" / f"{case_id}.json").read_text(encoding="utf-8"))
    plan_format = (ROOT / "context_AI/ml/plan_format.md").read_text(encoding="utf-8")
    system_prompt = (ROOT / "context_AI/ml/autogen_prompt.md").read_text(encoding="utf-8")

    sections = [
        "# Issue",
        f"Title: {case['title']}",
        "",
        case["body"],
        "",
        "# Evaluation Contract",
        plan_format,
        "",
        "# Repository Context",
    ]
    for relative in case["attached_files"]:
        file_path = ROOT / relative
        sections.extend(
            [
                f"## File: `{relative}`",
                "```text",
                file_path.read_text(encoding="utf-8"),
                "```",
                "",
            ]
        )
    return system_prompt, "\n".join(sections)


def execute_remote(jupyter_url: str, jupyter_token: str, code: str, timeout: int) -> str:
    http_base = jupyter_url.rstrip("/")
    ws_base = re.sub(r"^http", "ws", http_base)
    response = requests.post(
        f"{http_base}/api/kernels",
        params={"token": jupyter_token},
        json={"name": "python3"},
        timeout=20,
    )
    response.raise_for_status()
    kernel_id = response.json()["id"]
    output: list[str] = []
    try:
        ws = websocket.create_connection(
            f"{ws_base}/api/kernels/{kernel_id}/channels?token={jupyter_token}",
            timeout=900,
        )
        msg_id = uuid.uuid4().hex
        message = {
            "header": {
                "msg_id": msg_id,
                "username": "codex",
                "session": "codex",
                "msg_type": "execute_request",
                "version": "5.3",
            },
            "parent_header": {},
            "metadata": {},
            "content": {
                "code": code,
                "silent": False,
                "store_history": False,
                "user_expressions": {},
                "allow_stdin": False,
                "stop_on_error": True,
            },
            "channel": "shell",
            "buffers": [],
        }
        ws.send(json.dumps(message))
        started = time.time()
        while time.time() - started < timeout:
            event = json.loads(ws.recv())
            if event.get("parent_header", {}).get("msg_id") != msg_id:
                continue
            msg_type = event["msg_type"]
            content = event.get("content", {})
            if msg_type == "stream":
                text = content.get("text", "")
                output.append(text)
                print(text, end="", flush=True)
            elif msg_type == "error":
                text = "\n".join(content.get("traceback", []))
                output.append(text)
                print(text, flush=True)
            elif msg_type == "status" and content.get("execution_state") == "idle":
                break
        ws.close()
    finally:
        requests.delete(
            f"{http_base}/api/kernels/{kernel_id}",
            params={"token": jupyter_token},
            timeout=20,
        )
    return "".join(output)


def remote_code(
    model_slug: str,
    model_id: str,
    case_id: str,
    system_prompt: str,
    user_prompt: str,
    hf_token: str,
    max_new_tokens: int,
) -> str:
    return f"""
import gc
import json
import os
import time
import traceback

os.environ["HF_TOKEN"] = {json.dumps(hf_token)}

model_slug = {json.dumps(model_slug)}
model_id = {json.dumps(model_id)}
case_id = {json.dumps(case_id)}
system_prompt = {json.dumps(system_prompt)}
user_prompt = {json.dumps(user_prompt)}
required_headings = {json.dumps(REQUIRED_HEADINGS)}
max_new_tokens = {max_new_tokens}

print("START_MODEL " + model_id, flush=True)
row = {{"slug": model_slug, "model_id": model_id, "issue_id": case_id, "status": "started"}}

try:
    import torch
    from transformers import AutoModelForCausalLM, AutoModelForImageTextToText, AutoTokenizer, BitsAndBytesConfig

    quantization = BitsAndBytesConfig(
        load_in_4bit=True,
        bnb_4bit_compute_dtype=torch.bfloat16,
        bnb_4bit_quant_type="nf4",
        bnb_4bit_use_double_quant=True,
    )

    started = time.perf_counter()
    tokenizer_kwargs = {{"token": os.environ.get("HF_TOKEN") or None, "trust_remote_code": True}}
    if "Devstral" in model_id:
        tokenizer_kwargs["fix_mistral_regex"] = True
    tokenizer = AutoTokenizer.from_pretrained(model_id, **tokenizer_kwargs)
    print("TOKENIZER_OK", flush=True)

    load_error = None
    model = None
    load_attempts = [
        ("AutoModelForCausalLM", AutoModelForCausalLM, "bnb4"),
        ("AutoModelForImageTextToText", AutoModelForImageTextToText, "bnb4"),
        ("AutoModelForCausalLM", AutoModelForCausalLM, "native"),
        ("AutoModelForImageTextToText", AutoModelForImageTextToText, "native"),
    ]
    for loader_name, loader, quantization_mode in load_attempts:
        try:
            kwargs = {{
                "token": os.environ.get("HF_TOKEN") or None,
                "trust_remote_code": True,
                "device_map": "auto",
                "dtype": torch.bfloat16,
                "low_cpu_mem_usage": True,
            }}
            if quantization_mode == "bnb4":
                kwargs["quantization_config"] = quantization
            model = loader.from_pretrained(model_id, **kwargs)
            row["loader"] = loader_name
            row["runtime_quantization"] = quantization_mode
            break
        except Exception as exc:
            load_error = type(exc).__name__ + ": " + str(exc)[:1000]
            print("LOADER_FAILED " + loader_name + "/" + quantization_mode + " " + load_error, flush=True)
    if model is None:
        raise RuntimeError(load_error or "No loader succeeded")

    load_seconds = time.perf_counter() - started
    print("MODEL_LOADED " + str(round(load_seconds, 2)), flush=True)

    messages = [
        {{"role": "system", "content": system_prompt}},
        {{"role": "user", "content": user_prompt}},
    ]
    if getattr(tokenizer, "chat_template", None):
        text = tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)
    else:
        text = "\\n\\n".join([system_prompt, user_prompt, "# Implementation Plan"])

    inputs = tokenizer(text, return_tensors="pt")
    inputs = {{key: value.to(model.device) for key, value in inputs.items()}}
    prompt_tokens = int(inputs["input_ids"].shape[-1])

    if torch.cuda.is_available():
        torch.cuda.empty_cache()
        torch.cuda.reset_peak_memory_stats()

    generation_started = time.perf_counter()
    with torch.inference_mode():
        output = model.generate(
            **inputs,
            max_new_tokens=max_new_tokens,
            do_sample=False,
            pad_token_id=tokenizer.eos_token_id,
        )
    latency_seconds = time.perf_counter() - generation_started
    completion_tokens = int(output.shape[-1] - inputs["input_ids"].shape[-1])
    generated = tokenizer.decode(output[0][inputs["input_ids"].shape[-1]:], skip_special_tokens=True)
    headings_present = sum(1 for heading in required_headings if heading in generated)
    peak_vram_gb = round(torch.cuda.max_memory_allocated() / 1024**3, 3) if torch.cuda.is_available() else None

    row.update(
        {{
            "status": "ok",
            "load_seconds": round(load_seconds, 3),
            "latency_seconds": round(latency_seconds, 3),
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "tokens_per_second": round(completion_tokens / latency_seconds, 3) if latency_seconds else None,
            "peak_vram_gb": peak_vram_gb,
            "headings_present": headings_present,
            "headings_required": len(required_headings),
            "format_valid": headings_present == len(required_headings),
            "plan_preview": generated[:1600],
            "plan_output": generated,
        }}
    )
except Exception as exc:
    row.update(
        {{
            "status": "failed",
            "error": type(exc).__name__ + ": " + str(exc)[:1000],
            "traceback": traceback.format_exc()[-2500:],
        }}
    )
finally:
    try:
        del model
    except Exception:
        pass
    gc.collect()
    try:
        import torch
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
    except Exception:
        pass

print("SMOKE_JSON " + json.dumps(row, ensure_ascii=False), flush=True)
"""


def write_outputs(rows: list[dict]) -> None:
    results_dir = SUITE / "results"
    results_dir.mkdir(parents=True, exist_ok=True)
    (results_dir / "container_smoke_results.json").write_text(
        json.dumps(rows, indent=2, ensure_ascii=False),
        encoding="utf-8",
    )

    fields = [
        "slug",
        "model_id",
        "issue_id",
        "status",
        "loader",
        "runtime_quantization",
        "load_seconds",
        "latency_seconds",
        "prompt_tokens",
        "completion_tokens",
        "tokens_per_second",
        "peak_vram_gb",
        "headings_present",
        "headings_required",
        "format_valid",
        "error",
    ]
    with (results_dir / "container_smoke_results.csv").open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)

    lines = [
        "# Container Autogen Smoke Benchmark",
        "",
        "| Model | Status | Loader | Quantization | Format valid | Latency, s | Tokens/s | Peak VRAM, GB | Notes |",
        "| --- | --- | --- | --- | --- | ---: | ---: | ---: | --- |",
    ]
    for row in rows:
        notes = row.get("error", "")
        if row.get("status") == "ok" and not row.get("format_valid"):
            notes = f"{row.get('headings_present', 0)}/{row.get('headings_required', 0)} required headings"
        lines.append(
            "| {model} | {status} | {loader} | {quant} | {fmt} | {latency} | {tps} | {vram} | {notes} |".format(
                model=row.get("model_id", ""),
                status=row.get("status", ""),
                loader=row.get("loader", ""),
                quant=row.get("runtime_quantization", ""),
                fmt=row.get("format_valid", ""),
                latency=row.get("latency_seconds", ""),
                tps=row.get("tokens_per_second", ""),
                vram=row.get("peak_vram_gb", ""),
                notes=str(notes).replace("|", "/")[:180],
            )
        )
    (results_dir / "container_smoke_summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")

    output_lines = ["# Container Smoke Model Outputs", ""]
    output_lines.extend(
        [
            "This file contains consolidated model outputs for the smoke benchmark.",
            "It is intentionally stored as one file rather than separate per-model response files.",
            "",
        ]
    )
    for row in rows:
        output_lines.extend(
            [
                f"## {row.get('model_id', row.get('slug', 'unknown'))}",
                "",
                f"- Issue: `{row.get('issue_id', '')}`",
                f"- Status: `{row.get('status', '')}`",
                f"- Format valid: `{row.get('format_valid', '')}`",
                f"- Latency seconds: `{row.get('latency_seconds', '')}`",
                f"- Tokens per second: `{row.get('tokens_per_second', '')}`",
                f"- Peak VRAM GB: `{row.get('peak_vram_gb', '')}`",
                "",
            ]
        )
        if row.get("status") == "ok":
            output_lines.extend(["````markdown", row.get("plan_output", "").strip(), "````", ""])
        else:
            output_lines.extend(["````text", row.get("error", "").strip(), "````", ""])
    (results_dir / "container_smoke_model_outputs.md").write_text(
        "\n".join(output_lines).rstrip() + "\n",
        encoding="utf-8",
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--jupyter-url", required=True)
    parser.add_argument("--jupyter-token", default=os.getenv("JUPYTER_TOKEN"))
    parser.add_argument("--case", default="issue_02")
    parser.add_argument("--models", nargs="*", default=list(MODELS))
    parser.add_argument("--max-new-tokens", type=int, default=1280)
    parser.add_argument("--timeout", type=int, default=3600)
    args = parser.parse_args()

    if not args.jupyter_token:
        raise SystemExit("Jupyter token is required via --jupyter-token or JUPYTER_TOKEN")

    env = load_env_file(ROOT / ".env.local")
    hf_token = env.get("HF_TOKEN") or os.getenv("HF_TOKEN", "")
    system_prompt, user_prompt = build_prompt(args.case)

    rows: list[dict] = []
    for slug in args.models:
        model_id = MODELS[slug]
        print(f"\n=== {slug}: {model_id} ===", flush=True)
        output = execute_remote(
            args.jupyter_url,
            args.jupyter_token,
            remote_code(slug, model_id, args.case, system_prompt, user_prompt, hf_token, args.max_new_tokens),
            args.timeout,
        )
        marker = "SMOKE_JSON "
        matches = [line[len(marker):] for line in output.splitlines() if line.startswith(marker)]
        if matches:
            rows.append(json.loads(matches[-1]))
        else:
            rows.append({"slug": slug, "model_id": model_id, "issue_id": args.case, "status": "failed", "error": "No SMOKE_JSON returned"})
        write_outputs(rows)

    return 0 if all(row.get("status") == "ok" for row in rows) else 1


if __name__ == "__main__":
    raise SystemExit(main())
