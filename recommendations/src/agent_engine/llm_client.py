import asyncio
import json
from dataclasses import dataclass, field, replace
from typing import Any

import httpx

from agent_engine.errors import EmptyModelOutputError, InferenceTimeoutError, ModelUnavailableError
from agent_engine.settings import AgentSettings, ModelEndpoint


@dataclass(frozen=True)
class ToolCall:
    id: str
    name: str
    arguments: dict[str, Any]


@dataclass(frozen=True)
class CompletionUsage:
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0

    def __add__(self, other: "CompletionUsage") -> "CompletionUsage":
        return CompletionUsage(
            prompt_tokens=self.prompt_tokens + other.prompt_tokens,
            completion_tokens=self.completion_tokens + other.completion_tokens,
            total_tokens=self.total_tokens + other.total_tokens,
        )


@dataclass(frozen=True)
class ChatCompletion:
    content: str
    reasoning: str
    tool_calls: list[ToolCall] = field(default_factory=list)
    usage: CompletionUsage = field(default_factory=CompletionUsage)
    model: str = ""


class OpenAICompatibleClient:
    """Small SERGE-style client for OpenAI-compatible model endpoints."""

    def __init__(
        self,
        settings: AgentSettings,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        self.settings = settings
        self._client = client
        self.endpoints = settings.model_endpoints()

    async def ready(self) -> bool:
        for endpoint in self.endpoints:
            try:
                data = await self._request_json("GET", "/models", endpoint=endpoint)
            except (ModelUnavailableError, InferenceTimeoutError):
                continue
            models = data.get("data", [])
            if any(
                item.get("id") == endpoint.model
                for item in models
                if isinstance(item, dict)
            ):
                return True
        return False

    async def complete(
        self,
        *,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]],
        response_schema: dict[str, Any] | None = None,
        max_tokens: int | None = None,
        enable_thinking: bool | None = None,
    ) -> ChatCompletion:
        last_error: Exception | None = None
        for endpoint in self.endpoints:
            try:
                response = await self._complete_with_endpoint(
                    endpoint,
                    messages=messages,
                    tools=tools,
                    response_schema=response_schema,
                    max_tokens=max_tokens,
                    enable_thinking=enable_thinking,
                )
                return replace(response, model=response.model or endpoint.model)
            except (ModelUnavailableError, InferenceTimeoutError) as exc:
                last_error = exc
        assert last_error is not None
        raise last_error

    async def _complete_with_endpoint(
        self,
        endpoint: ModelEndpoint,
        *,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]],
        response_schema: dict[str, Any] | None,
        max_tokens: int | None,
        enable_thinking: bool | None,
    ) -> ChatCompletion:
        payload = {
            "model": endpoint.model,
            "messages": messages,
            "temperature": 0,
            "max_tokens": max_tokens or self.settings.max_completion_tokens,
            "stream": True,
            "stream_options": {"include_usage": True},
        }
        if tools:
            payload["tools"] = tools
            payload["tool_choice"] = "auto"
        if enable_thinking is not None:
            payload["chat_template_kwargs"] = {"enable_thinking": enable_thinking}
        if response_schema is not None:
            payload["response_format"] = {
                "type": "json_schema",
                "json_schema": {
                    "name": "agent_engine_structured_response",
                    "schema": response_schema,
                    "strict": True,
                },
            }
        response = await self._request_stream(
            "POST", "/chat/completions", endpoint=endpoint, json=payload
        )
        if not response.content.strip() and not response.tool_calls:
            raise EmptyModelOutputError("model returned neither plan content nor tool calls")
        return response

    async def _request_stream(
        self, method: str, path: str, *, endpoint: ModelEndpoint, **kwargs: Any
    ) -> ChatCompletion:
        last_error: Exception | None = None
        for attempt in range(self.settings.max_retries + 1):
            owns_client = self._client is None
            client = self._client or self._new_client(endpoint)
            try:
                async with client.stream(
                    method, path, headers=self._headers(endpoint), **kwargs
                ) as response:
                    if response.status_code == 429 or response.status_code >= 500:
                        body = (await response.aread()).decode(errors="replace")
                        raise ModelUnavailableError(
                            f"model endpoint returned {response.status_code}: {body[:300]}"
                        )
                    if response.status_code >= 400:
                        body = (await response.aread()).decode(errors="replace")
                        raise ModelUnavailableError(
                            "model endpoint rejected request with "
                            f"{response.status_code}: {body[:300]}"
                        )
                    content_type = response.headers.get("content-type", "")
                    if "text/event-stream" not in content_type:
                        data = json.loads((await response.aread()).decode())
                        return _parse_non_stream_completion(data)
                    return await _parse_stream_completion(response)
            except httpx.TimeoutException as exc:
                last_error = InferenceTimeoutError("model inference timed out")
                if attempt >= self.settings.max_retries:
                    raise last_error from exc
            except httpx.RequestError as exc:
                last_error = ModelUnavailableError(f"cannot reach model endpoint: {exc}")
                if attempt >= self.settings.max_retries:
                    raise last_error from exc
            except (json.JSONDecodeError, KeyError, TypeError, ValueError) as exc:
                raise ModelUnavailableError(f"model endpoint returned invalid data: {exc}") from exc
            except ModelUnavailableError as exc:
                last_error = exc
                if attempt >= self.settings.max_retries:
                    raise
            finally:
                if owns_client:
                    await client.aclose()
            await asyncio.sleep(self.settings.retry_backoff_seconds * (2**attempt))
        assert last_error is not None
        raise last_error

    async def _request_json(
        self, method: str, path: str, *, endpoint: ModelEndpoint, **kwargs: Any
    ) -> dict[str, Any]:
        owns_client = self._client is None
        client = self._client or self._new_client(endpoint)
        try:
            response = await client.request(method, path, headers=self._headers(endpoint), **kwargs)
            response.raise_for_status()
            data = response.json()
            if not isinstance(data, dict):
                raise ValueError("response is not an object")
            return data
        except httpx.TimeoutException as exc:
            raise InferenceTimeoutError("model readiness check timed out") from exc
        except (httpx.HTTPError, ValueError) as exc:
            raise ModelUnavailableError(f"cannot query model endpoint: {exc}") from exc
        finally:
            if owns_client:
                await client.aclose()

    def _headers(self, endpoint: ModelEndpoint) -> dict[str, str]:
        headers = {"Accept": "text/event-stream"}
        if endpoint.openai_api_key:
            headers["Authorization"] = f"Bearer {endpoint.openai_api_key}"
        return headers

    def _new_client(self, endpoint: ModelEndpoint) -> httpx.AsyncClient:
        return httpx.AsyncClient(
            base_url=endpoint.openai_base_url,
            timeout=self.settings.request_timeout_seconds,
        )


async def _parse_stream_completion(response: httpx.Response) -> ChatCompletion:
    content_parts: list[str] = []
    reasoning_parts: list[str] = []
    calls: dict[int, dict[str, str]] = {}
    usage = CompletionUsage()
    async for line in response.aiter_lines():
        if not line.startswith("data:"):
            continue
        value = line[5:].strip()
        if not value or value == "[DONE]":
            continue
        chunk = json.loads(value)
        usage = _usage_from(chunk.get("usage")) or usage
        choices = chunk.get("choices") or []
        if not choices:
            continue
        delta = choices[0].get("delta") or {}
        if delta.get("content"):
            content_parts.append(delta["content"])
        reasoning = delta.get("reasoning_content") or delta.get("reasoning")
        if reasoning:
            reasoning_parts.append(reasoning)
        for raw_call in delta.get("tool_calls") or []:
            index = int(raw_call.get("index", 0))
            target = calls.setdefault(index, {"id": "", "name": "", "arguments": ""})
            target["id"] += raw_call.get("id") or ""
            function = raw_call.get("function") or {}
            target["name"] += function.get("name") or ""
            target["arguments"] += function.get("arguments") or ""
    return ChatCompletion(
        content="".join(content_parts),
        reasoning="".join(reasoning_parts),
        tool_calls=_build_tool_calls(calls),
        usage=usage,
    )


def _parse_non_stream_completion(data: dict[str, Any]) -> ChatCompletion:
    message = data["choices"][0]["message"]
    calls = {}
    for index, raw_call in enumerate(message.get("tool_calls") or []):
        function = raw_call.get("function") or {}
        calls[index] = {
            "id": raw_call.get("id") or f"call-{index}",
            "name": function.get("name") or "",
            "arguments": function.get("arguments") or "{}",
        }
    return ChatCompletion(
        content=message.get("content") or "",
        reasoning=message.get("reasoning_content") or message.get("reasoning") or "",
        tool_calls=_build_tool_calls(calls),
        usage=_usage_from(data.get("usage")) or CompletionUsage(),
    )


def _build_tool_calls(calls: dict[int, dict[str, str]]) -> list[ToolCall]:
    parsed = []
    for index in sorted(calls):
        call = calls[index]
        try:
            arguments = json.loads(call["arguments"] or "{}")
        except json.JSONDecodeError as exc:
            raise ModelUnavailableError(f"model returned invalid tool arguments: {exc}") from exc
        if not isinstance(arguments, dict) or not call["name"]:
            raise ModelUnavailableError("model returned an invalid tool call")
        parsed.append(
            ToolCall(
                id=call["id"] or f"call-{index}",
                name=call["name"],
                arguments=arguments,
            )
        )
    return parsed


def _usage_from(value: Any) -> CompletionUsage | None:
    if not isinstance(value, dict):
        return None
    return CompletionUsage(
        prompt_tokens=int(value.get("prompt_tokens") or 0),
        completion_tokens=int(value.get("completion_tokens") or 0),
        total_tokens=int(value.get("total_tokens") or 0),
    )
