import fnmatch
from abc import ABC, abstractmethod
from pathlib import PurePosixPath

from agent_engine.models import PlanConfiguration, RepositoryFile


class RepositorySource(ABC):
    """Read-only repository contract; clone-cache support can implement this interface later."""

    @abstractmethod
    def paths(self) -> list[str]:
        raise NotImplementedError

    @abstractmethod
    def read(self, path: str) -> str:
        raise NotImplementedError


class ProvidedFilesRepositorySource(RepositorySource):
    """Repository source backed only by files supplied by GitFlame."""

    def __init__(self, files: list[RepositoryFile], config: PlanConfiguration) -> None:
        selected: dict[str, str] = {}
        for file in sorted(files, key=lambda item: item.path):
            if path_is_allowed(file.path, config):
                selected[file.path] = file.content
        self._files = selected

    def paths(self) -> list[str]:
        return list(self._files)

    def read(self, path: str) -> str:
        try:
            return self._files[path]
        except KeyError as exc:
            raise FileNotFoundError(f"repository file is unavailable: {path}") from exc


def normalize_tool_path(value: str, *, allow_root: bool = False) -> str:
    normalized = value.replace("\\", "/").removeprefix("./").strip("/")
    if not normalized and allow_root:
        return ""
    if not normalized or value.startswith(("/", "\\")) or ".." in normalized.split("/"):
        raise ValueError("path must be repository-relative and cannot traverse parents")
    if normalized == ".git" or normalized.startswith(".git/"):
        raise ValueError(".git paths are not accessible")
    return normalized


def parent_directories(path: str) -> list[str]:
    parents = []
    current = PurePosixPath(path).parent
    while str(current) != ".":
        parents.append(str(current))
        current = current.parent
    return parents


def path_is_allowed(path: str, config: PlanConfiguration) -> bool:
    return _matches(path, config.include) and not _matches(path, config.exclude)


def _matches(path: str, patterns: list[str]) -> bool:
    return any(
        fnmatch.fnmatchcase(path, pattern)
        or (pattern.startswith("**/") and fnmatch.fnmatchcase(path, pattern[3:]))
        for pattern in patterns
    )
