import pytest
from pydantic import ValidationError

from agent_engine.models import GeneratedFilesContract, RagResult, parse_configuration


def test_nested_gitflame_yaml_is_adapted_to_plan_configuration():
    config = parse_configuration(
        """
analysis:
  include: [src/**]
  exclude: [src/generated/**]
rag:
  max_files: 12
  max_snippets_per_file: 4
"""
    )

    assert config.include == ["src/**"]
    assert config.exclude == ["src/generated/**"]
    assert config.max_files == 12
    assert config.max_snippets_per_file == 4


def test_generated_files_contract_accepts_valid_actions():
    contract = GeneratedFilesContract.model_validate(
        {
            "summary": "Generated implementation.",
            "files": [
                {
                    "action": "create",
                    "path": "src/new_module.py",
                    "content": "VALUE = 1\n",
                    "diff": None,
                    "explanation": "Adds the new module.",
                },
                {
                    "action": "delete",
                    "path": "src/legacy.py",
                    "explanation": "Removes unused legacy code.",
                },
            ],
        }
    )

    assert [item.action for item in contract.files] == ["create", "delete"]


def test_rag_result_contract_matches_strict_v1_response():
    result = RagResult.model_validate(
        {
            "path": "src/auth.py",
            "start_line": 1,
            "end_line": 12,
            "score": 0.91,
            "content": "def auth():\n    return True\n",
        }
    )

    assert result.model_dump(mode="json") == {
        "path": "src/auth.py",
        "start_line": 1,
        "end_line": 12,
        "score": 0.91,
        "content": "def auth():\n    return True\n",
    }


def test_rag_result_rejects_extended_response_without_schema_update():
    with pytest.raises(ValidationError):
        RagResult.model_validate(
            {
                "path": "src/auth.py",
                "start_line": 1,
                "end_line": 12,
                "score": 0.91,
                "content": "def auth():\n    return True\n",
                "reason": "Extended v2 field is not accepted by current Agent Engine.",
                "source": "reranker",
                "repository_id": "repo-1",
                "commit_sha": "abc123",
            }
        )


@pytest.mark.parametrize(
    "payload",
    [
        {
            "summary": "Bad.",
            "files": [
                {
                    "action": "modify",
                    "path": "/tmp/app.py",
                    "content": "x = 1\n",
                    "explanation": "Absolute path.",
                }
            ],
        },
        {
            "summary": "Bad.",
            "files": [
                {
                    "action": "modify",
                    "path": "src/app.py",
                    "content": "",
                    "explanation": "Empty content.",
                }
            ],
        },
        {
            "summary": "Bad.",
            "files": [
                {
                    "action": "modify",
                    "path": "src/app.py",
                    "content": "x = 1\n",
                    "explanation": "First.",
                },
                {
                    "action": "delete",
                    "path": "src/app.py",
                    "explanation": "Duplicate.",
                },
            ],
        },
    ],
)
def test_generated_files_contract_rejects_invalid_payloads(payload):
    with pytest.raises(ValueError):
        GeneratedFilesContract.model_validate(payload)
