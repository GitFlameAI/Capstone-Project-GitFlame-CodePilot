"""Prometheus metrics shared by the Python services.

Everything here is deliberately low-cardinality: model names, operation names,
outcomes and tool names are drawn from a fixed vocabulary. Repository ids, issue
ids and file paths are never labels — they belong in the logs, where an
unbounded number of distinct values costs nothing.
"""

from __future__ import annotations

import time
from contextlib import contextmanager
from typing import Iterator

from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

HTTP_REQUESTS = Counter(
    "codepilot_http_requests_total",
    "HTTP requests handled by the service.",
    ["method", "route", "status"],
)
HTTP_REQUEST_DURATION = Histogram(
    "codepilot_http_request_duration_seconds",
    "HTTP request duration.",
    ["method", "route"],
    buckets=(0.05, 0.1, 0.5, 1, 5, 15, 60, 180, 600),
)

LLM_REQUESTS = Counter(
    "codepilot_llm_requests_total",
    "Model completion requests, by outcome.",
    ["model", "operation", "outcome"],
)
LLM_REQUEST_DURATION = Histogram(
    "codepilot_llm_request_duration_seconds",
    "Model completion duration.",
    ["model", "operation"],
    # Model calls are slow and their tail is what matters, so the buckets run
    # far wider than the HTTP ones.
    buckets=(1, 5, 15, 30, 60, 120, 300, 600, 1200),
)
LLM_TOKENS = Counter(
    "codepilot_llm_tokens_total",
    "Tokens consumed by model completions.",
    ["model", "operation", "kind"],
)

RAG_REQUESTS = Counter(
    "codepilot_rag_requests_total",
    "Calls to the CodeRAG service, by outcome.",
    ["operation", "outcome"],
)
RAG_REQUEST_DURATION = Histogram(
    "codepilot_rag_request_duration_seconds",
    "CodeRAG call duration.",
    ["operation"],
    buckets=(0.1, 0.5, 1, 5, 15, 60, 300),
)

ANALYZER_RUNS = Counter(
    "codepilot_analyzer_runs_total",
    "Static analyzer runs, by outcome.",
    ["tool", "outcome"],
)
ANALYZER_DURATION = Histogram(
    "codepilot_analyzer_duration_seconds",
    "Static analyzer duration.",
    ["tool"],
    buckets=(1, 5, 15, 30, 60, 120, 300),
)

AGENT_TOOL_CALLS = Counter(
    "codepilot_agent_tool_calls_total",
    "Tool calls issued by the agent loop.",
    ["tool"],
)


def metrics_payload() -> tuple[bytes, str]:
    """Return the exposition body and its content type."""
    return generate_latest(), CONTENT_TYPE_LATEST


@contextmanager
def observe_rag(operation: str) -> Iterator[None]:
    started = time.perf_counter()
    outcome = "success"
    try:
        yield
    except Exception:
        outcome = "error"
        raise
    finally:
        RAG_REQUEST_DURATION.labels(operation=operation).observe(time.perf_counter() - started)
        RAG_REQUESTS.labels(operation=operation, outcome=outcome).inc()


@contextmanager
def observe_analyzer(tool: str) -> Iterator[None]:
    started = time.perf_counter()
    outcome = "success"
    try:
        yield
    except Exception:
        outcome = "error"
        raise
    finally:
        ANALYZER_DURATION.labels(tool=tool).observe(time.perf_counter() - started)
        ANALYZER_RUNS.labels(tool=tool, outcome=outcome).inc()


def record_llm_result(
    *, model: str, operation: str, outcome: str, duration_seconds: float,
    prompt_tokens: int = 0, completion_tokens: int = 0,
) -> None:
    model = model or "unknown"
    LLM_REQUESTS.labels(model=model, operation=operation, outcome=outcome).inc()
    LLM_REQUEST_DURATION.labels(model=model, operation=operation).observe(duration_seconds)
    if prompt_tokens:
        LLM_TOKENS.labels(model=model, operation=operation, kind="prompt").inc(prompt_tokens)
    if completion_tokens:
        LLM_TOKENS.labels(model=model, operation=operation, kind="completion").inc(completion_tokens)
