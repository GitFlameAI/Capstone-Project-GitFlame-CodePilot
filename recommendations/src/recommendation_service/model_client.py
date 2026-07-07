from dataclasses import dataclass
from typing import Any

from pydantic import ValidationError

from agent_engine.errors import (
    EmptyModelOutputError,
    InferenceTimeoutError,
    ModelUnavailableError as AgentModelUnavailableError,
)
from agent_engine.llm_client import OpenAICompatibleClient
from agent_engine.settings import AgentSettings
from recommendation_service.models import RecommendationResponse
from recommendation_service.settings import Settings


class ModelUnavailableError(RuntimeError):
    pass


class ModelTimeoutError(RuntimeError):
    pass


class ModelOutputError(RuntimeError):
    pass


@dataclass(frozen=True)
class InferenceMetrics:
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


class RecommendationModelClient:
    def __init__(
        self,
        settings: Settings,
        client: OpenAICompatibleClient | None = None,
    ) -> None:
        self.settings = settings
        self._client = client or OpenAICompatibleClient(
            AgentSettings(
                model=settings.model,
                openai_base_url=settings.openai_base_url,
                openai_api_key=settings.openai_api_key,
                request_timeout_seconds=settings.request_timeout_seconds,
                max_retries=settings.max_retries,
                retry_backoff_seconds=settings.retry_backoff_seconds,
            )
        )

    async def ready(self) -> bool:
        return await self._client.ready()

    async def analyze(
        self,
        *,
        system_prompt: str,
        user_prompt: str,
        response_schema: dict[str, Any],
    ) -> tuple[RecommendationResponse, InferenceMetrics]:
        try:
            completion = await self._client.complete(
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_prompt},
                ],
                tools=[],
                response_schema=response_schema,
            )
        except InferenceTimeoutError as exc:
            raise ModelTimeoutError("model inference timed out") from exc
        except (AgentModelUnavailableError, EmptyModelOutputError) as exc:
            raise ModelUnavailableError(str(exc)) from exc

        try:
            response = RecommendationResponse.model_validate_json(completion.content)
        except (ValidationError, ValueError) as exc:
            raise ModelOutputError(f"model returned invalid structured output: {exc}") from exc

        metrics = InferenceMetrics(
            prompt_tokens=completion.usage.prompt_tokens,
            completion_tokens=completion.usage.completion_tokens,
            total_tokens=completion.usage.total_tokens,
        )
        return response, metrics
