import unittest

from agent_engine.errors import RagUnavailableError
from agent_engine.context import ContextCompressor
from agent_engine.llm_client import ChatCompletion
from agent_engine.models import (
    GeneratePlanRequest,
    PlanConfiguration,
    RagResult,
    RepositoryFile,
)
from agent_engine.repository import ProvidedFilesRepositorySource
from agent_engine.rag import MAX_RAG_QUERY_CHARS, build_issue_search_query
from agent_engine.service import AgentEngineService
from agent_engine.settings import AgentSettings
from agent_engine.tools import ToolSandbox


def _plan(path: str) -> str:
    return f"""# Implementation Plan

## Issue Summary
Add expiration validation to authentication tokens.

## Goal
Reject expired tokens before authentication succeeds.

## Relevant Files
- `{path}`: Contains token validation behavior.

## Proposed Changes
- Add an expiration check to the existing validation path.

## Implementation Steps
1. Inspect the current token parsing and validation sequence.
2. Validate expiration before returning an authenticated identity.

## Expected Files to Change
- `{path}`: Modify the token validation flow.

## Tests and Verification
- Verify valid tokens pass and expired tokens fail.

## Risks and Open Questions
- Confirm the expected clock-skew policy.
"""


class FakeChatClient:
    def __init__(self, plan: str) -> None:
        self.plan = plan
        self.messages = []

    async def ready(self) -> bool:
        return True

    async def complete(self, *, messages, tools, **_kwargs):
        self.messages.append(messages)
        return ChatCompletion(content=self.plan, reasoning="", model="test-model")


class FakeRag:
    def __init__(self, results: list[RagResult] | None = None, *, unavailable=False) -> None:
        self.results = results or []
        self.unavailable = unavailable
        self.calls = []

    async def ready(self) -> bool:
        return not self.unavailable

    async def search(self, *, query, top_k, filters=None):
        self.calls.append({"query": query, "top_k": top_k, "filters": filters})
        if self.unavailable:
            raise RagUnavailableError("test RAG unavailable")
        return self.results[:top_k]


def _request() -> GeneratePlanRequest:
    return GeneratePlanRequest.model_validate(
        {
            "request_id": "task-1",
            "issue": {
                "id": "issue-7",
                "title": "Validate token expiration",
                "body": "Reject expired access tokens before authentication succeeds.",
            },
            "repository": {
                "id": "owner/repository",
                "default_branch": "main",
                "commit_sha": "abc123",
            },
            "configuration_yaml": "rag:\n  max_files: 1\n  max_snippets_per_file: 2\n",
            "repository_files": [
                {"path": "src/first.py", "content": "FIRST = True\n"},
                {"path": "src/auth.py", "content": "def validate(token): pass\n"},
            ],
        }
    )


class IssueRetrievalTests(unittest.IsolatedAsyncioTestCase):
    async def test_issue_context_is_retrieved_before_first_model_call(self):
        result = RagResult(
            path="src/auth.py",
            start_line=1,
            end_line=1,
            score=0.91,
            content="def validate(token): pass",
        )
        rag = FakeRag([result])
        chat = FakeChatClient(_plan("src/auth.py"))
        service = AgentEngineService(
            AgentSettings(
                issue_retrieval_candidate_limit=8,
                issue_retrieval_top_p=0.85,
            ),
            model_client=chat,
            rag_client=rag,
        )

        response = await service.generate(_request())

        self.assertEqual(response.relevant_files[0].path, "src/auth.py")
        self.assertEqual(rag.calls[0]["top_k"], 8)
        self.assertEqual(rag.calls[0]["filters"]["repository_id"], "owner/repository")
        self.assertEqual(rag.calls[0]["filters"]["commit_sha"], "abc123")
        self.assertIn("Validate token expiration", rag.calls[0]["query"])
        self.assertIn("Reject expired access tokens", rag.calls[0]["query"])
        self.assertIn("PRESELECTED ISSUE CONTEXT START", chat.messages[0][1]["content"])
        self.assertIn("def validate(token): pass", chat.messages[0][1]["content"])

    async def test_top_p_selects_a_dynamic_number_of_files(self):
        source = ProvidedFilesRepositorySource(
            [RepositoryFile(path="src/seed.py", content="SEED = True\n")],
            PlanConfiguration(max_files=20, max_snippets_per_file=3),
        )
        rag = FakeRag(
            [
                RagResult(
                    path="src/auth.py",
                    start_line=1,
                    end_line=5,
                    score=1.0,
                    content="primary",
                ),
                RagResult(
                    path="src/auth.py",
                    start_line=8,
                    end_line=12,
                    score=0.8,
                    content="same file",
                ),
                RagResult(
                    path="src/token.py",
                    start_line=1,
                    end_line=4,
                    score=0.5,
                    content="secondary",
                ),
                RagResult(
                    path="src/unrelated.py",
                    start_line=1,
                    end_line=4,
                    score=0.1,
                    content="low relevance",
                ),
            ]
        )
        sandbox = ToolSandbox(
            source,
            rag,
            ContextCompressor(10_000, 8_192),
            max_rag_files=20,
            max_rag_snippets_per_file=3,
        )

        narrow = await sandbox.prefetch_repository(
            query="token expiration",
            top_k=50,
            top_p=0.60,
        )
        balanced = await sandbox.prefetch_repository(
            query="token expiration",
            top_k=50,
            top_p=0.85,
        )
        broad = await sandbox.prefetch_repository(
            query="token expiration",
            top_k=50,
            top_p=0.95,
        )

        self.assertEqual(
            {result.path for result in narrow},
            {"src/auth.py"},
        )
        self.assertEqual(
            {result.path for result in balanced},
            {"src/auth.py", "src/token.py"},
        )
        self.assertEqual(
            {result.path for result in broad},
            {"src/auth.py", "src/token.py", "src/unrelated.py"},
        )
        self.assertEqual(len(balanced), 3)

    async def test_unavailable_rag_falls_back_to_supplied_files(self):
        rag = FakeRag(unavailable=True)
        chat = FakeChatClient(_plan("src/first.py"))
        service = AgentEngineService(
            AgentSettings(),
            model_client=chat,
            rag_client=rag,
        )

        response = await service.generate(_request())

        self.assertEqual(response.relevant_files[0].path, "src/first.py")
        self.assertNotIn("PRESELECTED ISSUE CONTEXT START", chat.messages[0][1]["content"])

    def test_max_files_does_not_remove_repository_files_from_tool_access(self):
        source = ProvidedFilesRepositorySource(
            [
                RepositoryFile(path="src/a.py", content="A = 1\n"),
                RepositoryFile(path="src/b.py", content="B = 2\n"),
            ],
            PlanConfiguration(max_files=1),
        )

        self.assertEqual(source.paths(), ["src/a.py", "src/b.py"])

    def test_issue_query_is_bounded_and_keeps_the_end_of_long_bodies(self):
        request = _request()
        request.issue.body = "first " + ("x" * 3_000) + " final-error-marker"

        query = build_issue_search_query(request.issue)

        self.assertEqual(len(query), MAX_RAG_QUERY_CHARS)
        self.assertTrue(query.startswith("Validate token expiration"))
        self.assertTrue(query.endswith("final-error-marker"))


if __name__ == "__main__":
    unittest.main()
