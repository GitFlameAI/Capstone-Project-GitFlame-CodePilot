#!/usr/bin/env python3
import argparse
import asyncio
import json
import platform
import statistics
import sys
import time
from dataclasses import asdict
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from recommendation_service.model_client import (  # noqa: E402
    ModelOutputError,
    RecommendationModelClient,
)
from recommendation_service.models import AnalyzeRequest  # noqa: E402
from recommendation_service.service import RecommendationService  # noqa: E402
from recommendation_service.settings import Settings  # noqa: E402

DEFAULT_MODELS = ["laguna"]


def load_cases(fixtures_dir: Path) -> list[dict[str, Any]]:
    return [json.loads(path.read_text()) for path in sorted(fixtures_dir.glob("*.json"))]


def match_findings(
    predicted: list[dict[str, Any]],
    expected: list[dict[str, Any]],
    line_tolerance: int = 3,
) -> dict[str, float | int]:
    unmatched = set(range(len(expected)))
    true_positives = 0
    for finding in predicted:
        match = next(
            (
                index
                for index in unmatched
                if expected[index]["category"] == finding["category"]
                and expected[index]["file"] == finding["file"]
                and abs(expected[index]["line"] - finding["line"]) <= line_tolerance
            ),
            None,
        )
        if match is not None:
            true_positives += 1
            unmatched.remove(match)

    false_positives = len(predicted) - true_positives
    false_negatives = len(expected) - true_positives
    precision = true_positives / len(predicted) if predicted else 0.0
    recall = true_positives / len(expected) if expected else 1.0
    f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
    return {
        "true_positives": true_positives,
        "false_positives": false_positives,
        "false_negatives": false_negatives,
        "precision": precision,
        "recall": recall,
        "f1": f1,
    }


async def run_model(model: str, cases: list[dict[str, Any]], timeout: float) -> dict[str, Any]:
    settings = Settings(model=model, request_timeout_seconds=timeout)
    service = RecommendationService(RecommendationModelClient(settings))
    case_results = []
    started = time.perf_counter()

    for case in cases:
        case_started = time.perf_counter()
        try:
            response, metrics = await service.analyze(
                AnalyzeRequest.model_validate(
                    {
                        "config_yaml": case["config_yaml"],
                        "repo_context": case["repo_context"],
                    }
                )
            )
            findings = [item.model_dump(mode="json") for item in response.recommendations]
            score = match_findings(findings, case["expected"])
            case_results.append(
                {
                    "name": case["name"],
                    "status": "ok",
                    "wall_time_seconds": time.perf_counter() - case_started,
                    "summary": response.summary,
                    "recommendations": findings,
                    "expected": case["expected"],
                    "score": score,
                    "inference_metrics": asdict(metrics),
                }
            )
        except Exception as exc:  # Benchmark records failures instead of hiding them.
            error = f"{type(exc).__name__}: {exc}"
            schema_valid = isinstance(exc, ModelOutputError) and not str(exc).startswith(
                "model returned invalid structured output"
            )
            case_results.append(
                {
                    "name": case["name"],
                    "status": "error",
                    "wall_time_seconds": time.perf_counter() - case_started,
                    "error": error,
                    "schema_valid": schema_valid,
                    "file_line_valid": False,
                    "expected": case["expected"],
                }
            )

    successful = [case for case in case_results if case["status"] == "ok"]
    totals = {
        "true_positives": sum(case["score"]["true_positives"] for case in successful),
        "false_positives": sum(case["score"]["false_positives"] for case in successful),
        "false_negatives": sum(case["score"]["false_negatives"] for case in successful)
        + sum(len(case["expected"]) for case in case_results if case["status"] == "error"),
    }
    tp = totals["true_positives"]
    fp = totals["false_positives"]
    fn = totals["false_negatives"]
    precision = tp / (tp + fp) if tp + fp else 0.0
    recall = tp / (tp + fn) if tp + fn else 0.0
    f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
    categories = {
        recommendation["category"]
        for case in successful
        for recommendation in case["recommendations"]
    }
    latencies = [case["wall_time_seconds"] for case in successful]
    completion_tokens = [
        case["inference_metrics"]["completion_tokens"]
        for case in successful
        if case["inference_metrics"]["completion_tokens"]
    ]
    total_latency = sum(case["wall_time_seconds"] for case in successful)
    tokens_per_second = (
        sum(completion_tokens) / total_latency
        if completion_tokens and total_latency
        else None
    )
    return {
        "model": model,
        "status": "ok" if len(successful) == len(cases) else "partial_or_failed",
        "schema_valid_rate": (
            len(successful)
            + sum(
                bool(case.get("schema_valid"))
                for case in case_results
                if case["status"] == "error"
            )
        )
        / len(cases),
        "file_line_valid_rate": (
            len(successful)
            + sum(
                bool(case.get("file_line_valid"))
                for case in case_results
                if case["status"] == "error"
            )
        )
        / len(cases),
        "category_coverage": sorted(categories),
        "precision": precision,
        "recall": recall,
        "f1": f1,
        "mean_latency_seconds": statistics.mean(latencies) if latencies else None,
        "tokens_per_second": tokens_per_second,
        "total_wall_time_seconds": time.perf_counter() - started,
        "cases": case_results,
    }


async def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--models", nargs="+", default=DEFAULT_MODELS)
    parser.add_argument("--fixtures", type=Path, default=ROOT / "experiments" / "fixtures")
    parser.add_argument(
        "--output",
        type=Path,
        default=ROOT / "experiments" / "results" / "model_benchmark.json",
    )
    parser.add_argument("--timeout", type=float, default=300)
    args = parser.parse_args()

    cases = load_cases(args.fixtures)
    results = []
    for model in args.models:
        print(f"Running {model} on {len(cases)} cases...", flush=True)
        results.append(await run_model(model, cases, args.timeout))

    payload = {
        "benchmark_version": 1,
        "environment": {
            "platform": platform.platform(),
            "machine": platform.machine(),
            "python": platform.python_version(),
            "runtime": "OpenAI-compatible endpoint",
            "generation": {"temperature": 0},
            "line_match_tolerance": 3,
        },
        "models": results,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(payload, indent=2) + "\n")
    print(f"Wrote {args.output}")


if __name__ == "__main__":
    asyncio.run(main())
