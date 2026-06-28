import time

from pydantic import ValidationError

from agent_engine.context import ContextCompressor
from agent_engine.errors import InvalidGeneratedFilesError
from agent_engine.llm_client import OpenAICompatibleClient
from agent_engine.loop import AgentLoop, ChatClient
from agent_engine.models import (
    GeneratedFilesContract,
    GenerateFilesRequest,
    GenerateFilesResponse,
    GeneratePlanRequest,
    GeneratePlanResponse,
    Usage,
    parse_configuration,
)
from agent_engine.plan_validator import PlanValidator
from agent_engine.prompt import (
    CODE_GENERATION_SYSTEM_PROMPT,
    build_code_generation_prompt,
    build_initial_prompt,
)
from agent_engine.rag import DisabledRagClient, HttpRagClient, RagSearch
from agent_engine.repository import ProvidedFilesRepositorySource, path_is_allowed
from agent_engine.settings import AgentSettings
from agent_engine.tools import ToolSandbox


class AgentEngineService:
    def __init__(
        self,
        settings: AgentSettings,
        *,
        model_client: ChatClient | None = None,
        rag_client: RagSearch | None = None,
    ) -> None:
        self.settings = settings
        self.model_client = model_client or OpenAICompatibleClient(settings)
        self.rag_client = rag_client or self._build_rag_client(settings)

    async def ready(self) -> bool:
        return await self.model_client.ready()

    async def generate(self, request: GeneratePlanRequest) -> GeneratePlanResponse:
        configuration = parse_configuration(request.configuration_yaml)
        source = ProvidedFilesRepositorySource(request.repository_files, configuration)
        compressor = ContextCompressor(
            context_limit_tokens=self.settings.context_limit_tokens,
            max_tool_output_chars=self.settings.max_tool_output_chars,
        )
        sandbox = ToolSandbox(
            source,
            self.rag_client,
            compressor,
            rag_filters={
                "repository_id": request.repository.id,
                "commit_sha": request.repository.commit_sha,
                "include": configuration.include,
                "exclude": configuration.exclude,
            },
            rag_path_allowed=lambda path: path_is_allowed(path, configuration),
            max_rag_files=configuration.max_files,
            max_rag_snippets_per_file=configuration.max_snippets_per_file,
        )
        loop = AgentLoop(
            self.model_client,
            sandbox,
            PlanValidator(),
            compressor,
            self.settings,
        )
        result = await loop.run(build_initial_prompt(request, configuration, source))
        metrics = result.metrics
        return GeneratePlanResponse(
            request_id=request.request_id,
            plan_markdown=result.plan.markdown,
            relevant_files=result.plan.relevant_files,
            model=metrics.model,
            usage=Usage(
                prompt_tokens=metrics.usage.prompt_tokens,
                completion_tokens=metrics.usage.completion_tokens,
                total_tokens=metrics.usage.total_tokens,
                tool_calls=metrics.tool_calls,
                reasoning_chars=metrics.reasoning_chars,
                generation_time_seconds=metrics.generation_time_seconds,
            ),
        )

    async def generate_files(self, request: GenerateFilesRequest) -> GenerateFilesResponse:
        configuration = parse_configuration(request.configuration_yaml)
        source = ProvidedFilesRepositorySource(request.repository_files, configuration)
        compressor = ContextCompressor(
            context_limit_tokens=self.settings.context_limit_tokens,
            max_tool_output_chars=self.settings.max_tool_output_chars,
        )
        schema = GeneratedFilesContract.model_json_schema()
        prompt = build_code_generation_prompt(
            request,
            configuration,
            source,
            schema,
            compressor,
        )
        started = time.perf_counter()
        completion = await self.model_client.complete(
            messages=[
                {"role": "system", "content": CODE_GENERATION_SYSTEM_PROMPT},
                {
                    "role": "user",
                    "content": compressor.compress_text(prompt, compressor.max_context_chars),
                },
            ],
            tools=[],
            response_schema=schema,
        )
        generation_time = time.perf_counter() - started
        try:
            contract = GeneratedFilesContract.model_validate_json(completion.content)
        except (ValidationError, ValueError) as exc:
            raise InvalidGeneratedFilesError(
                f"model returned invalid generated files contract: {exc}"
            ) from exc

        self._validate_generated_files_contract(contract, source, configuration)
        return GenerateFilesResponse(
            request_id=request.request_id,
            summary=contract.summary,
            files=contract.files,
            model=completion.model or self.settings.model,
            usage=Usage(
                prompt_tokens=completion.usage.prompt_tokens,
                completion_tokens=completion.usage.completion_tokens,
                total_tokens=completion.usage.total_tokens,
                tool_calls=0,
                reasoning_chars=len(completion.reasoning),
                generation_time_seconds=generation_time,
            ),
        )

    @staticmethod
    def _build_rag_client(settings: AgentSettings) -> RagSearch:
        if not settings.rag_base_url:
            return DisabledRagClient()
        return HttpRagClient(
            settings.rag_base_url,
            api_key=settings.rag_api_key,
            timeout_seconds=min(settings.request_timeout_seconds, 30.0),
        )

    @staticmethod
    def _validate_generated_files_contract(
        contract: GeneratedFilesContract,
        source: ProvidedFilesRepositorySource,
        configuration,
    ) -> None:
        existing_paths = set(source.paths())
        for item in contract.files:
            if not path_is_allowed(item.path, configuration):
                raise InvalidGeneratedFilesError(
                    f"generated file path is excluded by configuration: {item.path}"
                )
            if item.action == "create" and item.path in existing_paths:
                raise InvalidGeneratedFilesError(
                    f"create action targets an existing supplied file: {item.path}"
                )
            if item.action in {"modify", "delete"} and item.path not in existing_paths:
                raise InvalidGeneratedFilesError(
                    f"{item.action} action targets an unknown supplied file: {item.path}"
                )
