from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse

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
) -> FastAPI:
    resolved_settings = settings or Settings.from_env()
    resolved_client = model_client or RecommendationModelClient(resolved_settings)
    recommendation_service = RecommendationService(resolved_client)

    app = FastAPI(
        title="GitFlame Recommendation ML Service",
        version="0.1.0",
        description="Real model-backed Sprint 1 recommendation service. No mock fallback.",
    )

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
    import uvicorn

    uvicorn.run("recommendation_service.app:app", host="0.0.0.0", port=8000)

