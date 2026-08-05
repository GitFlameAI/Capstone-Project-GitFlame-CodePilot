import asyncio
import os
import resource
import shutil
import signal
import tempfile
import time
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator, Mapping, Sequence

from recommendation_service.models import RepoFile
from recommendation_service.settings import AnalyzerSettings


@dataclass(frozen=True)
class RepositorySnapshot:
    root: Path
    artifacts: Path
    home: Path
    paths: frozenset[str]


@dataclass(frozen=True)
class CommandResult:
    returncode: int | None
    stdout: bytes
    stderr: bytes
    duration_ms: int
    timed_out: bool = False


class SnapshotLimitError(ValueError):
    pass


class AnalyzerRunner:
    """Runs trusted analyzer binaries against untrusted repository text."""

    _ALLOWED_ENV_OVERRIDES = frozenset({"PMD_JAVA_OPTS", "XDG_CACHE_HOME"})

    def __init__(self, settings: AnalyzerSettings) -> None:
        self.settings = settings

    @contextmanager
    def snapshot(self, files: list[RepoFile]) -> Iterator[RepositorySnapshot]:
        total_bytes = sum(len(file.content.encode("utf-8")) for file in files)
        if total_bytes > self.settings.max_repository_bytes:
            raise SnapshotLimitError(
                f"repository snapshot is {total_bytes} bytes; limit is "
                f"{self.settings.max_repository_bytes} bytes"
            )

        with tempfile.TemporaryDirectory(prefix="codepilot-analyze-") as temporary:
            base = Path(temporary)
            root = base / "repository"
            artifacts = base / "artifacts"
            home = base / "home"
            root.mkdir(mode=0o700)
            artifacts.mkdir(mode=0o700)
            home.mkdir(mode=0o700)

            staged_paths: set[str] = set()
            for file in files:
                if Path(file.path).name in {
                    ".gitleaks.toml",
                    ".gitleaksignore",
                    ".semgrepignore",
                    "osv-scanner.toml",
                }:
                    continue
                destination = root.joinpath(*file.path.split("/"))
                destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                destination.write_text(file.content, encoding="utf-8")
                destination.chmod(0o400)
                staged_paths.add(file.path)

            yield RepositorySnapshot(
                root=root,
                artifacts=artifacts,
                home=home,
                paths=frozenset(staged_paths),
            )

    async def run(
        self,
        command: Sequence[str],
        snapshot: RepositorySnapshot,
        *,
        env: Mapping[str, str] | None = None,
    ) -> CommandResult:
        if not command or any(not isinstance(argument, str) for argument in command):
            raise ValueError("analyzer command must be a non-empty string sequence")
        if "\x00" in "".join(command):
            raise ValueError("analyzer command contains a null byte")

        child_env = self._environment(snapshot, env)
        executable = command[0]
        resolved = (
            executable
            if os.path.isabs(executable)
            else shutil.which(executable, path=child_env["PATH"])
        )
        if not resolved or not os.access(resolved, os.X_OK):
            raise FileNotFoundError(executable)

        stdout_path = snapshot.artifacts / "process.stdout"
        stderr_path = snapshot.artifacts / "process.stderr"
        started = time.monotonic()
        with stdout_path.open("w+b") as stdout_file, stderr_path.open("w+b") as stderr_file:
            process = await asyncio.create_subprocess_exec(
                resolved,
                *command[1:],
                cwd=snapshot.root,
                env=child_env,
                stdin=asyncio.subprocess.DEVNULL,
                stdout=stdout_file,
                stderr=stderr_file,
                start_new_session=True,
                preexec_fn=self._apply_resource_limits,
            )
            timed_out = False
            try:
                await asyncio.wait_for(
                    process.wait(), timeout=self.settings.timeout_seconds
                )
            except TimeoutError:
                timed_out = True
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                await process.wait()

            stdout_file.flush()
            stderr_file.flush()

        duration_ms = int((time.monotonic() - started) * 1000)
        return CommandResult(
            returncode=process.returncode,
            stdout=self.read_limited(stdout_path),
            stderr=self.read_limited(stderr_path),
            duration_ms=duration_ms,
            timed_out=timed_out,
        )

    def read_limited(self, path: Path) -> bytes:
        if not path.is_file():
            return b""
        size = path.stat().st_size
        if size > self.settings.max_output_bytes:
            raise ValueError(
                f"analyzer output is {size} bytes; limit is {self.settings.max_output_bytes} bytes"
            )
        return path.read_bytes()

    def _environment(
        self,
        snapshot: RepositorySnapshot,
        overrides: Mapping[str, str] | None,
    ) -> dict[str, str]:
        environment = {
            "PATH": "/opt/pmd/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "HOME": str(snapshot.home),
            "TMPDIR": str(snapshot.artifacts),
            "LANG": "C.UTF-8",
            "LC_ALL": "C.UTF-8",
            "NO_COLOR": "1",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
            "SEMGREP_SEND_METRICS": "off",
            "SEMGREP_ENABLE_METRICS": "0",
            "SEMGREP_APP_TOKEN": "",
        }
        java_home = os.getenv("JAVA_HOME")
        if not java_home and Path("/usr/lib/jvm/default-java").is_dir():
            java_home = "/usr/lib/jvm/default-java"
        if java_home:
            environment["JAVA_HOME"] = java_home
        if overrides:
            unknown = set(overrides) - self._ALLOWED_ENV_OVERRIDES
            if unknown:
                raise ValueError(
                    "unsupported analyzer environment variables: "
                    + ", ".join(sorted(unknown))
                )
            environment.update({key: value for key, value in overrides.items() if value})
        return environment

    def _apply_resource_limits(self) -> None:
        output_bytes = self.settings.max_output_bytes
        resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
        resource.setrlimit(
            resource.RLIMIT_CPU,
            (self.settings.cpu_seconds, self.settings.cpu_seconds + 1),
        )
        resource.setrlimit(resource.RLIMIT_FSIZE, (output_bytes, output_bytes))
        resource.setrlimit(resource.RLIMIT_NOFILE, (128, 128))
        os.umask(0o077)
