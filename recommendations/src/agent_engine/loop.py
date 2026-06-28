import asyncio
import json
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
from agent_engine.llm_client import ChatCompletion, CompletionUsage
from agent_engine.plan_validator import PlanValidator, ValidatedPlan
from agent_engine.prompt import SYSTEM_PROMPT, build_validation_feedback
from agent_engine.settings import AgentSettings
from agent_engine.tools import ToolSandbox


class ChatClient(Protocol):
    async def ready(self) -> bool: ...

    async def complete(
        self,
        *,
        messages: list[dict[str, Any]],
        tools: list[dict[str, Any]],
        response_schema: dict[str, Any] | None = None,
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

        for _step in range(self.settings.max_steps):
            started = time.perf_counter()
            completion = await self.client.complete(
                messages=self.compressor.fit_messages(messages),
                tools=self.sandbox.definitions,
            )
            generation_time += time.perf_counter() - started
            total_usage += completion.usage
            reasoning_chars += len(completion.reasoning)
            model = completion.model or model

            if completion.tool_calls:
                total_tool_calls += len(completion.tool_calls)
                if total_tool_calls > self.settings.max_tool_calls:
                    raise ToolLimitExceededError(
                        f"model exceeded the {self.settings.max_tool_calls}-call tool limit"
                    )
                messages.append(_assistant_tool_call_message(completion))
                for call in completion.tool_calls:
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
