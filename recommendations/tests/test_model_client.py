import httpx
import pytest

from agent_engine.llm_client import OpenAICompatibleClient
from agent_engine.settings import AgentSettings
from recommendation_service.model_client import (
    ModelOutputError,
    ModelUnavailableError,
    RecommendationModelClient,
)
from recommendation_service.models import RecommendationResponse
from recommendation_service.settings import Settings
from tests.conftest import openai_response


@pytest.mark.asyncio
async def test_model_client_uses_schema_and_parses_usage():
    captured = {}

    async def handler(request: httpx.Request) -> httpx.Response:
        if request.url.path == "/models":
            return httpx.Response(200, json={"data": [{"id": "laguna"}]})
        captured.update(__import__("json").loads(request.content))
        return httpx.Response(
            200,
            json=openai_response(
                {
                    "summary": "No issues.",
                    "recommendations": [],
                },
                prompt_tokens=10,
                completion_tokens=5,
                total_tokens=15,
            ),
        )

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler),
        base_url="https://model.test/v1",
    )
    client = RecommendationModelClient(
        Settings(model="laguna", openai_base_url="https://model.test/v1"),
        client=OpenAICompatibleClient(
            AgentSettings(
                model="laguna",
                openai_base_url="https://model.test/v1",
            ),
            client=http_client,
        ),
    )

    assert await client.ready()
    response, metrics = await client.analyze(
        system_prompt="system",
        user_prompt="user",
        response_schema=RecommendationResponse.model_json_schema(),
    )

    assert response.recommendations == []
    assert captured["response_format"]["json_schema"]["schema"]["additionalProperties"] is False
    assert captured["temperature"] == 0
    assert metrics.total_tokens == 15
    await http_client.aclose()


@pytest.mark.asyncio
async def test_model_client_rejects_invalid_json():
    async def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"choices": [{"message": {"content": "not-json"}}]})

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler),
        base_url="https://model.test/v1",
    )
    client = RecommendationModelClient(
        Settings(model="laguna", openai_base_url="https://model.test/v1"),
        client=OpenAICompatibleClient(
            AgentSettings(
                model="laguna",
                openai_base_url="https://model.test/v1",
            ),
            client=http_client,
        ),
    )

    with pytest.raises(ModelOutputError):
        await client.analyze(system_prompt="s", user_prompt="u", response_schema={})
    await http_client.aclose()


@pytest.mark.asyncio
async def test_model_client_treats_missing_model_as_unavailable():
    async def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(404, json={"error": "model not found"})

    http_client = httpx.AsyncClient(
        transport=httpx.MockTransport(handler),
        base_url="https://model.test/v1",
    )
    client = RecommendationModelClient(
        Settings(model="laguna", openai_base_url="https://model.test/v1"),
        client=OpenAICompatibleClient(
            AgentSettings(
                model="laguna",
                openai_base_url="https://model.test/v1",
            ),
            client=http_client,
        ),
    )

    with pytest.raises(ModelUnavailableError):
        await client.analyze(system_prompt="s", user_prompt="u", response_schema={})
    await http_client.aclose()
