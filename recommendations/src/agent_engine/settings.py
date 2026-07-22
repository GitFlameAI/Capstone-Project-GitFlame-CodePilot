import os
from dataclasses import dataclass


@dataclass(frozen=True)
class ModelEndpoint:
    model: str
    openai_base_url: str
    openai_api_key: str | None = None


@dataclass(frozen=True)
class AgentSettings:
    model: str = "Qwen/Qwen3-Coder-30B-A3B-Instruct"
    openai_base_url: str = "http://127.0.0.1:8000/v1"
    openai_api_key: str | None = None
    fallback_model: str | None = None
    fallback_openai_base_url: str | None = None
    fallback_openai_api_key: str | None = None
    request_timeout_seconds: float = 180.0
    agent_timeout_seconds: float = 600.0
    max_steps: int = 12
    max_tool_calls: int = 20
    max_tool_output_chars: int = 8_192
    context_limit_tokens: int = 32_768
    max_completion_tokens: int = 12_000
    plan_max_completion_tokens: int = 6_000
    code_max_completion_tokens: int = 12_000
    synthesis_reserved_steps: int = 3
    max_retries: int = 2
    retry_backoff_seconds: float = 0.25
    rag_base_url: str | None = None
    rag_api_key: str | None = None

    @classmethod
    def from_env(cls) -> "AgentSettings":
        return cls(
            model=os.getenv("AGENT_MODEL", cls.model),
            openai_base_url=os.getenv("OPENAI_BASE_URL", cls.openai_base_url).rstrip("/"),
            openai_api_key=os.getenv("OPENAI_API_KEY"),
            fallback_model=os.getenv("AGENT_FALLBACK_MODEL") or None,
            fallback_openai_base_url=os.getenv("FALLBACK_OPENAI_BASE_URL", "").rstrip("/") or None,
            fallback_openai_api_key=os.getenv("FALLBACK_OPENAI_API_KEY") or None,
            request_timeout_seconds=float(
                os.getenv("MODEL_REQUEST_TIMEOUT_SECONDS", str(cls.request_timeout_seconds))
            ),
            agent_timeout_seconds=float(
                os.getenv("AGENT_TIMEOUT_SECONDS", str(cls.agent_timeout_seconds))
            ),
            max_steps=int(os.getenv("AGENT_MAX_STEPS", str(cls.max_steps))),
            max_tool_calls=int(os.getenv("AGENT_MAX_TOOL_CALLS", str(cls.max_tool_calls))),
            max_tool_output_chars=int(
                os.getenv("MAX_TOOL_OUTPUT_CHARS", str(cls.max_tool_output_chars))
            ),
            context_limit_tokens=int(
                os.getenv("MODEL_CONTEXT_LIMIT", str(cls.context_limit_tokens))
            ),
            max_completion_tokens=int(
                os.getenv("MODEL_MAX_COMPLETION_TOKENS", str(cls.max_completion_tokens))
            ),
            plan_max_completion_tokens=int(
                os.getenv(
                    "PLAN_MAX_COMPLETION_TOKENS",
                    os.getenv("MODEL_MAX_COMPLETION_TOKENS", str(cls.plan_max_completion_tokens)),
                )
            ),
            code_max_completion_tokens=int(
                os.getenv(
                    "CODE_MAX_COMPLETION_TOKENS",
                    os.getenv("MODEL_MAX_COMPLETION_TOKENS", str(cls.code_max_completion_tokens)),
                )
            ),
            synthesis_reserved_steps=int(
                os.getenv("AGENT_SYNTHESIS_RESERVED_STEPS", str(cls.synthesis_reserved_steps))
            ),
            max_retries=int(os.getenv("MODEL_MAX_RETRIES", str(cls.max_retries))),
            retry_backoff_seconds=float(
                os.getenv("MODEL_RETRY_BACKOFF_SECONDS", str(cls.retry_backoff_seconds))
            ),
            rag_base_url=os.getenv("RAG_BASE_URL", "").rstrip("/") or None,
            rag_api_key=os.getenv("RAG_API_KEY"),
        )

    def model_endpoints(self) -> list[ModelEndpoint]:
        endpoints = [
            ModelEndpoint(
                model=self.model,
                openai_base_url=self.openai_base_url,
                openai_api_key=self.openai_api_key,
            )
        ]
        if self.fallback_model:
            endpoints.append(
                ModelEndpoint(
                    model=self.fallback_model,
                    openai_base_url=self.fallback_openai_base_url or self.openai_base_url,
                    openai_api_key=self.fallback_openai_api_key or self.openai_api_key,
                )
            )
        return endpoints
