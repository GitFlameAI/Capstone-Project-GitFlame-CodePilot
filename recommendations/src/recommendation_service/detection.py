from pathlib import PurePosixPath

from recommendation_service.models import RepoFile, TechnologyInventory


LANGUAGE_EXTENSIONS = {
    ".py": "python",
    ".go": "go",
    ".js": "javascript",
    ".jsx": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".vue": "vue",
    ".java": "java",
    ".kt": "kotlin",
    ".kts": "kotlin",
    ".rs": "rust",
    ".rb": "ruby",
    ".php": "php",
    ".cs": "csharp",
    ".c": "c",
    ".h": "c",
    ".cc": "cpp",
    ".cpp": "cpp",
    ".cxx": "cpp",
    ".hpp": "cpp",
    ".swift": "swift",
    ".scala": "scala",
    ".sh": "shell",
    ".sql": "sql",
}

MANIFESTS = {
    "go.mod": ("go_modules", "go"),
    "pyproject.toml": ("python", "python"),
    "requirements.txt": ("pip", "python"),
    "pipfile": ("pipenv", "python"),
    "poetry.lock": ("poetry", "python"),
    "package.json": ("npm", "javascript"),
    "package-lock.json": ("npm", "javascript"),
    "yarn.lock": ("yarn", "javascript"),
    "pnpm-lock.yaml": ("pnpm", "javascript"),
    "pom.xml": ("maven", "java"),
    "build.gradle": ("gradle", "java"),
    "build.gradle.kts": ("gradle", "kotlin"),
    "cargo.toml": ("cargo", "rust"),
    "gemfile": ("bundler", "ruby"),
    "composer.json": ("composer", "php"),
}

LANGUAGE_ANALYZERS = {
    "python": {"ruff", "semgrep"},
    "go": {"staticcheck", "gosec", "semgrep"},
    "javascript": {"eslint", "semgrep"},
    "typescript": {"eslint", "semgrep"},
    "vue": {"eslint", "semgrep"},
}


def detect_technologies(files: list[RepoFile]) -> TechnologyInventory:
    languages: set[str] = set()
    manifests: set[str] = set()
    package_managers: set[str] = set()

    for file in files:
        path = PurePosixPath(file.path)
        language = LANGUAGE_EXTENSIONS.get(path.suffix.lower())
        if language:
            languages.add(language)

        name = path.name.lower()
        manifest = MANIFESTS.get(name)
        if manifest:
            package_manager, manifest_language = manifest
            manifests.add(file.path)
            package_managers.add(package_manager)
            languages.add(manifest_language)
        elif path.suffix.lower() == ".csproj":
            manifests.add(file.path)
            package_managers.add("nuget")
            languages.add("csharp")

    analyzers = {"gitleaks", "pmd-cpd", "semgrep"}
    if package_managers:
        analyzers.add("osv-scanner")
    for language in languages:
        analyzers.update(LANGUAGE_ANALYZERS.get(language, set()))

    return TechnologyInventory(
        languages=sorted(languages),
        manifests=sorted(manifests),
        package_managers=sorted(package_managers),
        analyzer_candidates=sorted(analyzers),
    )
