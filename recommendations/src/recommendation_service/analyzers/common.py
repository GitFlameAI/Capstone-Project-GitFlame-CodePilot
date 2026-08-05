import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from recommendation_service.models import (
    AnalyzerDiagnostic,
    AnalyzerStatus,
    Category,
    Finding,
    RepositoryReference,
    Severity,
    TechnologyInventory,
)


@dataclass(frozen=True)
class AnalyzerContext:
    repository: RepositoryReference
    technologies: TechnologyInventory
    categories: frozenset[Category]


@dataclass(frozen=True)
class AdapterResult:
    findings: list[Finding]
    diagnostic: AnalyzerDiagnostic


def diagnostic(
    tool: str,
    status: AnalyzerStatus,
    message: str,
    duration_ms: int = 0,
) -> AnalyzerDiagnostic:
    return AnalyzerDiagnostic(
        tool=tool,
        status=status,
        message=clean_text(message, 1_000) or status.value,
        duration_ms=max(0, duration_ms),
    )


def clean_text(value: Any, limit: int) -> str:
    if isinstance(value, (bytes, bytearray)):
        value = value.decode("utf-8", errors="replace")
    text = re.sub(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", "", str(value or ""))
    return " ".join(text.split())[:limit]


def normalize_path(raw: Any, root: Path, allowed: frozenset[str]) -> str | None:
    value = str(raw or "").replace("\\", "/")
    root_text = str(root).replace("\\", "/").rstrip("/")
    if value.startswith(root_text + "/"):
        value = value[len(root_text) + 1 :]
    value = value.removeprefix("./")
    if value in allowed:
        return value
    return None


def severity_from_text(value: Any, default: Severity = Severity.MEDIUM) -> Severity:
    normalized = str(value or "").strip().lower()
    if any(token in normalized for token in ("critical", "high", "error")):
        return Severity.HIGH
    if any(token in normalized for token in ("medium", "moderate", "warning", "warn")):
        return Severity.MEDIUM
    if any(token in normalized for token in ("low", "info", "informational")):
        return Severity.LOW
    try:
        score = float(normalized)
    except ValueError:
        return default
    if score >= 7:
        return Severity.HIGH
    if score >= 4:
        return Severity.MEDIUM
    return Severity.LOW
