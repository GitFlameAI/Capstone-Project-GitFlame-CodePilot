import json

import httpx
import pytest

from agent_engine.errors import RagUnavailableError
from agent_engine.rag import HttpRagClient
from agent_engine.settings import AgentSettings


@pytest.mark.asyncio
async def test_http_rag_client_matches_coderag_contract() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        assert request.headers["Authorization"] == "Bearer rag-secret"
        if request.url.path == "/health":
            return httpx.Response(
                200,
                json={"status": "ok", "database": "ready", "version": "1.0.0"},
            )
        assert request.url.path == "/search"
        assert request.method == "POST"
        assert request.read()
        return httpx.Response(
            200,
            json={
                "results": [
                    {
                        "path": "src/auth.py",
                        "start_line": 10,
                        "end_line": 18,
                        "score": 0.92,
                        "content": "def authenticate(token): ...",
                    }
                ]
            },
        )

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(handler),
        base_url="https://coderag.test",
    ) as http_client:
        rag = HttpRagClient(
            "https://coderag.test",
            api_key="rag-secret",
            client=http_client,
        )
        assert await rag.ready() is True
        results = await rag.search(
            query="where is authentication validated?",
            top_k=5,
            filters={
                "repository_id": "owner/repository",
                "commit_sha": "abc123",
                "include": ["src/**"],
                "exclude": ["tests/**"],
            },
        )

    assert len(requests) == 2
    assert results[0].model_dump() == {
        "path": "src/auth.py",
        "start_line": 10,
        "end_line": 18,
        "score": 0.92,
        "content": "def authenticate(token): ...",
    }
    search_payload = json.loads(requests[1].content)
    assert search_payload == {
        "query": "where is authentication validated?",
        "top_k": 5,
        "filters": {
            "repository_id": "owner/repository",
            "commit_sha": "abc123",
            "include": ["src/**"],
            "exclude": ["tests/**"],
        },
    }


@pytest.mark.asyncio
async def test_http_rag_client_accepts_honest_empty_result() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(200, json={"results": []})
    )
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://coderag.test",
    ) as http_client:
        rag = HttpRagClient("https://coderag.test", client=http_client)
        results = await rag.search(
            query="unknown behavior",
            top_k=10,
            filters={"repository_id": "owner/repository", "commit_sha": "abc123"},
        )

    assert results == []


@pytest.mark.asyncio
async def test_http_rag_client_rejects_invalid_evidence_contract() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(
            200,
            json={
                "results": [
                    {
                        "path": "src/auth.py",
                        "start_line": 18,
                        "end_line": 10,
                        "score": 1.2,
                        "content": "invalid",
                    }
                ]
            },
        )
    )
    async with httpx.AsyncClient(
        transport=transport,
        base_url="https://coderag.test",
    ) as http_client:
        rag = HttpRagClient("https://coderag.test", client=http_client)
        with pytest.raises(RagUnavailableError, match="invalid response"):
            await rag.search(query="authentication", top_k=5)


def test_laguna_context_default_matches_runtime_contract() -> None:
    assert AgentSettings().context_limit_tokens == 32_768
