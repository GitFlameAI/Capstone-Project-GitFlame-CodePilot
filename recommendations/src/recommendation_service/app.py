from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse, Response

from observability import (
    ObservabilityMiddleware,
    llm_operation_scope,
    metrics_payload,
    setup_logging,
)

from recommendation_service.analyzers import AnalyzerOrchestrator
from recommendation_service.config import ConfigError
from recommendation_service.model_client import (
    ModelOutputError,
    ModelTimeoutError,
    ModelUnavailableError,
    RecommendationModelClient,
)
from recommendation_service.models import (
    AnalyzeRequest,
    ErrorResponse,
    HealthResponse,
    RecommendationResponse,
)
from recommendation_service.service import RecommendationService
from recommendation_service.settings import Settings


def create_app(
    *,
    settings: Settings | None = None,
    model_client: RecommendationModelClient | None = None,
    analyzer_orchestrator: AnalyzerOrchestrator | None = None,
) -> FastAPI:
    setup_logging("recommendation-service")
    resolved_settings = settings or Settings.from_env()
    resolved_client = model_client or RecommendationModelClient(resolved_settings)
    recommendation_service = RecommendationService(
        resolved_client,
        analyzer_orchestrator=analyzer_orchestrator,
    )

    app = FastAPI(
        title="GitFlame Recommendation ML Service",
        version="0.1.0",
        description="Real model-backed Sprint 1 recommendation service. No mock fallback.",
    )

    app.add_middleware(ObservabilityMiddleware, service="recommendation-service")

    @app.get("/metrics", include_in_schema=False)
    async def metrics() -> Response:
        body, content_type = metrics_payload()
        return Response(content=body, media_type=content_type)

    @app.exception_handler(ConfigError)
    async def config_error_handler(_, exc: ConfigError) -> JSONResponse:
        return JSONResponse(status_code=422, content={"detail": str(exc)})

    @app.get("/health", response_model=HealthResponse)
    async def health() -> HealthResponse:
        return HealthResponse(status="ok", model=resolved_settings.model)

    @app.get(
        "/ready",
        response_model=HealthResponse,
        responses={503: {"model": ErrorResponse}},
    )
    async def ready() -> HealthResponse:
        if not await resolved_client.ready():
            raise HTTPException(
                status_code=503,
                detail=f"model {resolved_settings.model} is not available",
            )
        return HealthResponse(status="ready", model=resolved_settings.model)

    @app.post(
        "/v1/recommendations/analyze",
        response_model=RecommendationResponse,
        responses={
            422: {"model": ErrorResponse},
            502: {"model": ErrorResponse},
            503: {"model": ErrorResponse},
            504: {"model": ErrorResponse},
        },
    )
    async def analyze(request: AnalyzeRequest) -> RecommendationResponse:
        try:
            with llm_operation_scope("recommendations"):
                response, _ = await recommendation_service.analyze(request)
            return response
        except ModelUnavailableError as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        except ModelTimeoutError as exc:
            raise HTTPException(status_code=504, detail=str(exc)) from exc
        except ModelOutputError as exc:
            raise HTTPException(status_code=502, detail=str(exc)) from exc

    return app


app = create_app()


def run() -> None:
    import os

    import uvicorn

    uvicorn.run(
        "recommendation_service.app:app",
        host="0.0.0.0",
        port=int(os.environ.get("PORT", 8000)),
        log_config=None,
        access_log=False,
    )
