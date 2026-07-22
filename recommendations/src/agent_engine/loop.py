import asyncio
import json
import logging
import time
from dataclasses import dataclass
from typing import Any, Protocol

from agent_engine.context import ContextCompressor
from agent_engine.errors import (
    InferenceTimeoutError,
    InvalidPlanError,
    #RagUnavailableError,
    ToolLimitExceededError,
)
from agent_engine.llm_client import ChatCompletion, CompletionUsage, ToolCall
from agent_engine.plan_validator import PlanValidator, ValidatedPlan
from agent_engine.prompt import SYSTEM_PROMPT, build_validation_feedback
from agent_engine.settings import AgentSettings
from agent_engine.tools import ToolSandbox

logger = logging.getLogger(__name__)


class ChatClient(Protocol):
    async def ready(self) -> bool: ...

    async def complete(
        self,
        *,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]],
        response_schema: dict[str, Any] | None = None,
        max_tokens: int | None = None,
        enable_thinking: bool | None = None,
    ) -> ChatCompletion: ...


@dataclass(frozen=True)
class LoopMetrics:
    usage: CompletionUsage
    tool_calls: int
    reasoning_chars: int
    generation_time_seconds: float
    model: str


@dataclass(frozen=True)
class LoopResult:
    plan: ValidatedPlan
    metrics: LoopMetrics


class AgentLoop:
    def __init__(
        self,
        client: ChatClient,
        sandbox: ToolSandbox,
        validator: PlanValidator,
        compressor: ContextCompressor,
        settings: AgentSettings,
    ) -> None:
        self.client = client
        self.sandbox = sandbox
        self.validator = validator
        self.compressor = compressor
        self.settings = settings

    async def run(self, initial_prompt: str) -> LoopResult:
        try:
            async with asyncio.timeout(self.settings.agent_timeout_seconds):
                return await self._run(initial_prompt)
        except TimeoutError as exc:
            raise InferenceTimeoutError("Agent Loop exceeded its total timeout") from exc

    async def _run(self, initial_prompt: str) -> LoopResult:
        messages: list[dict[str, Any]] = [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": initial_prompt},
        ]
        total_usage = CompletionUsage()
        total_tool_calls = 0
        reasoning_chars = 0
        generation_time = 0.0
        model = self.settings.model
        last_validation_errors: list[str] = []
        repeated_tool_calls: set[str] = set()
        force_synthesis = False
        synthesis_notice_sent = False
        reserved_steps = min(
            max(0, self.settings.synthesis_reserved_steps),
            max(0, self.settings.max_steps - 1),
        )

        for step_index in range(self.settings.max_steps):
            step_number = step_index + 1
            tools_enabled = (
                not force_synthesis
                and self.settings.max_steps - step_index > reserved_steps
            )
            if not tools_enabled and not synthesis_notice_sent:
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            "Tool use is now disabled. Use the repository evidence already "
                            "collected in this conversation and return the required complete "
                            "Markdown implementation plan now. Do not ask for more tools."
                        ),
                    }
                )
                synthesis_notice_sent = True
            started = time.perf_counter()
            completion = await self.client.complete(
                messages=self.compressor.fit_messages(messages),
                tools=self.sandbox.definitions if tools_enabled else [],
                max_tokens=self.settings.plan_max_completion_tokens,
            )
            generation_time += time.perf_counter() - started
            total_usage += completion.usage
            reasoning_chars += len(completion.reasoning)
            model = completion.model or model

            if completion.tool_calls:
                if not tools_enabled:
                    messages.append(
                        {
                            "role": "user",
                            "content": (
                                "Tool calls are not available in the synthesis phase. Return the "
                                "required Markdown plan using existing evidence only."
                            ),
                        }
                    )
                    continue

                duplicate_call = _first_duplicate_tool_call(
                    completion.tool_calls,
                    repeated_tool_calls,
                )
                if duplicate_call:
                    logger.warning(
                        "agent repeated tool call at step=%d tool=%s arguments=%s; "
                        "forcing synthesis",
                        step_number,
                        duplicate_call.name,
                        _tool_call_arguments_for_log(duplicate_call.arguments),
                    )
                    force_synthesis = True
                    messages.append(
                        {
                            "role": "user",
                            "content": (
                                "You repeated an identical tool call, so the research phase is "
                                "complete. Return the required Markdown implementation plan using "
                                "the evidence already collected."
                            ),
                        }
                    )
                    continue

                total_tool_calls += len(completion.tool_calls)
                if total_tool_calls > self.settings.max_tool_calls:
                    raise ToolLimitExceededError(
                        f"model exceeded the {self.settings.max_tool_calls}-call tool limit"
                    )
                messages.append(_assistant_tool_call_message(completion))
                for call in completion.tool_calls:
                    logger.info(
                        "agent tool call step=%d tool=%s arguments=%s",
                        step_number,
                        call.name,
                        _tool_call_arguments_for_log(call.arguments),
                    )
                    try:
                        output = await self.sandbox.execute(call.name, call.arguments)
                    #except RagUnavailableError:
                    #    raise
                    except Exception as exc:
                        output = json.dumps(
                            {"error": type(exc).__name__, "detail": str(exc)},
                            ensure_ascii=False,
                        )
                    messages.append(
                        {
                            "role": "tool",
                            "tool_call_id": call.id,
                            "name": call.name,
                            "content": output,
                        }
                    )
                continue

            candidate = completion.content.strip()
            last_validation_errors = self.validator.collect_errors(
                candidate, self.sandbox.evidence_paths
            )
            if not last_validation_errors:
                plan = self.validator.validate(candidate, self.sandbox.evidence_paths)
                return LoopResult(
                    plan=plan,
                    metrics=LoopMetrics(
                        usage=total_usage,
                        tool_calls=total_tool_calls,
                        reasoning_chars=reasoning_chars,
                        generation_time_seconds=generation_time,
                        model=model,
                    ),
                )
            messages.extend(
                [
                    {"role": "assistant", "content": candidate},
                    {"role": "user", "content": build_validation_feedback(last_validation_errors)},
                ]
            )

        if last_validation_errors:
            raise InvalidPlanError("; ".join(last_validation_errors))
        raise ToolLimitExceededError(
            f"Agent Loop reached the {self.settings.max_steps}-step limit without a valid plan"
        )


def _first_duplicate_tool_call(
    calls: list[ToolCall],
    previous_calls: set[str],
) -> ToolCall | None:
    for call in calls:
        key = _tool_call_key(call)
        if key in previous_calls:
            return call
        previous_calls.add(key)
    return None


def _tool_call_key(call: ToolCall) -> str:
    return f"{call.name}:{json.dumps(call.arguments, ensure_ascii=False, sort_keys=True)}"


def _tool_call_arguments_for_log(arguments: dict[str, Any]) -> str:
    return json.dumps(arguments, ensure_ascii=False, sort_keys=True)[:500]


def _assistant_tool_call_message(completion: ChatCompletion) -> dict[str, Any]:
    return {
        "role": "assistant",
        "content": completion.content or None,
        "tool_calls": [
            {
                "id": call.id,
                "type": "function",
                "function": {
                    "name": call.name,
                    "arguments": json.dumps(call.arguments, ensure_ascii=False),
                },
            }
            for call in completion.tool_calls
        ],
    }
