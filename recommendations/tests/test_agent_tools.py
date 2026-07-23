import json

import pytest

from agent_engine.context import ContextCompressor
from agent_engine.errors import RagUnavailableError, ToolExecutionError
from agent_engine.models import PlanConfiguration, RagResult, RepositoryFile
from agent_engine.rag import DisabledRagClient
from agent_engine.repository import ProvidedFilesRepositorySource
from agent_engine.tools import ToolSandbox


class FakeRag:
    async def ready(self):
        return True

    async def search(self, *, query, top_k, filters=None):
        assert query == "authentication"
        assert top_k == 2
        return [
            RagResult(
                path="src/auth.py",
                start_line=1,
                end_line=2,
                score=0.9,
                content="def auth(): pass",
            )
        ]


class EmptyRag:
    async def ready(self):
        return True

    async def search(self, *, query, top_k, filters=None):
        return []


@pytest.fixture
def sandbox():
    source = ProvidedFilesRepositorySource(
        [
            RepositoryFile(path="src/auth.py", content="def auth():\n    return True\n"),
            RepositoryFile(path="src/api/routes.py", content="from src.auth import auth\n"),
        ],
        PlanConfiguration(include=["src/**"]),
    )
    return ToolSandbox(source, FakeRag(), ContextCompressor(10_000, 8_192))


@pytest.mark.asyncio
async def test_repository_tools_are_bounded_and_read_only(sandbox):
    read_result = json.loads(
        await sandbox.execute("read_file", {"path": "src/auth.py", "start_line": 1, "end_line": 2})
    )
    list_result = json.loads(await sandbox.execute("list_dir", {"path": "src"}))
    grep_result = json.loads(await sandbox.execute("grep", {"pattern": "auth", "path": "src"}))

    assert read_result["content"].startswith("1: def auth")
    assert {entry["name"] for entry in list_result["entries"]} == {"api", "auth.py"}
    assert len(grep_result["matches"]) == 2


@pytest.mark.asyncio
async def test_repository_tools_block_traversal_and_unknown_tools(sandbox):
    with pytest.raises(ToolExecutionError):
        await sandbox.execute("read_file", {"path": "../.env"})
    with pytest.raises(ToolExecutionError):
        await sandbox.execute("write_file", {"path": "src/auth.py"})


@pytest.mark.asyncio
async def test_search_repository_uses_external_rag_contract(sandbox):
    result = json.loads(
        await sandbox.execute("search_repository", {"query": "authentication", "top_k": 2})
    )

    assert result["status"] == "completed"
    assert result["results"][0] == {
        "path": "src/auth.py",
        "start_line": 1,
        "end_line": 2,
        "score": 0.9,
        "content": "def auth(): pass",
    }


@pytest.mark.asyncio
async def test_search_repository_marks_empty_rag_results(sandbox):
    sandbox.rag = EmptyRag()
    result = json.loads(await sandbox.execute("search_repository", {"query": "authentication"}))

    assert result == {"status": "empty", "results": []}


@pytest.mark.asyncio
async def test_disabled_rag_has_explicit_error(sandbox):
    sandbox.rag = DisabledRagClient()
    with pytest.raises(RagUnavailableError):
        await sandbox.execute("search_repository", {"query": "authentication"})
