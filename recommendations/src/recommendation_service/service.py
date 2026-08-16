import logging

from recommendation_service.analyzers import AnalyzerOrchestrator
from recommendation_service.config import (
    ConfigError,
    filter_analysis_files,
    parse_config,
)
from recommendation_service.detection import detect_technologies
from recommendation_service.model_client import (
    InferenceMetrics,
    ModelOutputError,
    RecommendationModelClient,
)
from recommendation_service.models import (
    SEVERITY_RANK,
    AnalyzeRequest,
    AnalyzerStatus,
    Finding,
    RepoFile,
    RecommendationResponse,
)
from recommendation_service.prompt import SYSTEM_PROMPT, build_analysis_prompt

logger = logging.getLogger(__name__)


class RecommendationService:
    def __init__(
        self,
        model_client: RecommendationModelClient,
        analyzer_orchestrator: AnalyzerOrchestrator | None = None,
    ) -> None:
        self.model_client = model_client
        self.analyzer_orchestrator = analyzer_orchestrator or AnalyzerOrchestrator()

    async def analyze(
        self, request: AnalyzeRequest
    ) -> tuple[RecommendationResponse, InferenceMetrics]:
        config = parse_config(request.config_yaml)
        analysis_files = filter_analysis_files(request.repo_context, config)
        technologies = detect_technologies(analysis_files)
        analyzer_report = await self.analyzer_orchestrator.analyze(
            request.repository,
            analysis_files,
            technologies,
            config.recommendations.categories,
        )
        files = _select_prompt_files(
            analysis_files,
            analyzer_report.findings,
            config.rag.max_files,
        )
        files = _redact_secret_lines(files, analyzer_report.findings)
        prompt_paths = {file.path for file in files}
        prompt_findings = [
            finding for finding in analyzer_report.findings if finding.file in prompt_paths
        ][:100]
        logger.info(
            "recommendation analysis repository_id=%s commit_sha=%s snapshot_files=%d prompt_files=%d languages=%s findings=%d analyzer_statuses=%s",
            request.repository.id,
            request.repository.commit_sha,
            len(analysis_files),
            len(files),
            ",".join(technologies.languages) or "unknown",
            len(analyzer_report.findings),
            ",".join(
                f"{item.tool}:{item.status.value}" for item in analyzer_report.diagnostics
            ),
        )
        failed_analyzers = [
            item
            for item in analyzer_report.diagnostics
            if item.status in {AnalyzerStatus.FAILED, AnalyzerStatus.TIMED_OUT}
        ]
        if failed_analyzers:
            failures = ", ".join(
                f"{item.tool}: {item.message}" for item in failed_analyzers
            )
            raise ModelOutputError(
                f"recommendation analysis is incomplete because analyzers failed: {failures}"
            )
        if not prompt_findings:
            logger.info(
                "recommendation analysis completed without findings repository_id=%s commit_sha=%s",
                request.repository.id,
                request.repository.commit_sha,
            )
            return RecommendationResponse(
                summary="Static analyzers found no supported issues for the selected categories.",
                recommendations=[],
            ), InferenceMetrics()

        schema = RecommendationResponse.model_json_schema()
        prompt = build_analysis_prompt(
            files,
            config,
            schema,
            request.repository,
            technologies,
            prompt_findings,
        )
        response, metrics = await self.model_client.analyze(
            system_prompt=SYSTEM_PROMPT,
            user_prompt=prompt,
            response_schema=schema,
        )

        file_lines = {
            file.path: max(1, len(file.content.splitlines()))
            for file in files
        }
        allowed_categories = set(config.recommendations.categories)
        minimum_severity = SEVERITY_RANK[config.recommendations.severity_threshold]
        filtered = []
        seen_fingerprints: set[str] = set()
        findings_by_fingerprint = {
            finding.fingerprint: finding for finding in prompt_findings
        }
        duplicate_count = 0
        for recommendation in response.recommendations:
            finding = findings_by_fingerprint.get(recommendation.finding_fingerprint)
            if finding is None:
                raise ModelOutputError(
                    "model referenced an unknown analyzer finding fingerprint: "
                    f"{recommendation.finding_fingerprint}"
                )
            if recommendation.file not in file_lines:
                raise ModelOutputError(
                    f"model referenced an unknown or excluded file: {recommendation.file}"
                )
            if recommendation.line > file_lines[recommendation.file]:
                raise ModelOutputError(
                    f"model referenced invalid line {recommendation.line} in {recommendation.file}"
                )
            if recommendation.category not in allowed_categories:
                raise ModelOutputError(
                    f"model returned disallowed category: {recommendation.category.value}"
                )
            if not (
                finding.file == recommendation.file
                and finding.category == recommendation.category
                and finding.start_line <= recommendation.line <= finding.end_line
            ):
                raise ModelOutputError(
                    "model recommendation does not match its analyzer finding: "
                    f"{recommendation.finding_fingerprint}"
                )
            if finding.severity != recommendation.severity:
                raise ModelOutputError(
                    "model changed analyzer severity for recommendation at "
                    f"{recommendation.file}:{recommendation.line}"
                )
            if recommendation.finding_fingerprint in seen_fingerprints:
                duplicate_count += 1
                continue
            seen_fingerprints.add(recommendation.finding_fingerprint)
            if SEVERITY_RANK[recommendation.severity] >= minimum_severity:
                filtered.append(recommendation)

        if duplicate_count:
            logger.warning(
                "deduplicated model recommendations repository_id=%s commit_sha=%s duplicates=%d",
                request.repository.id,
                request.repository.commit_sha,
                duplicate_count,
            )

        return RecommendationResponse(summary=response.summary, recommendations=filtered), metrics


def _select_prompt_files(
    files: list[RepoFile], findings: list[Finding], limit: int
) -> list[RepoFile]:
    by_path = {file.path: file for file in files}
    selected_paths: list[str] = []
    for finding in findings:
        for path in (finding.file, *finding.related_files):
            if path in by_path and path not in selected_paths:
                selected_paths.append(path)
    for file in files:
        if file.path not in selected_paths:
            selected_paths.append(file.path)
    return [by_path[path] for path in selected_paths[:limit]]


def _redact_secret_lines(files: list[RepoFile], findings: list[Finding]) -> list[RepoFile]:
    ranges: dict[str, list[tuple[int, int]]] = {}
    for finding in findings:
        if finding.tool == "gitleaks":
            ranges.setdefault(finding.file, []).append(
                (finding.start_line, finding.end_line)
            )
    if not ranges:
        return files

    redacted = []
    for file in files:
        file_ranges = ranges.get(file.path)
        if not file_ranges:
            redacted.append(file)
            continue
        lines = file.content.splitlines(keepends=True)
        for start, end in file_ranges:
            for index in range(max(0, start - 1), min(len(lines), end)):
                newline = "\n" if lines[index].endswith("\n") else ""
                lines[index] = "[REDACTED SECRET]" + newline
        redacted.append(RepoFile(path=file.path, content="".join(lines)))
    return redacted


__all__ = ["ConfigError", "RecommendationService"]
