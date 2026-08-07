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
from recommendation_service.models import AnalyzerStatus, Category, Severity
from recommendation_service.settings import AnalyzerSettings


class OsvScannerAdapter:
    name = "osv-scanner"

    def __init__(self, settings: AnalyzerSettings) -> None:
        self.settings = settings

    def applies(self, context: AnalyzerContext) -> bool:
        return Category.SECURITY in context.categories and bool(context.technologies.package_managers)

    async def run(self, runner: AnalyzerRunner, snapshot: RepositorySnapshot, context: AnalyzerContext) -> AdapterResult:
        cache_dir = Path(self.settings.osv_cache_dir) if self.settings.osv_cache_dir else None
        if not cache_dir or not cache_dir.is_dir() or not any(cache_dir.iterdir()):
            return AdapterResult(
                [],
                diagnostic(
                    self.name,
                    AnalyzerStatus.SKIPPED,
                    "offline vulnerability database is unavailable; network fallback is disabled",
                ),
            )

        report_path = snapshot.artifacts / "osv.json"
        result = await runner.run(
            [
                self.settings.osv_executable,
                "scan",
                "source",
                "--format=json",
                "--verbosity=error",
                "--offline",
                "--offline-vulnerabilities",
                "--no-resolve",
                "--recursive",
                "--output-file",
                str(report_path),
                ".",
            ],
            snapshot,
            env={"XDG_CACHE_HOME": str(cache_dir)},
        )
        if result.timed_out:
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.TIMED_OUT, "scan timed out", result.duration_ms))
        if result.returncode not in {0, 1}:
            message = clean_text(result.stderr, 500) or f"scan exited with code {result.returncode}"
            return AdapterResult([], diagnostic(self.name, AnalyzerStatus.FAILED, message, result.duration_ms))
        findings = self.parse(runner.read_limited(report_path), snapshot, context)
        return AdapterResult(findings, diagnostic(self.name, AnalyzerStatus.COMPLETED, f"produced {len(findings)} findings", result.duration_ms))

    def parse(self, payload: bytes, snapshot: RepositorySnapshot, context: AnalyzerContext) -> list:
        document = json.loads(payload or b"{}")
        findings = []
        seen: set[tuple[str, str, str]] = set()
        for result in document.get("results", []):
            if not isinstance(result, dict):
                continue
            source = result.get("source") if isinstance(result.get("source"), dict) else {}
            path = normalize_path(source.get("path"), snapshot.root, snapshot.paths)
            if not path:
                continue
            for package_entry in result.get("packages", []):
                if not isinstance(package_entry, dict):
                    continue
                package = package_entry.get("package") if isinstance(package_entry.get("package"), dict) else {}
                name = clean_text(package.get("name"), 300) or "unknown package"
                version = clean_text(package.get("version"), 100) or "unknown version"
                ecosystem = clean_text(package.get("ecosystem"), 100) or "unknown ecosystem"
                for vulnerability in package_entry.get("vulnerabilities", []):
                    if not isinstance(vulnerability, dict):
                        continue
                    vulnerability_id = clean_text(vulnerability.get("id"), 300)
                    if not vulnerability_id or (path, name, vulnerability_id) in seen:
                        continue
                    seen.add((path, name, vulnerability_id))
                    severity = _osv_severity(vulnerability)
                    summary = clean_text(vulnerability.get("summary"), 1_600)
                    message = summary or f"{name} {version} is affected by {vulnerability_id}."
                    fixed = _fixed_versions(vulnerability)
                    evidence = f"Package {name} {version} ({ecosystem})"
                    if fixed:
                        evidence += f"; fixed versions: {', '.join(fixed[:5])}"
                    findings.append(
                        build_finding(
                            repository_id=context.repository.id,
                            commit_sha=context.repository.commit_sha,
                            tool=self.name,
                            rule_id=vulnerability_id,
                            category=Category.SECURITY,
                            severity=severity,
                            file=path,
                            start_line=1,
                            end_line=1,
                            message=message,
                            evidence=evidence,
                        )
                    )
        return findings


def _osv_severity(vulnerability: dict[str, Any]) -> Severity:
    database_specific = vulnerability.get("database_specific")
    if isinstance(database_specific, dict) and database_specific.get("severity"):
        return severity_from_text(database_specific["severity"])
    for entry in vulnerability.get("severity", []):
        if isinstance(entry, dict):
            score = str(entry.get("score") or "")
            if score and not score.upper().startswith("CVSS:"):
                return severity_from_text(score)
    return Severity.MEDIUM


def _fixed_versions(vulnerability: dict[str, Any]) -> list[str]:
    versions: list[str] = []
    for affected in vulnerability.get("affected", []):
        if not isinstance(affected, dict):
            continue
        for range_item in affected.get("ranges", []):
            if not isinstance(range_item, dict):
                continue
            for event in range_item.get("events", []):
                if isinstance(event, dict) and event.get("fixed"):
                    versions.append(clean_text(event["fixed"], 100))
    return list(dict.fromkeys(version for version in versions if version))
