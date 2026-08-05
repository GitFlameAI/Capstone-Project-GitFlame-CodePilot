import json
from pathlib import Path
from typing import Any

from recommendation_service.analyzers.common import (
    AdapterResult,
    AnalyzerContext,
    clean_text,
    diagnostic,
    normalize_path,
    severity_from_text,
)
from recommendation_service.analyzers.runner import AnalyzerRunner, RepositorySnapshot
from recommendation_service.findings import build_finding
from recommendation_service.models import AnalyzerStatus, Category
from recommendation_service.settings import AnalyzerSettings


class SemgrepAdapter:
    name = "semgrep"

    def __init__(self, settings: AnalyzerSettings) -> None:
        self.settings = settings

    def applies(self, context: AnalyzerContext) -> bool:
        return bool(
            context.categories
            & {Category.SECURITY, Category.MAINTAINABILITY, Category.PERFORMANCE}
        )

    async def run(
        self,
        runner: AnalyzerRunner,
        snapshot: RepositorySnapshot,
        context: AnalyzerContext,
    ) -> AdapterResult:
        rules = Path(self.settings.semgrep_rules_path) if self.settings.semgrep_rules_path else (
            Path(__file__).with_name("rules") / "semgrep.yml"
        )
        if not rules.is_file():
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.SKIPPED, "local rules are unavailable"))

        report_path = snapshot.artifacts / "semgrep.json"
        result = await runner.run(
            [
                self.settings.semgrep_executable,
                "scan",
                "--config",
                str(rules),
                "--json",
                "--output",
                str(report_path),
                "--metrics",
                "off",
                "--disable-version-check",
                "--jobs",
                "1",
                "--timeout",
                str(max(1, int(self.settings.timeout_seconds / 2))),
                "--max-memory",
                str(self.settings.memory_limit_mb),
                "--no-git-ignore",
                ".",
            ],
            snapshot,
        )
        if result.timed_out:
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.TIMED_OUT, "scan timed out", result.duration_ms))
        if result.returncode != 0:
            message = clean_text(result.stderr, 500) or f"scan exited with code {result.returncode}"
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.FAILED, message, result.duration_ms))

        findings = self.parse(runner.read_limited(report_path), snapshot, context)
        return AdapterResult(
            findings,
            diagnostic(self.name, AnalyzerStatus.COMPLETED, f"produced {len(findings)} findings", result.duration_ms),
        )

    def parse(
        self,
        payload: bytes,
        snapshot: RepositorySnapshot,
        context: AnalyzerContext,
    ) -> list:
        document = json.loads(payload or b"{}")
        findings = []
        for item in document.get("results", []):
            if not isinstance(item, dict):
                continue
            path = normalize_path(item.get("path"), snapshot.root, snapshot.paths)
            if not path:
                continue
            extra = item.get("extra") if isinstance(item.get("extra"), dict) else {}
            metadata = extra.get("metadata") if isinstance(extra.get("metadata"), dict) else {}
            category = _category(metadata.get("category"), item.get("check_id"))
            if category not in context.categories:
                continue
            start = _line(item.get("start"), 1)
            end = max(start, _line(item.get("end"), start))
            rule_id = clean_text(item.get("check_id"), 300) or "semgrep.unknown"
            message = clean_text(extra.get("message"), 2_000) or f"Semgrep rule {rule_id} matched."
            findings.append(
                build_finding(
                    repository_id=context.repository.id,
                    commit_sha=context.repository.commit_sha,
                    tool=self.name,
                    rule_id=rule_id,
                    category=category,
                    severity=severity_from_text(extra.get("severity")),
                    file=path,
                    start_line=start,
                    end_line=end,
                    message=message,
                    evidence=f"Semgrep match at {path}:{start}-{end}",
                )
            )
        return findings


def _line(value: Any, default: int) -> int:
    if isinstance(value, dict):
        value = value.get("line")
    try:
        return max(1, int(value))
    except (TypeError, ValueError):
        return default


def _category(value: Any, rule_id: Any) -> Category:
    normalized = str(value or "").strip().lower().replace("-", "_")
    if normalized in {category.value for category in Category}:
        return Category(normalized)
    rule = str(rule_id or "").lower()
    if "performance" in rule:
        return Category.PERFORMANCE
    if any(token in rule for token in ("security", "injection", "secret", "unsafe")):
        return Category.SECURITY
    return Category.MAINTAINABILITY
