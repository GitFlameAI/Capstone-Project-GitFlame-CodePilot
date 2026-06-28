from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from agent_engine.errors import (
    AgentEngineError,
    ConfigurationError,
    EmptyModelOutputError,
    InferenceTimeoutError,
    InvalidGeneratedFilesError,
    InvalidPlanError,
    ModelUnavailableError,
    RagUnavailableError,
    ToolLimitExceededError,
)
from agent_engine.models import (
    ErrorResponse,
    GenerateFilesRequest,
    GenerateFilesResponse,
    GeneratePlanRequest,
    GeneratePlanResponse,
    HealthResponse,
)
from agent_engine.rag import RagSearch
from agent_engine.service import AgentEngineService
from agent_engine.settings import AgentSettings


def create_app(
    *,
    settings: AgentSettings | None = None,
    model_client=None,
    rag_client: RagSearch | None = None,
) -> FastAPI:
    resolved_settings = settings or AgentSettings.from_env()
    service = AgentEngineService(
        resolved_settings,
        model_client=model_client,
        rag_client=rag_client,
    )
    app = FastAPI(
        title="GitFlame SERGE-based Agent Engine",
        version="3.0.0",
        description=(
            "Stateless issue-to-plan and approved-plan-to-generated-files Agent Engine."
        ),
    )

    @app.exception_handler(ConfigurationError)
    async def configuration_error_handler(
        _request: Request, exc: ConfigurationError
    ) -> JSONResponse:
        return _error(422, exc.code, str(exc))

    @app.exception_handler(RequestValidationError)
    async def request_validation_error_handler(
        _request: Request, exc: RequestValidationError
    ) -> JSONResponse:
        return _error(422, "invalid_request", str(exc))

    for error_type, status_code in {
        ModelUnavailableError: 503,
        RagUnavailableError: 503,
        InvalidPlanError: 502,
        InvalidGeneratedFilesError: 502,
        EmptyModelOutputError: 502,
        ToolLimitExceededError: 422,
        InferenceTimeoutError: 504,
    }.items():
        app.add_exception_handler(error_type, _handler(status_code))

    @app.get("/health", response_model=HealthResponse)
    async def health() -> HealthResponse:
        return HealthResponse(status="ok", model=resolved_settings.model)

    @app.get(
        "/ready",
        response_model=HealthResponse,
        responses={503: {"model": ErrorResponse}},
    )
    async def ready() -> HealthResponse | JSONResponse:
        if not await service.ready():
            return _error(
                503,
                ModelUnavailableError.code,
                f"model {resolved_settings.model} is not ready",
            )
        return HealthResponse(status="ready", model=resolved_settings.model)

    @app.post(
        "/v1/plans/generate",
        response_model=GeneratePlanResponse,
        responses={
            422: {"model": ErrorResponse},
            502: {"model": ErrorResponse},
            503: {"model": ErrorResponse},
            504: {"model": ErrorResponse},
        },
    )
    async def generate(request: GeneratePlanRequest) -> GeneratePlanResponse:
        return await service.generate(request)

    @app.post(
        "/v1/files/generate",
        response_model=GenerateFilesResponse,
        responses={
            422: {"model": ErrorResponse},
            502: {"model": ErrorResponse},
            503: {"model": ErrorResponse},
            504: {"model": ErrorResponse},
        },
    )
    async def generate_files(request: GenerateFilesRequest) -> GenerateFilesResponse:
        return await service.generate_files(request)

    return app


def _handler(status_code: int):
    async def handler(_request: Request, exc: AgentEngineError) -> JSONResponse:
        return _error(status_code, exc.code, str(exc))

    return handler


def _error(status_code: int, code: str, detail: str) -> JSONResponse:
    return JSONResponse(status_code=status_code, content={"code": code, "detail": detail})


app = create_app()


def run() -> None:
    import uvicorn

    uvicorn.run("agent_engine.app:app", host="0.0.0.0", port=8001)
