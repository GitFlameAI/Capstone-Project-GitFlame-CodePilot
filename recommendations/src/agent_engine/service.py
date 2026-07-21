import json
import logging
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

logger = logging.getLogger(__name__)


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
            contract = _parse_generated_files_contract(completion.content)
            self._validate_generated_files_contract(contract, source, configuration)
        except (InvalidGeneratedFilesError, ValidationError, ValueError) as exc:
            first_context = _invalid_generated_files_context(completion.content)
            if first_context:
                logger.warning("model returned invalid generated files contract: %s", first_context)
            repair_completion = await self.model_client.complete(
                messages=[
                    {"role": "system", "content": CODE_GENERATION_SYSTEM_PROMPT},
                    {
                        "role": "user",
                        "content": compressor.compress_text(
                            _build_generated_files_repair_prompt(
                                prompt,
                                completion.content,
                                str(exc),
                                first_context,
                            ),
                            compressor.max_context_chars,
                        ),
                    },
                ],
                tools=[],
                response_schema=schema,
            )
            generation_time = time.perf_counter() - started
            try:
                contract = _parse_generated_files_contract(repair_completion.content)
                self._validate_generated_files_contract(contract, source, configuration)
            except (InvalidGeneratedFilesError, ValidationError, ValueError) as repair_exc:
                context = (
                    _invalid_generated_files_context(repair_completion.content)
                    or first_context
                )
                if context:
                    logger.warning(
                        "model returned invalid generated files contract after repair: %s",
                        context,
                    )
                raise InvalidGeneratedFilesError(
                    f"model returned invalid generated files contract: {repair_exc}"
                    + (f"; {context}" if context else "")
                ) from repair_exc
            completion = repair_completion

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
            if item.action == "modify":
                _validate_modify_content_shape(
                    item.path,
                    item.content or "",
                    source.read(item.path),
                )


def _invalid_generated_files_context(raw_content: str) -> str:
    try:
        payload = json.loads(raw_content)
    except (TypeError, json.JSONDecodeError):
        return ""
    files = payload.get("files") if isinstance(payload, dict) else None
    if not isinstance(files, list):
        return ""
    for index, item in enumerate(files):
        if not isinstance(item, dict):
            return f"invalid file candidate at files.{index}: non-object item"
        action = str(item.get("action", ""))
        path = str(item.get("path", ""))
        explanation = str(item.get("explanation", ""))
        content = item.get("content")
        diff = item.get("diff")
        if action in {"create", "modify"} and not str(content or "").strip():
            return _format_invalid_file_context(index, action, path, explanation)
        if action == "delete" and (str(content or "").strip() or str(diff or "").strip()):
            return _format_invalid_file_context(index, action, path, explanation)
        if action not in {"create", "modify", "delete"}:
            return _format_invalid_file_context(index, action, path, explanation)
    return ""


def _parse_generated_files_contract(raw_content: str) -> GeneratedFilesContract:
    try:
        payload = json.loads(raw_content)
    except (TypeError, json.JSONDecodeError):
        return GeneratedFilesContract.model_validate_json(raw_content)
    return GeneratedFilesContract.model_validate(
        _merge_duplicate_generated_files_payload(payload)
    )


def _merge_duplicate_generated_files_payload(payload):
    if not isinstance(payload, dict):
        return payload
    files = payload.get("files")
    if not isinstance(files, list):
        return payload

    merged_files = []
    index_by_path = {}
    for item in files:
        if not isinstance(item, dict):
            merged_files.append(item)
            continue
        path = _generated_file_merge_path(item.get("path", ""))
        if not path or path not in index_by_path:
            index_by_path[path] = len(merged_files)
            merged_files.append(item)
            continue

        existing_index = index_by_path[path]
        previous = merged_files[existing_index]
        if isinstance(previous, dict):
            merged_files[existing_index] = _merge_generated_file_item(previous, item)
        else:
            merged_files.append(item)

    if len(merged_files) == len(files):
        return payload
    return {**payload, "files": merged_files}


def _generated_file_merge_path(value) -> str:
    raw = str(value or "")
    normalized = raw.replace("\\", "/").removeprefix("./").strip("/")
    if (
        not normalized
        or raw.startswith(("/", "\\"))
        or ".." in normalized.split("/")
        or ":" in normalized.split("/")[0]
        or normalized == ".git"
        or normalized.startswith(".git/")
    ):
        return raw.strip()
    return normalized


def _merge_generated_file_item(previous: dict, current: dict) -> dict:
    merged = dict(previous)
    for key, value in current.items():
        if key == "explanation":
            continue
        if key in {"content", "diff"}:
            if str(value or "").strip() or key not in merged:
                merged[key] = value
            continue
        if str(value or "").strip() or key not in merged:
            merged[key] = value

    explanation = _merge_explanation(
        str(previous.get("explanation", "")),
        str(current.get("explanation", "")),
    )
    if explanation:
        merged["explanation"] = explanation
    return merged


def _merge_explanation(previous: str, current: str) -> str:
    previous = previous.strip()
    current = current.strip()
    if not previous:
        return current
    if not current or current == previous:
        return previous
    return previous + "\n" + current


def _validate_modify_content_shape(path: str, generated: str, original: str) -> None:
    generated = _normalize_content(generated)
    original = _normalize_content(original)
    generated_lines = _meaningful_lines(generated)
    original_lines = _meaningful_lines(original)
    if len(original_lines) >= 3 and len(generated_lines) <= 1:
        raise InvalidGeneratedFilesError(
            f"modify content for {path} appears compressed or partial; "
            "return the full file with line breaks"
        )
    if len(original) >= 120 and len(generated) < max(80, int(len(original) * 0.35)):
        raise InvalidGeneratedFilesError(
            f"modify content for {path} is too short for a full replacement file"
        )
    if len(original_lines) >= 8 and len(generated_lines) < max(3, int(len(original_lines) * 0.35)):
        raise InvalidGeneratedFilesError(
            f"modify content for {path} has too few lines for a full replacement file"
        )


def _normalize_content(value: str) -> str:
    return value.replace("\r\n", "\n").replace("\r", "\n").strip()


def _meaningful_lines(value: str) -> list[str]:
    return [line for line in value.split("\n") if line.strip()]


def _build_generated_files_repair_prompt(
    original_prompt: str,
    invalid_candidate: str,
    validation_error: str,
    context: str,
) -> str:
    return "\n".join(
        [
            original_prompt,
            "",
            "The previous generated files JSON was invalid. Repair it and return one complete",
            "generated files JSON object only.",
            "For modify operations, return the entire final file content with preserved line",
            "breaks; do not return snippets, summaries, minified code, or only changed lines.",
            "",
            "Validation error:",
            validation_error,
            "",
            "Invalid candidate context:",
            context or "<none>",
            "",
            "<invalid_generated_files_candidate>",
            _truncate(invalid_candidate, 20_000),
            "</invalid_generated_files_candidate>",
            "",
            "Rules for the repaired JSON:",
            "- Keep only valid repository-relative paths.",
            "- For every create or modify operation, include non-empty `content` with the "
            "complete replacement file content.",
            "- Do not return a modify operation if you cannot provide the complete "
            "replacement content.",
            "- Return JSON only.",
        ]
    )


def _format_invalid_file_context(index: int, action: str, path: str, explanation: str) -> str:
    return (
        f"invalid file candidate at files.{index}: "
        f"action={action or '<missing>'}, "
        f"path={path or '<missing>'}, "
        f"explanation={_truncate(explanation, 240) or '<missing>'}"
    )


def _truncate(value: str, limit: int) -> str:
    value = value.replace("\n", "\\n")
    if len(value) <= limit:
        return value
    return value[: limit - 3] + "..."
