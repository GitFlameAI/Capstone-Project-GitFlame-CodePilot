import unittest

from pydantic import ValidationError

from recommendation_service.config import ServiceConfig, filter_analysis_files, filter_repo_context
from recommendation_service.detection import detect_technologies
from recommendation_service.findings import build_finding
from recommendation_service.model_client import InferenceMetrics
from recommendation_service.models import (
    AnalyzeRequest,
    AnalyzerReport,
    Category,
    RecommendationResponse,
    RepoFile,
    Severity,
)
from recommendation_service.service import RecommendationService


class RecommendationFoundationTests(unittest.TestCase):
    def test_request_requires_repository_and_commit_sha(self):
        request = AnalyzeRequest.model_validate(
            {
                "repository": {"id": "owner/repository", "commit_sha": "abc123"},
                "config_yaml": "version: 1",
                "repo_context": [{"path": "src/app.py", "content": "print('ok')\n"}],
            }
        )

        self.assertEqual(request.repository.id, "owner/repository")
        self.assertEqual(request.repository.commit_sha, "abc123")
        with self.assertRaises(ValidationError):
            AnalyzeRequest.model_validate(
                {
                    "repository": {"id": "owner/repository", "commit_sha": ""},
                    "config_yaml": "version: 1",
                    "repo_context": [{"path": "src/app.py", "content": ""}],
                }
            )

    def test_full_analysis_snapshot_is_not_limited_by_model_context(self):
        files = [
            RepoFile(path=f"src/file_{index:02d}.py", content="VALUE = 1\n")
            for index in range(25)
        ]
        config = ServiceConfig.model_validate({"rag": {"max_files": 4}})

        self.assertEqual(len(filter_analysis_files(files, config)), 25)
        self.assertEqual(len(filter_repo_context(files, config)), 4)

    def test_detects_languages_manifests_and_candidate_analyzers(self):
        inventory = detect_technologies(
            [
                RepoFile(path="backend/go.mod", content="module example\n"),
                RepoFile(path="backend/main.go", content="package main\n"),
                RepoFile(path="service/pyproject.toml", content="[project]\n"),
                RepoFile(path="service/app.py", content="print('ok')\n"),
                RepoFile(path="frontend/package.json", content="{}\n"),
                RepoFile(path="frontend/src/app.ts", content="export {}\n"),
            ]
        )

        self.assertEqual(
            inventory.languages,
            ["go", "javascript", "python", "typescript"],
        )
        self.assertEqual(
            inventory.package_managers,
            ["go_modules", "npm", "python"],
        )
        self.assertTrue(
            {"eslint", "gitleaks", "gosec", "osv-scanner", "pmd-cpd", "ruff", "semgrep", "staticcheck"}
            <= set(inventory.analyzer_candidates)
        )

    def test_finding_fingerprint_is_stable_across_commit_revisions(self):
        common = {
            "repository_id": "owner/repository",
            "tool": "semgrep",
            "rule_id": "python.security.shell-true",
            "category": Category.SECURITY,
            "severity": Severity.HIGH,
            "file": "src/commands.py",
            "start_line": 42,
            "end_line": 42,
            "message": "Shell invocation accepts untrusted input.",
        }

        first = build_finding(commit_sha="abc123", **common)
        second = build_finding(commit_sha="def456", **common)

        self.assertEqual(len(first.fingerprint), 64)
        self.assertEqual(first.fingerprint, second.fingerprint)
        self.assertNotEqual(first.commit_sha, second.commit_sha)


class FakeRecommendationModel:
    def __init__(self) -> None:
        self.user_prompt = ""
        self.called = False

    async def analyze(self, *, system_prompt, user_prompt, response_schema):
        self.called = True
        self.user_prompt = user_prompt
        return RecommendationResponse(
            summary="No deterministic analyzers executed yet.",
            recommendations=[],
        ), InferenceMetrics()


class EmptyAnalyzerOrchestrator:
    async def analyze(self, repository, files, technologies, categories):
        return AnalyzerReport()


class RecommendationServiceFoundationTests(unittest.IsolatedAsyncioTestCase):
    async def test_service_skips_model_when_analyzers_find_nothing(self):
        model = FakeRecommendationModel()
        service = RecommendationService(
            model,
            analyzer_orchestrator=EmptyAnalyzerOrchestrator(),
        )
        request = AnalyzeRequest.model_validate(
            {
                "repository": {"id": "owner/repository", "commit_sha": "abc123"},
                "config_yaml": "version: 1",
                "repo_context": [
                    {"path": "go.mod", "content": "module example\n"},
                    {"path": "main.go", "content": "package main\n"},
                ],
            }
        )

        response, _ = await service.analyze(request)

        self.assertEqual(response.recommendations, [])
        self.assertFalse(model.called)
        self.assertEqual(model.user_prompt, "")
        self.assertIn("no supported issues", response.summary)


if __name__ == "__main__":
    unittest.main()
