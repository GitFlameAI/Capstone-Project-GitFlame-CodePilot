#!/usr/bin/env python3
"""Run issue-to-plan cases against an OpenAI-compatible chat endpoint."""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import statistics
import time
import urllib.error
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]
SUITE = Path(__file__).resolve().parent
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
PATH_PATTERN = re.compile(r"`([^`\n]+[./][^`\n]+)`\s*(\(create\))?")


def load_case(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def build_user_prompt(case: dict) -> str:
    sections = [
        "# Issue",
        f"Title: {case['title']}",
        "",
        case["body"],
        "",
        "# Evaluation Contract",
        (SUITE.parent.parent / "plan_format.md").read_text(encoding="utf-8"),
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
    return "\n".join(sections)


def request_completion(base_url: str, api_key: str, model: str, messages: list[dict], timeout: int) -> tuple[str, dict]:
    endpoint = base_url.rstrip("/") + "/chat/completions"
    payload = {
        "model": model,
        "messages": messages,
        "temperature": 0.1,
        "max_tokens": 3000,
        "stream": False,
    }
    request = urllib.request.Request(
        endpoint,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")[:1000]
        raise RuntimeError(f"HTTP {exc.code}: {detail}") from exc
    elapsed = time.perf_counter() - started
    data = json.loads(raw)
    content = data["choices"][0]["message"]["content"]
    usage = data.get("usage", {})
    completion_tokens = usage.get("completion_tokens")
    tokens_per_second = completion_tokens / elapsed if completion_tokens else None
    return content, {
        "latency_seconds": round(elapsed, 3),
        "prompt_tokens": usage.get("prompt_tokens"),
        "completion_tokens": completion_tokens,
        "tokens_per_second": round(tokens_per_second, 3) if tokens_per_second else None,
    }


def automatic_metrics(plan: str, case: dict) -> dict:
    heading_positions = [plan.find(heading) for heading in REQUIRED_HEADINGS]
    headings_present = sum(position >= 0 for position in heading_positions)
    ordered = all(
        left < right
        for left, right in zip(heading_positions, heading_positions[1:])
        if left >= 0 and right >= 0
    )
    expected = set(case["expected_relevant_files"])
    attached = set(case["attached_files"])
    path_matches = [(path.strip(), marker) for path, marker in PATH_PATTERN.findall(plan)]
    mentioned = {path for path, _ in path_matches}
    mentioned_existing = mentioned & attached
    hallucinated = sorted(path for path, marker in path_matches if path not in attached and not marker)
    true_positive = expected & mentioned_existing
    precision = len(true_positive) / len(mentioned_existing) if mentioned_existing else 0.0
    recall = len(true_positive) / len(expected) if expected else 1.0
    return {
        "format_valid": headings_present == len(REQUIRED_HEADINGS) and ordered,
        "headings_present": headings_present,
        "headings_required": len(REQUIRED_HEADINGS),
        "relevant_file_precision": round(precision, 3),
        "relevant_file_recall": round(recall, 3),
        "hallucinated_file_count": len(hallucinated),
        "hallucinated_files": hallucinated,
    }


def write_summary(model_slug: str, rows: list[dict]) -> None:
    result_path = SUITE / "results" / f"{model_slug}_automatic.csv"
    fields = [
        "model",
        "issue_id",
        "success",
        "format_valid",
        "relevant_file_precision",
        "relevant_file_recall",
        "hallucinated_file_count",
        "latency_seconds",
        "prompt_tokens",
        "completion_tokens",
        "tokens_per_second",
        "error",
    ]
    with result_path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)

    successful = [row for row in rows if row["success"]]
    aggregate = {
        "model": rows[0]["model"] if rows else model_slug,
        "cases": len(rows),
        "success_rate": len(successful) / len(rows) if rows else 0,
        "format_valid_rate": statistics.mean(row["format_valid"] for row in successful) if successful else 0,
        "mean_file_precision": statistics.mean(row["relevant_file_precision"] for row in successful) if successful else 0,
        "mean_file_recall": statistics.mean(row["relevant_file_recall"] for row in successful) if successful else 0,
        "hallucinated_files_total": sum(row["hallucinated_file_count"] for row in successful),
        "median_latency_seconds": statistics.median(row["latency_seconds"] for row in successful) if successful else None,
        "mean_tokens_per_second": statistics.mean(
            row["tokens_per_second"] for row in successful if row.get("tokens_per_second") is not None
        ) if any(row.get("tokens_per_second") is not None for row in successful) else None,
    }
    (SUITE / "results" / f"{model_slug}_aggregate.json").write_text(
        json.dumps(aggregate, indent=2), encoding="utf-8"
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default=os.getenv("MODEL_API_URL", "http://localhost:8000/v1"))
    parser.add_argument("--api-key", default=os.getenv("MODEL_API_KEY") or os.getenv("HF_TOKEN", "local"))
    parser.add_argument("--model", required=True)
    parser.add_argument("--model-slug", required=True)
    parser.add_argument("--case", default="all", help="Case ID or all")
    parser.add_argument(
        "--system-prompt",
        type=Path,
        default=SUITE / "benchmark_prompt.md",
        help="System prompt file (defaults to benchmark_prompt.md)",
    )
    parser.add_argument("--timeout", type=int, default=600)
    args = parser.parse_args()

    case_paths = sorted((SUITE / "cases").glob("issue_*.json"))
    if args.case != "all":
        case_paths = [path for path in case_paths if path.stem == args.case]
    if not case_paths:
        raise SystemExit(f"No benchmark cases matched {args.case!r}")

    response_dir = SUITE / "responses" / args.model_slug
    response_dir.mkdir(parents=True, exist_ok=True)
    system_prompt = args.system_prompt.read_text(encoding="utf-8")
    rows = []

    for case_path in case_paths:
        case = load_case(case_path)
        prompt = build_user_prompt(case)
        (response_dir / f"{case['id']}_input.md").write_text(prompt, encoding="utf-8")
        row = {"model": args.model, "issue_id": case["id"], "success": False, "error": ""}
        try:
            plan, runtime = request_completion(
                args.base_url,
                args.api_key,
                args.model,
                [{"role": "system", "content": system_prompt}, {"role": "user", "content": prompt}],
                args.timeout,
            )
            (response_dir / f"{case['id']}_response.md").write_text(plan.strip() + "\n", encoding="utf-8")
            metrics = automatic_metrics(plan, case)
            row.update(runtime)
            row.update(metrics)
            row["success"] = True
            (response_dir / f"{case['id']}_metrics.json").write_text(
                json.dumps({**runtime, **metrics}, indent=2), encoding="utf-8"
            )
        except Exception as exc:  # Keep the remaining cases running after one failure.
            row.update(
                {
                    "format_valid": False,
                    "relevant_file_precision": 0,
                    "relevant_file_recall": 0,
                    "hallucinated_file_count": 0,
                    "latency_seconds": None,
                    "prompt_tokens": None,
                    "completion_tokens": None,
                    "tokens_per_second": None,
                    "error": str(exc),
                }
            )
        rows.append(row)
        print(f"{case['id']}: {'ok' if row['success'] else row['error']}", flush=True)

    write_summary(args.model_slug, rows)
    return 0 if all(row["success"] for row in rows) else 1


if __name__ == "__main__":
    raise SystemExit(main())
