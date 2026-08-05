from typing import Any, Protocol

import httpx
from pydantic import ValidationError

from agent_engine.errors import RagUnavailableError
from agent_engine.models import Issue, RagResult


MAX_RAG_QUERY_CHARS = 2_000


def build_issue_search_query(issue: Issue) -> str:
    """Build one stable hybrid-search query without asking the LLM to rewrite the issue."""
    title = " ".join(issue.title.split())
    body = issue.body.strip()
    query = f"{title}\n\n{body}" if body else title
    if len(query) <= MAX_RAG_QUERY_CHARS:
        return query

    marker = "\n...[issue truncated]...\n"
    remaining = MAX_RAG_QUERY_CHARS - len(marker)
    head = remaining * 2 // 3
    tail = remaining - head
    return query[:head] + marker + query[-tail:]


class RagSearch(Protocol):
    async def search(
        self, *, query: str, top_k: int, filters: dict[str, Any] | None = None
    ) -> list[RagResult]: ...

    async def ready(self) -> bool: ...


class DisabledRagClient:
    async def search(
        self, *, query: str, top_k: int, filters: dict[str, Any] | None = None
    ) -> list[RagResult]:
        raise RagUnavailableError("external RAG is not configured")

    async def ready(self) -> bool:
        return False


class HttpRagClient:
    def __init__(
        self,
        base_url: str,
        *,
        api_key: str | None = None,
        timeout_seconds: float = 30.0,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout_seconds = timeout_seconds
        self._client = client

    async def ready(self) -> bool:
        try:
            await self._request("GET", "/health")
            return True
        except RagUnavailableError:
            return False

    async def search(
        self, *, query: str, top_k: int, filters: dict[str, Any] | None = None
    ) -> list[RagResult]:
        if not query.strip():
            raise ValueError("RAG query cannot be empty")
        payload = {"query": query[:2_000], "top_k": min(max(top_k, 1), 50)}
        if filters:
            payload["filters"] = filters
        data = await self._request("POST", "/search", json=payload)
        try:
            raw_results = data["results"]
            if not isinstance(raw_results, list):
                raise TypeError("results must be a list")
            return [RagResult.model_validate(result) for result in raw_results[: payload["top_k"]]]
        except (KeyError, TypeError, ValidationError, ValueError) as exc:
            raise RagUnavailableError(f"RAG returned an invalid response: {exc}") from exc

    async def _request(self, method: str, path: str, **kwargs: Any) -> dict[str, Any]:
        headers = kwargs.pop("headers", {})
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        owns_client = self._client is None
        client = self._client or httpx.AsyncClient(
            base_url=self.base_url, timeout=self.timeout_seconds
        )
        try:
            response = await client.request(method, path, headers=headers, **kwargs)
            response.raise_for_status()
            data = response.json()
            if not isinstance(data, dict):
                raise ValueError("response is not an object")
            return data
        except (httpx.HTTPError, ValueError) as exc:
            raise RagUnavailableError(f"cannot use external RAG: {exc}") from exc
        finally:
            if owns_client:
                await client.aclose()
