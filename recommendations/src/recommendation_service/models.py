from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


class Severity(StrEnum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"


SEVERITY_RANK = {
    Severity.LOW: 0,
    Severity.MEDIUM: 1,
    Severity.HIGH: 2,
}


class Category(StrEnum):
    CODE_DUPLICATION = "code_duplication"
    SECURITY = "security"
    MAINTAINABILITY = "maintainability"
    PERFORMANCE = "performance"
    ARCHITECTURE = "architecture"


ALL_CATEGORIES = list(Category)


class RepositoryReference(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: str = Field(min_length=1, max_length=200)
    commit_sha: str = Field(min_length=1, max_length=200)


class RepoFile(BaseModel):
    model_config = ConfigDict(extra="forbid")

    path: str = Field(min_length=1, max_length=500)
    content: str = Field(max_length=500_000)

    @field_validator("path")
    @classmethod
    def validate_path(cls, value: str) -> str:
        normalized = value.replace("\\", "/").removeprefix("./")
        parts = normalized.split("/")
        if (
            normalized.startswith("/")
            or "\x00" in normalized
            or any(part in {"", ".", ".."} for part in parts)
        ):
            raise ValueError("path must be a safe repository-relative path")
        return normalized


class AnalyzeRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    repository: RepositoryReference
    config_yaml: str = Field(min_length=1, max_length=100_000)
    repo_context: list[RepoFile] = Field(min_length=1, max_length=2_000)

    @field_validator("repo_context")
    @classmethod
    def paths_must_be_unique(cls, value: list[RepoFile]) -> list[RepoFile]:
        paths = [item.path for item in value]
        if len(paths) != len(set(paths)):
            raise ValueError("repo_context contains duplicate paths")
        return value


class TechnologyInventory(BaseModel):
    model_config = ConfigDict(extra="forbid")

    languages: list[str] = Field(default_factory=list)
    manifests: list[str] = Field(default_factory=list)
    package_managers: list[str] = Field(default_factory=list)
    analyzer_candidates: list[str] = Field(default_factory=list)


class Finding(BaseModel):
    """Normalized deterministic analyzer output before LLM explanation."""

    model_config = ConfigDict(extra="forbid")

    repository_id: str = Field(min_length=1, max_length=200)
    commit_sha: str = Field(min_length=1, max_length=200)
    tool: str = Field(min_length=1, max_length=100)
    rule_id: str = Field(min_length=1, max_length=300)
    category: Category
    severity: Severity
    file: str = Field(min_length=1, max_length=500)
    start_line: int = Field(ge=1)
    end_line: int = Field(ge=1)
    message: str = Field(min_length=1, max_length=2_000)
    evidence: str = Field(default="", max_length=4_000)
    related_files: list[str] = Field(default_factory=list, max_length=50)
    fingerprint: str = Field(min_length=64, max_length=64)

    @field_validator("file")
    @classmethod
    def validate_file(cls, value: str) -> str:
        return RepoFile.validate_path(value)

    @field_validator("related_files")
    @classmethod
    def validate_related_files(cls, value: list[str]) -> list[str]:
        normalized = [RepoFile.validate_path(path) for path in value]
        return list(dict.fromkeys(normalized))

    @model_validator(mode="after")
    def line_range_must_be_ordered(self) -> "Finding":
        if self.end_line < self.start_line:
            raise ValueError("end_line must be greater than or equal to start_line")
        return self


class Recommendation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    severity: Severity
    category: Category
    file: str = Field(min_length=1, max_length=500)
    line: int = Field(ge=1)
    problem: str = Field(min_length=1, max_length=800)
    suggestion: str = Field(min_length=1, max_length=800)
    confidence: float = Field(ge=0, le=1)


class RecommendationResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    summary: str = Field(min_length=1, max_length=800)
    recommendations: list[Recommendation] = Field(max_length=10)


class ErrorResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    detail: str


class HealthResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    status: str
    model: str | None = None


class AnalyzerStatus(StrEnum):
    COMPLETED = "completed"
    SKIPPED = "skipped"
    FAILED = "failed"
    TIMED_OUT = "timed_out"


class AnalyzerDiagnostic(BaseModel):
    model_config = ConfigDict(extra="forbid")

    tool: str = Field(min_length=1, max_length=100)
    status: AnalyzerStatus
    message: str = Field(min_length=1, max_length=1_000)
    duration_ms: int = Field(default=0, ge=0)


class AnalyzerReport(BaseModel):
    model_config = ConfigDict(extra="forbid")

    findings: list[Finding] = Field(default_factory=list, max_length=5_000)
    diagnostics: list[AnalyzerDiagnostic] = Field(default_factory=list, max_length=100)
