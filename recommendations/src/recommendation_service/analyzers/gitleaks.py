import json

from recommendation_service.analyzers.common import (
    AdapterResult,
    AnalyzerContext,
    clean_text,
    diagnostic,
    normalize_path,
)
from recommendation_service.analyzers.runner import AnalyzerRunner, RepositorySnapshot
from recommendation_service.findings import build_finding
from recommendation_service.models import AnalyzerStatus, Category, Severity
from recommendation_service.settings import AnalyzerSettings


class GitleaksAdapter:
    name = "gitleaks"

    def __init__(self, settings: AnalyzerSettings) -> None:
        self.settings = settings

    def applies(self, context: AnalyzerContext) -> bool:
        return Category.SECURITY in context.categories

    async def run(self, runner: AnalyzerRunner, snapshot: RepositorySnapshot, context: AnalyzerContext) -> AdapterResult:
        report_path = snapshot.artifacts / "gitleaks.json"
        result = await runner.run(
            [
                self.settings.gitleaks_executable,
                "dir",
                ".",
                "--no-banner",
                "--no-color",
                "--redact=100",
                "--report-format",
                "json",
                "--report-path",
                str(report_path),
                "--exit-code",
                "0",
                "--max-target-megabytes",
                "1",
            ],
            snapshot,
        )
        if result.timed_out:
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.TIMED_OUT, "scan timed out", result.duration_ms))
        if result.returncode != 0:
            message = clean_text(result.stderr, 500) or f"scan exited with code {result.returncode}"
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.FAILED, message, result.duration_ms))
        findings = self.parse(runner.read_limited(report_path), snapshot, context)
        return AdapterResult(findings, diagnostic(self.name, AnalyzerStatus.COMPLETED, f"produced {len(findings)} findings", result.duration_ms))

    def parse(self, payload: bytes, snapshot: RepositorySnapshot, context: AnalyzerContext) -> list:
        document = json.loads(payload or b"[]")
        if not isinstance(document, list):
            raise ValueError("Gitleaks report must be a JSON array")
        findings = []
        for item in document:
            if not isinstance(item, dict):
                continue
            path = normalize_path(item.get("File"), snapshot.root, snapshot.paths)
            if not path:
                continue
            start = _positive_int(item.get("StartLine"), 1)
            end = max(start, _positive_int(item.get("EndLine"), start))
            rule_id = clean_text(item.get("RuleID"), 300) or "gitleaks.secret"
            description = clean_text(item.get("Description"), 2_000) or "Potential secret detected."
            findings.append(
                build_finding(
                    repository_id=context.repository.id,
                    commit_sha=context.repository.commit_sha,
                    tool=self.name,
                    rule_id=rule_id,
                    category=Category.SECURITY,
                    severity=Severity.HIGH,
                    file=path,
                    start_line=start,
                    end_line=end,
                    message=description,
                    evidence=f"Secret-like value redacted at {path}:{start}-{end}",
                )
            )
        return findings


def _positive_int(value, default: int) -> int:
    try:
        return max(1, int(value))
    except (TypeError, ValueError):
        return default
