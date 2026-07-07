import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    model: str = "laguna"
    openai_base_url: str = "http://127.0.0.1:8000/v1"
    openai_api_key: str | None = None
    request_timeout_seconds: float = 180.0
    max_retries: int = 2
    retry_backoff_seconds: float = 0.25

    @classmethod
    def from_env(cls) -> "Settings":
        return cls(
            model=os.getenv("AGENT_MODEL", cls.model),
            openai_base_url=os.getenv("OPENAI_BASE_URL", cls.openai_base_url).rstrip("/"),
            openai_api_key=os.getenv("OPENAI_API_KEY"),
            request_timeout_seconds=float(
                os.getenv("MODEL_REQUEST_TIMEOUT_SECONDS", str(cls.request_timeout_seconds))
            ),
            max_retries=int(os.getenv("MODEL_MAX_RETRIES", str(cls.max_retries))),
            retry_backoff_seconds=float(
                os.getenv("MODEL_RETRY_BACKOFF_SECONDS", str(cls.retry_backoff_seconds))
            ),
        )

