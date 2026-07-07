from recommendation_service.config import ConfigError, filter_repo_context, parse_config
from recommendation_service.model_client import (
    InferenceMetrics,
    ModelOutputError,
    RecommendationModelClient,
)
from recommendation_service.models import (
    SEVERITY_RANK,
    AnalyzeRequest,
    RecommendationResponse,
)
from recommendation_service.prompt import SYSTEM_PROMPT, build_analysis_prompt


class RecommendationService:
    def __init__(self, model_client: RecommendationModelClient) -> None:
        self.model_client = model_client

    async def analyze(
        self, request: AnalyzeRequest
    ) -> tuple[RecommendationResponse, InferenceMetrics]:
        config = parse_config(request.config_yaml)
        files = filter_repo_context(request.repo_context, config)
        schema = RecommendationResponse.model_json_schema()
        prompt = build_analysis_prompt(files, config, schema)
        response, metrics = await self.model_client.analyze(
            system_prompt=SYSTEM_PROMPT,
            user_prompt=prompt,
            response_schema=schema,
        )

        file_lines = {
            file.path: max(1, len(file.content.splitlines()))
            for file in files
        }
        allowed_categories = set(config.recommendations.categories)
        minimum_severity = SEVERITY_RANK[config.recommendations.severity_threshold]
        filtered = []
        for recommendation in response.recommendations:
            if recommendation.file not in file_lines:
                raise ModelOutputError(
                    f"model referenced an unknown or excluded file: {recommendation.file}"
                )
            if recommendation.line > file_lines[recommendation.file]:
                raise ModelOutputError(
                    f"model referenced invalid line {recommendation.line} in {recommendation.file}"
                )
            if recommendation.category not in allowed_categories:
                raise ModelOutputError(
                    f"model returned disallowed category: {recommendation.category.value}"
                )
            if SEVERITY_RANK[recommendation.severity] >= minimum_severity:
                filtered.append(recommendation)

        return RecommendationResponse(summary=response.summary, recommendations=filtered), metrics


__all__ = ["ConfigError", "RecommendationService"]
