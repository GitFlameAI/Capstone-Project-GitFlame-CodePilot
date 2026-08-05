import json
import os
import unittest

from recommendation_service.analyzers.common import AnalyzerContext
from recommendation_service.analyzers.cpd import CpdAdapter
from recommendation_service.analyzers.gitleaks import GitleaksAdapter
from recommendation_service.analyzers.orchestrator import AnalyzerOrchestrator
from recommendation_service.analyzers.osv import OsvScannerAdapter
from recommendation_service.analyzers.runner import AnalyzerRunner
from recommendation_service.analyzers.semgrep import SemgrepAdapter
from recommendation_service.findings import build_finding
from recommendation_service.model_client import InferenceMetrics
from recommendation_service.models import (
    AnalyzeRequest,
    AnalyzerReport,
    AnalyzerStatus,
    Category,
    Recommendation,
    RecommendationResponse,
    RepoFile,
    RepositoryReference,
    Severity,
    TechnologyInventory,
)
from recommendation_service.service import RecommendationService
from recommendation_service.settings import AnalyzerSettings


class AnalyzerParserTests(unittest.TestCase):
    def setUp(self):
        self.settings = AnalyzerSettings()
        self.runner = AnalyzerRunner(self.settings)
        self.files = [
            RepoFile(path="src/app.py", content="print('ok')\n"),
            RepoFile(path="src/copy.py", content="print('ok')\n"),
            RepoFile(path="requirements.txt", content="demo==1.0\n"),
            RepoFile(path=".env", content="TOKEN=secret\n"),
        ]
        self.context = AnalyzerContext(
            repository=RepositoryReference(id="owner/repo", commit_sha="abc123"),
            technologies=TechnologyInventory(
                languages=["python"],
                manifests=["requirements.txt"],
                package_managers=["pip"],
            ),
            categories=frozenset(Category),
        )

    def test_semgrep_json_becomes_finding(self):
        with self.runner.snapshot(self.files) as snapshot:
            payload = json.dumps(
                {
                    "results": [
                        {
                            "check_id": "codepilot.python.security.test",
                            "path": "src/app.py",
                            "start": {"line": 1},
                            "end": {"line": 1},
                            "extra": {
                                "message": "Unsafe call.",
                                "severity": "ERROR",
                                "metadata": {"category": "security"},
                            },
                        }
                    ]
                }
            ).encode()
            findings = SemgrepAdapter(self.settings).parse(payload, snapshot, self.context)

        self.assertEqual(len(findings), 1)
        self.assertEqual(findings[0].category, Category.SECURITY)
        self.assertEqual(findings[0].file, "src/app.py")
        self.assertEqual(findings[0].commit_sha, "abc123")

    def test_gitleaks_never_preserves_secret_or_match(self):
        with self.runner.snapshot(self.files) as snapshot:
            payload = json.dumps(
                [
                    {
                        "RuleID": "generic-api-key",
                        "Description": "Generic API key",
                        "File": ".env",
                        "StartLine": 1,
                        "EndLine": 1,
                        "Secret": "do-not-leak-this",
                        "Match": "TOKEN=do-not-leak-this",
                    }
                ]
            ).encode()
            findings = GitleaksAdapter(self.settings).parse(payload, snapshot, self.context)

        serialized = findings[0].model_dump_json()
        self.assertNotIn("do-not-leak-this", serialized)
        self.assertIn("redacted", findings[0].evidence.lower())

    def test_osv_json_becomes_dependency_finding(self):
        with self.runner.snapshot(self.files) as snapshot:
            payload = json.dumps(
                {
                    "results": [
                        {
                            "source": {"path": str(snapshot.root / "requirements.txt")},
                            "packages": [
                                {
                                    "package": {
                                        "name": "demo",
                                        "version": "1.0",
                                        "ecosystem": "PyPI",
                                    },
                                    "vulnerabilities": [
                                        {
                                            "id": "OSV-TEST-1",
                                            "summary": "Known vulnerable dependency.",
                                            "database_specific": {"severity": "HIGH"},
                                            "affected": [
                                                {
                                                    "ranges": [
                                                        {"events": [{"fixed": "1.1"}]}
                                                    ]
                                                }
                                            ],
                                        }
                                    ],
                                }
                            ],
                        }
                    ]
                }
            ).encode()
            findings = OsvScannerAdapter(self.settings).parse(payload, snapshot, self.context)

        self.assertEqual(findings[0].rule_id, "OSV-TEST-1")
        self.assertIn("fixed versions: 1.1", findings[0].evidence)

    def test_cpd_xml_preserves_related_files(self):
        with self.runner.snapshot(self.files) as snapshot:
            payload = f"""<?xml version="1.0"?>
            <pmd-cpd>
              <duplication lines="8" tokens="80">
                <file line="2" endline="9" path="{snapshot.root / 'src/app.py'}" />
                <file line="4" endline="11" path="{snapshot.root / 'src/copy.py'}" />
              </duplication>
            </pmd-cpd>""".encode()
            findings = CpdAdapter(self.settings).parse(
                payload, snapshot, self.context, "python"
            )

        self.assertEqual(findings[0].file, "src/app.py")
        self.assertEqual(findings[0].related_files, ["src/copy.py"])
        self.assertEqual(findings[0].category, Category.CODE_DUPLICATION)


class AnalyzerRunnerTests(unittest.IsolatedAsyncioTestCase):
    async def test_analyzer_control_files_are_not_staged(self):
        runner = AnalyzerRunner(AnalyzerSettings())
        files = [
            RepoFile(path="app.py", content="print('ok')\n"),
            RepoFile(path=".gitleaks.toml", content="[allowlist]\n"),
            RepoFile(path="nested/.semgrepignore", content="*\n"),
        ]
        with runner.snapshot(files) as snapshot:
            self.assertTrue((snapshot.root / "app.py").is_file())
            self.assertFalse((snapshot.root / ".gitleaks.toml").exists())
            self.assertFalse((snapshot.root / "nested/.semgrepignore").exists())
            self.assertEqual(snapshot.paths, frozenset({"app.py"}))

    async def test_osv_has_no_network_fallback_without_offline_database(self):
        settings = AnalyzerSettings(osv_cache_dir=None)
        runner = AnalyzerRunner(settings)
        context = AnalyzerContext(
            repository=RepositoryReference(id="owner/repo", commit_sha="abc123"),
            technologies=TechnologyInventory(
                manifests=["requirements.txt"], package_managers=["pip"]
            ),
            categories=frozenset({Category.SECURITY}),
        )
        with runner.snapshot(
            [RepoFile(path="requirements.txt", content="demo==1.0\n")]
        ) as snapshot:
            result = await OsvScannerAdapter(settings).run(
                runner, snapshot, context
            )

        self.assertEqual(result.diagnostic.status, AnalyzerStatus.SKIPPED)
        self.assertIn("network fallback is disabled", result.diagnostic.message)

    async def test_unsafe_repository_paths_are_rejected(self):
        for path in (".", "folder//file.py", "folder/./file.py", "bad\x00.py"):
            with self.subTest(path=path):
                with self.assertRaises(ValueError):
                    RepoFile(path=path, content="")

    async def test_arguments_are_not_interpreted_by_a_shell(self):
        settings = AnalyzerSettings(timeout_seconds=5, cpu_seconds=5)
        runner = AnalyzerRunner(settings)
        marker = "should-not-exist"
        with runner.snapshot([RepoFile(path="app.py", content="print('ok')\n")]) as snapshot:
            result = await runner.run(
                ["/usr/bin/printf", "%s", f"$(touch {marker})"], snapshot
            )
            self.assertFalse((snapshot.root / marker).exists())

        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.decode(), f"$(touch {marker})")

    async def test_environment_overrides_are_allowlisted(self):
        runner = AnalyzerRunner(AnalyzerSettings())
        with runner.snapshot([RepoFile(path="app.py", content="")]) as snapshot:
            with self.assertRaisesRegex(ValueError, "unsupported analyzer environment"):
                await runner.run(
                    ["/usr/bin/true"],
                    snapshot,
                    env={"PATH": "/untrusted"},
                )

    async def test_timeout_terminates_process_group(self):
        settings = AnalyzerSettings(timeout_seconds=0.05, cpu_seconds=2)
        runner = AnalyzerRunner(settings)
        with runner.snapshot([RepoFile(path="app.py", content="")]) as snapshot:
            result = await runner.run(["/bin/sleep", "2"], snapshot)

        self.assertTrue(result.timed_out)
        self.assertIsNotNone(result.returncode)


@unittest.skipUnless(
    os.getenv("RUN_REAL_ANALYZERS") == "1",
    "real analyzer smoke test is opt-in",
)
class RealAnalyzerSmokeTests(unittest.IsolatedAsyncioTestCase):
    async def test_semgrep_gitleaks_and_cpd_execute_through_runner(self):
        duplicate = "\n".join(
            f"value_{index} = source_{index} + {index}" for index in range(60)
        )
        files = [
            RepoFile(
                path="unsafe.py",
                content=(
                    "import subprocess\n"
                    "subprocess.run(user_input, shell=True)\n"
                    'api_key = "N7q2vL9xP4mR8sT1wY6aB3cD5eF0gH2j"\n'
                ),
            ),
            RepoFile(path="copy_one.py", content=duplicate + "\n"),
            RepoFile(path="copy_two.py", content=duplicate + "\n"),
        ]
        report = await AnalyzerOrchestrator(AnalyzerSettings(timeout_seconds=30)).analyze(
            RepositoryReference(id="owner/repo", commit_sha="abc123"),
            files,
            TechnologyInventory(languages=["python"]),
            [Category.SECURITY, Category.MAINTAINABILITY, Category.CODE_DUPLICATION],
        )

        statuses = {item.tool: item.status.value for item in report.diagnostics}
        details = report.model_dump_json()
        self.assertEqual(statuses["semgrep"], "completed", details)
        self.assertEqual(statuses["gitleaks"], "completed", details)
        self.assertEqual(statuses["pmd-cpd"], "completed", details)
        tools = {finding.tool for finding in report.findings}
        self.assertIn("semgrep", tools)
        self.assertIn("gitleaks", tools)
        self.assertIn("pmd-cpd", tools)


class FakeAnalyzerOrchestrator:
    async def analyze(self, repository, files, technologies, categories):
        return AnalyzerReport(
            findings=[
                build_finding(
                    repository_id=repository.id,
                    commit_sha=repository.commit_sha,
                    tool="gitleaks",
                    rule_id="generic-api-key",
                    category=Category.SECURITY,
                    severity=Severity.HIGH,
                    file=".env",
                    start_line=1,
                    end_line=1,
                    message="Potential API key detected.",
                    evidence="Secret-like value redacted at .env:1-1",
                )
            ]
        )


class GroundedRecommendationModel:
    def __init__(self):
        self.prompt = ""

    async def analyze(self, *, system_prompt, user_prompt, response_schema):
        self.prompt = user_prompt
        return (
            RecommendationResponse(
                summary="One deterministic security finding.",
                recommendations=[
                    Recommendation(
                        severity=Severity.HIGH,
                        category=Category.SECURITY,
                        file=".env",
                        line=1,
                        problem="A credential is stored in the repository.",
                        suggestion="Rotate it and load it from a secret store.",
                        confidence=1.0,
                    )
                ],
            ),
            InferenceMetrics(),
        )


class AnalyzerRecommendationIntegrationTests(unittest.IsolatedAsyncioTestCase):
    async def test_secret_is_redacted_and_finding_grounds_recommendation(self):
        model = GroundedRecommendationModel()
        service = RecommendationService(
            model,
            analyzer_orchestrator=FakeAnalyzerOrchestrator(),
        )
        request = AnalyzeRequest.model_validate(
            {
                "repository": {"id": "owner/repo", "commit_sha": "abc123"},
                "config_yaml": "version: 1\nrecommendations:\n  categories: [security]\n",
                "repo_context": [
                    {"path": ".env", "content": "TOKEN=do-not-send-this\n"}
                ],
            }
        )

        response, _ = await service.analyze(request)

        self.assertEqual(len(response.recommendations), 1)
        self.assertIn("[REDACTED SECRET]", model.prompt)
        self.assertNotIn("do-not-send-this", model.prompt)
        self.assertIn("generic-api-key", model.prompt)


if __name__ == "__main__":
    unittest.main()
