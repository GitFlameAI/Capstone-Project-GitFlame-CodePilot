import logging

from observability.metrics import observe_analyzer
from recommendation_service.analyzers.common import AnalyzerContext, diagnostic
from recommendation_service.analyzers.cpd import CpdAdapter
from recommendation_service.analyzers.gitleaks import GitleaksAdapter
from recommendation_service.analyzers.osv import OsvScannerAdapter
from recommendation_service.analyzers.runner import AnalyzerRunner, SnapshotLimitError
from recommendation_service.analyzers.semgrep import SemgrepAdapter
from recommendation_service.models import (
    AnalyzerReport,
    AnalyzerStatus,
    Category,
    RepoFile,
    RepositoryReference,
    TechnologyInventory,
)
from recommendation_service.settings import AnalyzerSettings

logger = logging.getLogger(__name__)


class AnalyzerOrchestrator:
    def __init__(
        self,
        settings: AnalyzerSettings | None = None,
        *,
        runner: AnalyzerRunner | None = None,
        adapters: list | None = None,
    ) -> None:
        self.settings = settings or AnalyzerSettings.from_env()
        self.runner = runner or AnalyzerRunner(self.settings)
        self.adapters = adapters or [
            SemgrepAdapter(self.settings),
            GitleaksAdapter(self.settings),
            OsvScannerAdapter(self.settings),
            CpdAdapter(self.settings),
        ]

    async def analyze(
        self,
        repository: RepositoryReference,
        files: list[RepoFile],
        technologies: TechnologyInventory,
        categories: list[Category],
    ) -> AnalyzerReport:
        if not self.settings.enabled:
            return AnalyzerReport(
                diagnostics=[
                    diagnostic("analyzers", AnalyzerStatus.SKIPPED, "analyzers are disabled")
                ]
            )

        context = AnalyzerContext(
            repository=repository,
            technologies=technologies,
            categories=frozenset(categories),
        )
        findings = []
        diagnostics = []
        try:
            with self.runner.snapshot(files) as snapshot:
                for adapter in self.adapters:
                    if not adapter.applies(context):
                        diagnostics.append(
                            diagnostic(adapter.name, AnalyzerStatus.SKIPPED, "not applicable to selected categories or technologies")
                        )
                        continue
                    try:
                        with observe_analyzer(adapter.name):
                            result = await adapter.run(self.runner, snapshot, context)
                    except FileNotFoundError:
                        result = None
                        diagnostics.append(
                            diagnostic(adapter.name, AnalyzerStatus.SKIPPED, "executable is unavailable")
                        )
                    except Exception as exc:
                        logger.exception("analyzer failed tool=%s", adapter.name)
                        result = None
                        diagnostics.append(
                            diagnostic(adapter.name, AnalyzerStatus.FAILED, f"invalid or failed analyzer output: {exc}")
                        )
                    if result is not None:
                        findings.extend(result.findings)
                        diagnostics.append(result.diagnostic)
        except SnapshotLimitError as exc:
            diagnostics.append(diagnostic("analyzers", AnalyzerStatus.FAILED, str(exc)))

        unique = {finding.fingerprint: finding for finding in findings}
        return AnalyzerReport(findings=list(unique.values()), diagnostics=diagnostics)
