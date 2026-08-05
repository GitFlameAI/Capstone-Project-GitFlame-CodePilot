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


@dataclass(frozen=True)
class AnalyzerSettings:
    enabled: bool = True
    timeout_seconds: float = 90.0
    max_repository_bytes: int = 100 * 1024 * 1024
    max_output_bytes: int = 16 * 1024 * 1024
    memory_limit_mb: int = 1536
    cpu_seconds: int = 90
    semgrep_executable: str = "semgrep"
    gitleaks_executable: str = "gitleaks"
    osv_executable: str = "osv-scanner"
    cpd_executable: str = "pmd"
    semgrep_rules_path: str | None = None
    osv_cache_dir: str | None = None
    cpd_minimum_tokens: int = 75

    @classmethod
    def from_env(cls) -> "AnalyzerSettings":
        return cls(
            enabled=_env_bool("ANALYZERS_ENABLED", cls.enabled),
            timeout_seconds=float(
                os.getenv("ANALYZER_TIMEOUT_SECONDS", str(cls.timeout_seconds))
            ),
            max_repository_bytes=int(
                os.getenv("ANALYZER_MAX_REPOSITORY_BYTES", str(cls.max_repository_bytes))
            ),
            max_output_bytes=int(
                os.getenv("ANALYZER_MAX_OUTPUT_BYTES", str(cls.max_output_bytes))
            ),
            memory_limit_mb=int(
                os.getenv("ANALYZER_MEMORY_LIMIT_MB", str(cls.memory_limit_mb))
            ),
            cpu_seconds=int(os.getenv("ANALYZER_CPU_SECONDS", str(cls.cpu_seconds))),
            semgrep_executable=os.getenv("SEMGREP_EXECUTABLE", cls.semgrep_executable),
            gitleaks_executable=os.getenv("GITLEAKS_EXECUTABLE", cls.gitleaks_executable),
            osv_executable=os.getenv("OSV_SCANNER_EXECUTABLE", cls.osv_executable),
            cpd_executable=os.getenv("PMD_EXECUTABLE", cls.cpd_executable),
            semgrep_rules_path=os.getenv("SEMGREP_RULES_PATH") or None,
            osv_cache_dir=os.getenv("OSV_CACHE_DIR") or None,
            cpd_minimum_tokens=int(
                os.getenv("CPD_MINIMUM_TOKENS", str(cls.cpd_minimum_tokens))
            ),
        )


def _env_bool(name: str, default: bool) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}
