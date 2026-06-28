import json

import httpx
import pytest

from agent_engine.llm_client import OpenAICompatibleClient
from agent_engine.settings import AgentSettings


@pytest.mark.asyncio
async def test_openai_client_parses_streamed_content_reasoning_tools_and_usage():
    captured = {}

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.update(json.loads(request.content))
        events = [
            {"choices": [{"delta": {"reasoning_content": "inspect "}}]},
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {
                                    "index": 0,
                                    "id": "call-1",
                                    "function": {
                                        "name": "read_file",
                                        "arguments": '{"path":"src/auth.py"}',
                                    },
                                }
                            ]
                        }
                    }
                ]
            },
            {
                "choices": [],
                "usage": {"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
            },
        ]
        body = "".join(f"data: {json.dumps(event)}\n\n" for event in events) + "data: [DONE]\n\n"
        return httpx.Response(200, content=body, headers={"content-type": "text/event-stream"})

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), base_url="http://model/v1"
    )
    client = OpenAICompatibleClient(AgentSettings(), client=http_client)
    result = await client.complete(messages=[{"role": "user", "content": "plan"}], tools=[])

    assert captured["stream"] is True
    assert result.reasoning == "inspect "
    assert result.tool_calls[0].arguments == {"path": "src/auth.py"}
    assert result.usage.total_tokens == 14
    await http_client.aclose()


@pytest.mark.asyncio
async def test_openai_client_ready_checks_exact_model():
    async def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"data": [{"id": AgentSettings.model}]})

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), base_url="http://model/v1"
    )
    client = OpenAICompatibleClient(AgentSettings(), client=http_client)

    assert await client.ready() is True
    await http_client.aclose()


@pytest.mark.asyncio
async def test_openai_client_uses_fallback_model_when_primary_is_unavailable():
    seen_models = []
    seen_response_format = None

    async def handler(request: httpx.Request) -> httpx.Response:
        nonlocal seen_response_format
        payload = json.loads(request.content)
        seen_models.append(payload["model"])
        if payload["model"] == "primary-model":
            return httpx.Response(503, json={"error": "primary down"})
        seen_response_format = payload.get("response_format")
        body = (
            "data: "
            + json.dumps({"choices": [{"delta": {"content": '{"summary":"ok","files":[]}'}}]})
            + "\n\n"
            + "data: [DONE]\n\n"
        )
        return httpx.Response(200, content=body, headers={"content-type": "text/event-stream"})

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler), base_url="http://model/v1"
    )
    client = OpenAICompatibleClient(
        AgentSettings(
            model="primary-model",
            fallback_model="fallback-model",
            fallback_openai_base_url="http://fallback/v1",
            max_retries=0,
        ),
        client=http_client,
    )

    result = await client.complete(
        messages=[{"role": "user", "content": "generate"}],
        tools=[],
        response_schema={"type": "object", "additionalProperties": False},
    )

    assert seen_models == ["primary-model", "fallback-model"]
    assert result.model == "fallback-model"
    assert result.content == '{"summary":"ok","files":[]}'
    assert seen_response_format["type"] == "json_schema"
    assert seen_response_format["json_schema"]["strict"] is True
    await http_client.aclose()
