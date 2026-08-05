import hashlib

from recommendation_service.models import Category, Finding, Severity


def build_finding(
    *,
    repository_id: str,
    commit_sha: str,
    tool: str,
    rule_id: str,
    category: Category,
    severity: Severity,
    file: str,
    start_line: int,
    end_line: int,
    message: str,
    evidence: str = "",
    related_files: list[str] | None = None,
) -> Finding:
    fingerprint = finding_fingerprint(
        repository_id=repository_id,
        tool=tool,
        rule_id=rule_id,
        file=file,
        start_line=start_line,
        end_line=end_line,
    )
    return Finding(
        repository_id=repository_id,
        commit_sha=commit_sha,
        tool=tool,
        rule_id=rule_id,
        category=category,
        severity=severity,
        file=file,
        start_line=start_line,
        end_line=end_line,
        message=message,
        evidence=evidence,
        related_files=related_files or [],
        fingerprint=fingerprint,
    )


def finding_fingerprint(
    *,
    repository_id: str,
    tool: str,
    rule_id: str,
    file: str,
    start_line: int,
    end_line: int,
) -> str:
    identity = "\0".join(
        (
            repository_id.strip(),
            tool.strip().lower(),
            rule_id.strip(),
            file.replace("\\", "/").removeprefix("./"),
            str(start_line),
            str(end_line),
        )
    )
    return hashlib.sha256(identity.encode("utf-8")).hexdigest()
