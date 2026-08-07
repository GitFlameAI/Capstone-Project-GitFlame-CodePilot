import xml.etree.ElementTree as ET

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


CPD_LANGUAGES = {
    "c": "cpp",
    "cpp": "cpp",
    "csharp": "cs",
    "go": "go",
    "java": "java",
    "javascript": "ecmascript",
    "kotlin": "kotlin",
    "php": "php",
    "python": "python",
    "ruby": "ruby",
    "scala": "scala",
    "swift": "swift",
    "typescript": "typescript",
}


class CpdAdapter:
    name = "pmd-cpd"

    def __init__(self, settings: AnalyzerSettings) -> None:
        self.settings = settings

    def applies(self, context: AnalyzerContext) -> bool:
        return Category.CODE_DUPLICATION in context.categories and any(
            language in CPD_LANGUAGES for language in context.technologies.languages
        )

    async def run(self, runner: AnalyzerRunner, snapshot: RepositorySnapshot, context: AnalyzerContext) -> AdapterResult:
        languages = list(
            dict.fromkeys(
                CPD_LANGUAGES[language]
                for language in context.technologies.languages
                if language in CPD_LANGUAGES
            )
        )
        findings = []
        failures = []
        duration_ms = 0
        for language in languages:
            report_path = snapshot.artifacts / f"cpd-{language}.xml"
            result = await runner.run(
                [
                    self.settings.cpd_executable,
                    "cpd",
                    "--minimum-tokens",
                    str(self.settings.cpd_minimum_tokens),
                    "--dir",
                    ".",
                    "--language",
                    language,
                    "--format",
                    "xml",
                    "--report-file",
                    str(report_path),
                    "--no-fail-on-violation",
                    "--no-fail-on-error",
                    "--skip-duplicate-files",
                ],
                snapshot,
                env={"PMD_JAVA_OPTS": "-Xms64m -Xmx512m -XX:+UseSerialGC"},
            )
            duration_ms += result.duration_ms
            if result.timed_out:
                failures.append(f"{language}: timed out")
                continue
            if result.returncode != 0:
                failures.append(
                    f"{language}: {clean_text(result.stderr, 200) or f'exit {result.returncode}'}"
                )
                continue
            findings.extend(self.parse(runner.read_limited(report_path), snapshot, context, language))

        if failures and not findings:
            status = AnalyzerStatus.TIMED_OUT if all("timed out" in item for item in failures) else AnalyzerStatus.FAILED
            return AdapterResult([], diagnostic(self.name, status, "; ".join(failures), duration_ms))
        message = f"produced {len(findings)} findings across {len(languages)} languages"
        if failures:
            message += f"; partial failures: {'; '.join(failures)}"
        return AdapterResult(findings, diagnostic(self.name, AnalyzerStatus.COMPLETED, message, duration_ms))

    def parse(self, payload: bytes, snapshot: RepositorySnapshot, context: AnalyzerContext, language: str) -> list:
        if not payload.strip():
            return []
        root = ET.fromstring(payload)
        findings = []
        for duplication in root.iter():
            if _local_name(duplication.tag) != "duplication":
                continue
            locations = []
            for file_node in duplication:
                if _local_name(file_node.tag) != "file":
                    continue
                path = normalize_path(file_node.attrib.get("path"), snapshot.root, snapshot.paths)
                if not path:
                    continue
                start = _positive_int(file_node.attrib.get("line"), 1)
                configured_lines = _positive_int(duplication.attrib.get("lines"), 1)
                end = _positive_int(file_node.attrib.get("endline"), start + configured_lines - 1)
                locations.append((path, start, max(start, end)))
            if len(locations) < 2:
                continue
            primary = locations[0]
            related = [path for path, _, _ in locations[1:]]
            tokens = _positive_int(duplication.attrib.get("tokens"), self.settings.cpd_minimum_tokens)
            lines = _positive_int(duplication.attrib.get("lines"), primary[2] - primary[1] + 1)
            findings.append(
                build_finding(
                    repository_id=context.repository.id,
                    commit_sha=context.repository.commit_sha,
                    tool=self.name,
                    rule_id=f"cpd.{language}.duplicate-block",
                    category=Category.CODE_DUPLICATION,
                    severity=Severity.MEDIUM,
                    file=primary[0],
                    start_line=primary[1],
                    end_line=primary[2],
                    message=f"Duplicated block spans {lines} lines and {tokens} tokens.",
                    evidence="Also present in: " + ", ".join(related),
                    related_files=related,
                )
            )
        return findings


def _local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def _positive_int(value, default: int) -> int:
    try:
        return max(1, int(value))
    except (TypeError, ValueError):
        return max(1, default)
