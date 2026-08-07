import fnmatch
from typing import Any

import yaml
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from recommendation_service.models import ALL_CATEGORIES, Category, RepoFile, Severity


class AnalysisConfig(BaseModel):
    model_config = ConfigDict(extra="ignore")

    enabled: bool = True
    include: list[str] = Field(default_factory=lambda: ["**/*"])
    exclude: list[str] = Field(
        default_factory=lambda: ["node_modules/**", "dist/**", "build/**", ".git/**"]
    )


class RecommendationConfig(BaseModel):
    model_config = ConfigDict(extra="ignore")

    enabled: bool = True
    severity_threshold: Severity = Severity.LOW
    categories: list[Category] = Field(default_factory=lambda: list(ALL_CATEGORIES), min_length=1)


class RagConfig(BaseModel):
    model_config = ConfigDict(extra="ignore")

    max_files: int = Field(default=20, ge=1, le=200)
    max_file_size_kb: int = Field(default=120, ge=1, le=500)


class ServiceConfig(BaseModel):
    model_config = ConfigDict(extra="ignore")

    version: int = 1
    analysis: AnalysisConfig = Field(default_factory=AnalysisConfig)
    recommendations: RecommendationConfig = Field(default_factory=RecommendationConfig)
    rag: RagConfig = Field(default_factory=RagConfig)


class ConfigError(ValueError):
    pass


def parse_config(config_yaml: str) -> ServiceConfig:
    try:
        raw: Any = yaml.safe_load(config_yaml)
    except yaml.YAMLError as exc:
        raise ConfigError(f"invalid YAML: {exc}") from exc

    if not isinstance(raw, dict):
        raise ConfigError("config_yaml must contain a YAML mapping")

    if _contains_model_selection(raw):
        raise ConfigError("ML model selection is server-controlled and is not allowed in .yml")

    try:
        config = ServiceConfig.model_validate(raw)
    except ValidationError as exc:
        raise ConfigError(str(exc)) from exc

    if not config.analysis.enabled:
        raise ConfigError("repository analysis is disabled by .yml")
    if not config.recommendations.enabled:
        raise ConfigError("recommendations are disabled by .yml")
    return config


def filter_repo_context(files: list[RepoFile], config: ServiceConfig) -> list[RepoFile]:
    return filter_analysis_files(files, config, limit=config.rag.max_files)


def filter_analysis_files(
    files: list[RepoFile],
    config: ServiceConfig,
    *,
    limit: int | None = None,
) -> list[RepoFile]:
    selected = []
    max_bytes = config.rag.max_file_size_kb * 1024

    for file in sorted(files, key=lambda item: item.path):
        if len(file.content.encode("utf-8")) > max_bytes:
            continue
        if not _matches_any(file.path, config.analysis.include):
            continue
        if _matches_any(file.path, config.analysis.exclude):
            continue
        selected.append(file)
        if limit is not None and len(selected) >= limit:
            break

    if not selected:
        raise ConfigError("no repository files remain after applying .yml filters")
    return selected


def _matches_any(path: str, patterns: list[str]) -> bool:
    return any(
        fnmatch.fnmatchcase(path, pattern)
        or (pattern.startswith("**/") and fnmatch.fnmatchcase(path, pattern[3:]))
        for pattern in patterns
    )


def _contains_model_selection(value: Any) -> bool:
    if isinstance(value, dict):
        for key, nested in value.items():
            normalized = str(key).lower().replace("-", "_")
            if normalized in {"model", "model_id", "recommendation_model", "code_generation_model"}:
                return True
            if _contains_model_selection(nested):
                return True
    elif isinstance(value, list):
        return any(_contains_model_selection(item) for item in value)
    return False
