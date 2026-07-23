import re
from collections.abc import Awaitable, Callable
from typing import Any

from agent_engine.context import ContextCompressor
from agent_engine.errors import RagUnavailableError, ToolExecutionError
from agent_engine.rag import RagSearch
from agent_engine.repository import RepositorySource, normalize_tool_path, parent_directories

ToolHandler = Callable[[dict[str, Any]], Awaitable[Any]]


class ToolSandbox:
    """Allowlisted, read-only tool executor with bounded arguments and outputs."""

    def __init__(
        self,
        source: RepositorySource,
        rag: RagSearch,
        compressor: ContextCompressor,
        *,
        rag_filters: dict[str, Any] | None = None,
        rag_path_allowed: Callable[[str], bool] | None = None,
        max_rag_files: int = 200,
        max_rag_snippets_per_file: int = 20,
    ) -> None:
        self.source = source
        self.rag = rag
        self.compressor = compressor
        self.evidence_paths = set(source.paths())
        self.rag_filters = rag_filters or {}
        self.rag_path_allowed = rag_path_allowed or (lambda _path: True)
        self.max_rag_files = max_rag_files
        self.max_rag_snippets_per_file = max_rag_snippets_per_file
        self._handlers: dict[str, ToolHandler] = {
            "read_file": self._read_file,
            "list_dir": self._list_dir,
            "grep": self._grep,
            "search_repository": self._search_repository,
        }

    @property
    def definitions(self) -> list[dict[str, Any]]:
        return TOOL_DEFINITIONS

    async def execute(self, name: str, arguments: dict[str, Any]) -> str:
        handler = self._handlers.get(name)
        if handler is None:
            raise ToolExecutionError(f"tool is not allowed: {name}")
        try:
            result = await handler(arguments)
        except (RagUnavailableError, ToolExecutionError):
            raise
        except Exception as exc:
            raise ToolExecutionError(f"{name} failed: {exc}") from exc
        return self.compressor.compress_tool_result(result)

    async def _read_file(self, arguments: dict[str, Any]) -> dict[str, Any]:
        path = normalize_tool_path(str(arguments.get("path", "")))
        start = _bounded_int(arguments.get("start_line", 1), minimum=1, maximum=1_000_000)
        end = _bounded_int(
            arguments.get("end_line", start + 199),
            minimum=start,
            maximum=start + 399,
        )
        lines = self.source.read(path).splitlines()
        if start > max(1, len(lines)):
            raise ToolExecutionError(f"start_line {start} is outside {path}")
        selected = lines[start - 1 : end]
        return {
            "path": path,
            "start_line": start,
            "end_line": start + len(selected) - 1,
            "content": "\n".join(
                f"{number}: {line}" for number, line in enumerate(selected, start=start)
            ),
        }

    async def _list_dir(self, arguments: dict[str, Any]) -> dict[str, Any]:
        directory = normalize_tool_path(str(arguments.get("path", "")), allow_root=True)
        max_entries = _bounded_int(arguments.get("max_entries", 100), minimum=1, maximum=500)
        prefix = f"{directory}/" if directory else ""
        entries: set[tuple[str, str]] = set()
        for path in self.source.paths():
            if not path.startswith(prefix):
                continue
            remainder = path[len(prefix) :]
            first = remainder.split("/", 1)[0]
            kind = "directory" if "/" in remainder else "file"
            entries.add((first, kind))
        if not entries and directory not in {
            parent for path in self.source.paths() for parent in parent_directories(path)
        }:
            raise ToolExecutionError(f"directory is unavailable: {directory or '.'}")
        ordered = sorted(entries)[:max_entries]
        return {
            "path": directory or ".",
            "entries": [{"name": name, "type": kind} for name, kind in ordered],
            "truncated": len(entries) > len(ordered),
        }

    async def _grep(self, arguments: dict[str, Any]) -> dict[str, Any]:
        pattern = str(arguments.get("pattern", ""))
        if not pattern or len(pattern) > 200:
            raise ToolExecutionError("grep pattern must contain 1 to 200 characters")
        directory = normalize_tool_path(str(arguments.get("path", "")), allow_root=True)
        max_results = _bounded_int(arguments.get("max_results", 50), minimum=1, maximum=200)
        try:
            matcher = re.compile(pattern)
        except re.error as exc:
            raise ToolExecutionError(f"invalid grep pattern: {exc}") from exc
        prefix = f"{directory}/" if directory else ""
        matches = []
        for path in self.source.paths():
            if not path.startswith(prefix):
                continue
            for number, line in enumerate(self.source.read(path).splitlines(), start=1):
                if matcher.search(line):
                    matches.append({"path": path, "line": number, "content": line[:1_000]})
                    if len(matches) >= max_results:
                        return {"matches": matches, "truncated": True}
        return {"matches": matches, "truncated": False}

    async def _search_repository(self, arguments: dict[str, Any]) -> dict[str, Any]:
        query = str(arguments.get("query", "")).strip()
        top_k = _bounded_int(arguments.get("top_k", 10), minimum=1, maximum=50)
        filters = arguments.get("filters")
        if filters is not None and not isinstance(filters, dict):
            raise ToolExecutionError("RAG filters must be an object")
        effective_filters = {**(filters or {}), **self.rag_filters}
        results = await self.rag.search(
            query=query,
            top_k=top_k,
            filters=effective_filters or None,
        )
        bounded_results = []
        snippets_by_path: dict[str, int] = {}
        for result in results:
            if not self.rag_path_allowed(result.path):
                continue
            if result.path not in snippets_by_path and len(snippets_by_path) >= self.max_rag_files:
                continue
            count = snippets_by_path.get(result.path, 0)
            if count >= self.max_rag_snippets_per_file:
                continue
            snippets_by_path[result.path] = count + 1
            bounded_results.append(result)
        results = bounded_results
        self.evidence_paths.update(result.path for result in results)
        return {
            "status": "completed" if results else "empty",
            "results": [result.model_dump(mode="json") for result in results],
        }


def _bounded_int(value: Any, *, minimum: int, maximum: int) -> int:
    if isinstance(value, bool):
        raise ToolExecutionError("boolean is not a valid integer tool argument")
    try:
        parsed = int(value)
    except (TypeError, ValueError) as exc:
        raise ToolExecutionError("tool argument must be an integer") from exc
    if not minimum <= parsed <= maximum:
        raise ToolExecutionError(f"tool argument must be between {minimum} and {maximum}")
    return parsed


TOOL_DEFINITIONS = [
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read a bounded line range from a supplied repository file.",
            "parameters": {
                "type": "object",
                "additionalProperties": False,
                "required": ["path"],
                "properties": {
                    "path": {"type": "string"},
                    "start_line": {"type": "integer", "minimum": 1},
                    "end_line": {"type": "integer", "minimum": 1},
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "list_dir",
            "description": "List supplied files and directories below a repository path.",
            "parameters": {
                "type": "object",
                "additionalProperties": False,
                "properties": {
                    "path": {"type": "string", "default": ""},
                    "max_entries": {"type": "integer", "minimum": 1, "maximum": 500},
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "grep",
            "description": "Search supplied repository text and return bounded path/line matches.",
            "parameters": {
                "type": "object",
                "additionalProperties": False,
                "required": ["pattern"],
                "properties": {
                    "pattern": {"type": "string"},
                    "path": {"type": "string", "default": ""},
                    "max_results": {"type": "integer", "minimum": 1, "maximum": 200},
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_repository",
            "description": (
                "Search the external RAG index for semantically relevant snippets. "
                "Returns status='completed' with snippets or status='empty' with no snippets."
            ),
            "parameters": {
                "type": "object",
                "additionalProperties": False,
                "required": ["query"],
                "properties": {
                    "query": {"type": "string"},
                    "top_k": {"type": "integer", "minimum": 1, "maximum": 50},
                    "filters": {"type": "object"},
                },
            },
        },
    },
]
