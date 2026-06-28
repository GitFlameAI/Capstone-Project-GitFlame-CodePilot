import pytest

from agent_engine.models import GeneratedFilesContract, parse_configuration


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
