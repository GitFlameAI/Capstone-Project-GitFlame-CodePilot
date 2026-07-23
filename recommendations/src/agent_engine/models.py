from pathlib import PurePath
from typing import Any, Literal

import yaml
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from agent_engine.errors import ConfigurationError


def _safe_repo_path(value: str) -> str:
    normalized = value.replace("\\", "/").removeprefix("./").strip("/")
    if (
        not normalized
        or value.startswith(("/", "\\"))
        or ".." in normalized.split("/")
        or ":" in PurePath(normalized).parts[0]
    ):
        raise ValueError("path must be a safe repository-relative path")
    if normalized == ".git" or normalized.startswith(".git/"):
        raise ValueError(".git paths are not allowed")
    return normalized


class Issue(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: str = Field(min_length=1, max_length=200)
    title: str = Field(min_length=1, max_length=500)
    body: str = Field(default="", max_length=100_000)


class Repository(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: str = Field(min_length=1, max_length=200)
    default_branch: str = Field(default="main", min_length=1, max_length=200)
    commit_sha: str | None = Field(default=None, max_length=200)


class RepositoryFile(BaseModel):
    model_config = ConfigDict(extra="forbid")

    path: str = Field(min_length=1, max_length=500)
    content: str = Field(max_length=500_000)

    @field_validator("path")
    @classmethod
    def validate_path(cls, value: str) -> str:
        return _safe_repo_path(value)


class PlanConfiguration(BaseModel):
    model_config = ConfigDict(extra="ignore")

    include: list[str] = Field(default_factory=lambda: ["**/*"], max_length=100)
    exclude: list[str] = Field(
        default_factory=lambda: [
            ".git/**",
            "node_modules/**",
            "dist/**",
            "build/**",
            "*.pem",
            "*.key",
            ".env*",
        ],
        max_length=100,
    )
    max_files: int = Field(default=20, ge=1, le=200)
    max_snippets_per_file: int = Field(default=3, ge=1, le=20)


def parse_configuration(value: str) -> PlanConfiguration:
    try:
        raw = yaml.safe_load(value)
    except yaml.YAMLError as exc:
        raise ConfigurationError(f"invalid YAML: {exc}") from exc
    if raw is None:
        raw = {}
    if not isinstance(raw, dict):
        raise ConfigurationError("configuration_yaml must contain a YAML mapping")
    if _contains_model_selection(raw):
        raise ConfigurationError("model selection is operator-controlled")
    normalized = dict(raw)
    analysis = raw.get("analysis")
    if isinstance(analysis, dict):
        for key in ("include", "exclude"):
            if key in analysis:
                normalized.setdefault(key, analysis[key])
    rag = raw.get("rag")
    if isinstance(rag, dict):
        for key in ("max_files", "max_snippets_per_file"):
            if key in rag:
                normalized.setdefault(key, rag[key])
    try:
        return PlanConfiguration.model_validate(normalized)
    except ValueError as exc:
        raise ConfigurationError(str(exc)) from exc


def _contains_model_selection(value: Any) -> bool:
    if isinstance(value, dict):
        for key, nested in value.items():
            normalized = str(key).lower().replace("-", "_")
            if normalized in {"model", "model_id", "agent_model", "llm_model"}:
                return True
            if _contains_model_selection(nested):
                return True
    if isinstance(value, list):
        return any(_contains_model_selection(item) for item in value)
    return False


class GeneratePlanRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    request_id: str = Field(min_length=1, max_length=200)
    issue: Issue
    repository: Repository
    configuration_yaml: str = Field(default="{}", max_length=100_000)
    repository_files: list[RepositoryFile] = Field(default_factory=list, max_length=2_000)
    previous_plan: str | None = Field(default=None, max_length=200_000)
    correction_feedback: str | None = Field(default=None, max_length=50_000)

    @field_validator("repository_files")
    @classmethod
    def paths_must_be_unique(cls, value: list[RepositoryFile]) -> list[RepositoryFile]:
        paths = [item.path for item in value]
        if len(paths) != len(set(paths)):
            raise ValueError("repository_files contains duplicate paths")
        return value

    @model_validator(mode="after")
    def validate_correction_pair(self) -> "GeneratePlanRequest":
        if bool(self.previous_plan) != bool(self.correction_feedback):
            raise ValueError("previous_plan and correction_feedback must be supplied together")
        return self


class GenerateFilesRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    request_id: str = Field(min_length=1, max_length=200)
    issue: Issue
    repository: Repository
    approved_plan_markdown: str = Field(min_length=1, max_length=200_000)
    configuration_yaml: str = Field(default="{}", max_length=100_000)
    repository_files: list[RepositoryFile] = Field(default_factory=list, max_length=2_000)

    @field_validator("repository_files")
    @classmethod
    def paths_must_be_unique(cls, value: list[RepositoryFile]) -> list[RepositoryFile]:
        paths = [item.path for item in value]
        if len(paths) != len(set(paths)):
            raise ValueError("repository_files contains duplicate paths")
        return value


GeneratedFileAction = Literal["create", "modify", "delete"]


class GeneratedFileOperation(BaseModel):
    """A code-generation operation without source code embedded in JSON."""

    model_config = ConfigDict(extra="forbid")

    action: GeneratedFileAction
    path: str = Field(min_length=1, max_length=500)
    explanation: str = Field(min_length=1, max_length=1_000)

    @field_validator("path")
    @classmethod
    def validate_path(cls, value: str) -> str:
        return _safe_repo_path(value)


class GeneratedFileOperationsContract(BaseModel):
    """Small structured response used before plain-text file generation."""

    model_config = ConfigDict(extra="forbid")

    summary: str = Field(min_length=1, max_length=1_000)
    files: list[GeneratedFileOperation] = Field(min_length=1, max_length=50)

    @model_validator(mode="after")
    def paths_must_be_unique(self) -> "GeneratedFileOperationsContract":
        paths = [item.path for item in self.files]
        if len(paths) != len(set(paths)):
            raise ValueError("generated file operations contain duplicate paths")
        return self


class GeneratedFile(BaseModel):
    model_config = ConfigDict(extra="forbid")

    action: GeneratedFileAction
    path: str = Field(min_length=1, max_length=500)
    content: str | None = Field(default=None, max_length=1_000_000)
    diff: str | None = Field(default=None, max_length=1_000_000)
    explanation: str = Field(min_length=1, max_length=1_000)

    @field_validator("path")
    @classmethod
    def validate_path(cls, value: str) -> str:
        return _safe_repo_path(value)

    @model_validator(mode="after")
    def validate_action_payload(self) -> "GeneratedFile":
        if self.action in {"create", "modify"} and not (self.content or "").strip():
            raise ValueError("content is required for create and modify actions")
        if self.action == "delete" and (
            (self.content or "").strip() or (self.diff or "").strip()
        ):
            raise ValueError("delete actions must not include content or diff")
        return self


class RelevantFile(BaseModel):
    model_config = ConfigDict(extra="forbid")

    path: str
    reason: str
    create: bool = False


class Usage(BaseModel):
    model_config = ConfigDict(extra="forbid")

    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0
    tool_calls: int = 0
    reasoning_chars: int = 0
    generation_time_seconds: float = 0.0


class GeneratedFilesContract(BaseModel):
    model_config = ConfigDict(extra="forbid")

    summary: str = Field(min_length=1, max_length=1_000)
    files: list[GeneratedFile] = Field(min_length=1, max_length=50)

    @model_validator(mode="after")
    def paths_must_be_unique(self) -> "GeneratedFilesContract":
        paths = [item.path for item in self.files]
        if len(paths) != len(set(paths)):
            raise ValueError("generated files response contains duplicate paths")
        return self


class GenerateFilesResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    request_id: str
    status: Literal["completed"] = "completed"
    summary: str = Field(min_length=1, max_length=1_000)
    files: list[GeneratedFile] = Field(min_length=1, max_length=50)
    model: str
    usage: Usage


class GeneratePlanResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    request_id: str
    status: Literal["completed"] = "completed"
    plan_markdown: str
    relevant_files: list[RelevantFile]
    model: str
    usage: Usage


class HealthResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    status: Literal["ok", "ready"]
    model: str
    version: str = "3.0.0"


class ErrorResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    code: str
    detail: str


class RagResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    path: str
    start_line: int = Field(ge=1)
    end_line: int = Field(ge=1)
    score: float = Field(ge=0, le=1)
    content: str = Field(max_length=50_000)

    @field_validator("path")
    @classmethod
    def validate_path(cls, value: str) -> str:
        return _safe_repo_path(value)

    @model_validator(mode="after")
    def line_range_is_ordered(self) -> "RagResult":
        if self.end_line < self.start_line:
            raise ValueError("end_line must be greater than or equal to start_line")
        return self
