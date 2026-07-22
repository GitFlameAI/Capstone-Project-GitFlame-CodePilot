import json

import httpx
import pytest

from agent_engine.app import create_app
from agent_engine.errors import (
    EmptyModelOutputError,
    InferenceTimeoutError,
    InvalidPlanError,
    ModelUnavailableError,
    RagUnavailableError,
    ToolLimitExceededError,
)
from agent_engine.llm_client import ChatCompletion, CompletionUsage, ToolCall
from agent_engine.prompt import CODE_GENERATION_SYSTEM_PROMPT, build_validation_feedback
from agent_engine.settings import AgentSettings


def valid_plan(path: str = "src/auth.py") -> str:
    return f"""# Implementation Plan

## Issue Summary
Add expiration validation to authentication tokens.

## Goal
Reject expired tokens before authentication succeeds.

## Relevant Files
- `{path}`: Contains token validation behavior.

## Proposed Changes
- Add an expiration check to the existing validation path.

## Implementation Steps
1. Inspect the current token parsing and validation sequence.
2. Validate expiration before returning an authenticated identity.

## Expected Files to Change
- `{path}`: Modify the token validation flow.

## Tests and Verification
- Verify valid tokens pass and expired tokens fail.

## Risks and Open Questions
- Confirm the expected clock-skew policy with the backend team.
"""


class FakeChatClient:
    def __init__(self, completions=None, *, ready=True, error=None):
        self.completions = list(completions or [ChatCompletion(valid_plan(), "")])
        self.is_ready = ready
        self.error = error
        self.messages = []

    async def ready(self) -> bool:
        return self.is_ready

    async def complete(self, *, messages, tools):
        self.messages.append(messages)
        assert {item["function"]["name"] for item in tools} == {
            "read_file",
            "list_dir",
            "grep",
            "search_repository",
        }
        if self.error:
            raise self.error
        return self.completions.pop(0)


class FakeGeneratedFilesClient:
    def __init__(
        self,
        contract=None,
        *,
        contracts=None,
        ready=True,
        error=None,
        model="test-codegen-model",
    ):
        self.contract = contract
        self.contracts = list(contracts or [])
        self.is_ready = ready
        self.error = error
        self.model = model
        self.messages = []
        self.tools = None
        self.response_schema = None

    async def ready(self) -> bool:
        return self.is_ready

    async def complete(self, *, messages, tools, response_schema=None):
        self.messages.append(messages)
        self.tools = tools
        self.response_schema = response_schema
        if self.error:
            raise self.error
        contract = self.contracts.pop(0) if self.contracts else self.contract
        content = contract if isinstance(contract, str) else json.dumps(contract)
        return ChatCompletion(
            content,
            "hidden codegen reasoning",
            usage=CompletionUsage(120, 80, 200),
            model=self.model,
        )


@pytest.fixture
def agent_request() -> dict:
    return {
        "request_id": "task-123",
        "issue": {
            "id": "issue-42",
            "title": "Validate token expiration",
            "body": "Reject expired tokens.",
        },
        "repository": {
            "id": "repo-7",
            "default_branch": "main",
            "commit_sha": "abc123",
        },
        "configuration_yaml": "include:\n  - src/**\nexclude:\n  - vendor/**\n",
        "repository_files": [
            {
                "path": "src/auth.py",
                "content": "def validate(token):\n    return token.valid\n",
            },
            {"path": "vendor/ignored.py", "content": "secret = True\n"},
        ],
        "previous_plan": None,
        "correction_feedback": None,
    }


@pytest.fixture
def generated_files_request(agent_request) -> dict:
    return {
        "request_id": "codegen-123",
        "issue": agent_request["issue"],
        "repository": agent_request["repository"],
        "approved_plan_markdown": valid_plan(),
        "configuration_yaml": agent_request["configuration_yaml"],
        "repository_files": agent_request["repository_files"],
    }


@pytest.mark.asyncio
async def test_agent_health_ready_and_generate(agent_request):
    model = FakeChatClient(
        [
            ChatCompletion(
                valid_plan(),
                "private reasoning",
                usage=CompletionUsage(100, 50, 150),
            )
        ]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        health = await client.get("/health")
        ready = await client.get("/ready")
        response = await client.post("/v1/plans/generate", json=agent_request)

    assert health.status_code == 200
    assert ready.status_code == 200
    assert response.status_code == 200
    payload = response.json()
    assert payload["status"] == "completed"
    assert payload["relevant_files"] == [
        {
            "path": "src/auth.py",
            "reason": "Contains token validation behavior.",
            "create": False,
        }
    ]
    assert payload["usage"]["prompt_tokens"] == 100
    assert payload["usage"]["reasoning_chars"] == len("private reasoning")
    assert "private reasoning" not in payload["plan_markdown"]
    initial_prompt = model.messages[0][1]["content"]
    assert "src/auth.py" in initial_prompt
    assert "vendor/ignored.py" not in initial_prompt


@pytest.mark.asyncio
async def test_agent_executes_read_only_tool_before_plan(agent_request):
    model = FakeChatClient(
        [
            ChatCompletion(
                "",
                "",
                [ToolCall("call-1", "read_file", {"path": "src/auth.py"})],
            ),
            ChatCompletion(valid_plan(), ""),
        ]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/plans/generate", json=agent_request)

    assert response.status_code == 200
    assert response.json()["usage"]["tool_calls"] == 1
    tool_message = model.messages[1][-1]
    assert tool_message["role"] == "tool"
    assert "def validate(token)" in tool_message["content"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("error", "status", "code"),
    [
        (ModelUnavailableError("offline"), 503, "model_unavailable"),
        (RagUnavailableError("rag offline"), 503, "rag_unavailable"),
        (InvalidPlanError("bad plan"), 502, "invalid_output"),
        (EmptyModelOutputError("empty"), 502, "empty_output"),
        (ToolLimitExceededError("too many"), 422, "tool_limit_exceeded"),
        (InferenceTimeoutError("slow"), 504, "inference_timeout"),
    ],
)
async def test_agent_error_contract(agent_request, error, status, code):
    model = FakeChatClient(error=error)
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/plans/generate", json=agent_request)

    assert response.status_code == status
    assert response.json() == {"code": code, "detail": str(error)}


@pytest.mark.asyncio
async def test_agent_rejects_user_controlled_model(agent_request):
    agent_request["configuration_yaml"] = "model: attacker/model\n"
    app = create_app(settings=AgentSettings(), model_client=FakeChatClient())
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/plans/generate", json=agent_request)

    assert response.status_code == 422
    assert response.json()["code"] == "invalid_configuration"


@pytest.mark.asyncio
async def test_agent_request_validation_uses_error_contract(agent_request):
    del agent_request["issue"]["title"]
    app = create_app(settings=AgentSettings(), model_client=FakeChatClient())
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/plans/generate", json=agent_request)

    assert response.status_code == 422
    assert response.json()["code"] == "invalid_request"


@pytest.mark.asyncio
async def test_generate_files_returns_contract_without_writing_files(
    generated_files_request, tmp_path, monkeypatch
):
    monkeypatch.chdir(tmp_path)
    model = FakeGeneratedFilesClient(
        {
            "summary": "Implemented expiration validation.",
            "files": [
                {
                    "action": "modify",
                    "path": "src/auth.py",
                    "content": (
                        "def validate(token):\n"
                        "    if token.expired:\n"
                        "        return False\n"
                        "    return token.valid\n"
                    ),
                    "diff": (
                        "--- a/src/auth.py\n"
                        "+++ b/src/auth.py\n"
                        "@@\n"
                        "+    if token.expired:\n"
                        "+        return False\n"
                    ),
                    "explanation": "Adds the approved expiration guard.",
                },
                {
                    "action": "create",
                    "path": "src/token_errors.py",
                    "content": "class ExpiredTokenError(Exception):\n    pass\n",
                    "diff": None,
                    "explanation": "Adds a typed error for expired tokens.",
                },
            ],
        }
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    payload = response.json()
    assert payload["request_id"] == "codegen-123"
    assert payload["status"] == "completed"
    assert payload["model"] == "test-codegen-model"
    assert payload["usage"]["prompt_tokens"] == 120
    assert payload["usage"]["reasoning_chars"] == len("hidden codegen reasoning")
    assert payload["files"][0]["action"] == "modify"
    assert payload["files"][1]["path"] == "src/token_errors.py"
    assert model.tools == []
    assert model.response_schema["additionalProperties"] is False
    assert "hidden codegen reasoning" not in json.dumps(payload)
    assert not (tmp_path / "src" / "token_errors.py").exists()


@pytest.mark.asyncio
async def test_generate_files_merges_duplicate_paths(generated_files_request):
    model = FakeGeneratedFilesClient(
        {
            "summary": "Implemented expiration validation.",
            "files": [
                {
                    "action": "modify",
                    "path": "src/auth.py",
                    "content": "def validate(token):\n    return token.valid\n",
                    "explanation": "Keeps the original validation flow.",
                },
                {
                    "action": "modify",
                    "path": "./src/auth.py",
                    "content": (
                        "def validate(token):\n"
                        "    return token.valid and not token.expired\n"
                    ),
                    "explanation": "Adds token expiration validation.",
                },
            ],
        }
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    payload = response.json()
    assert len(payload["files"]) == 1
    assert payload["files"][0]["path"] == "src/auth.py"
    assert payload["files"][0]["content"].strip().endswith("not token.expired")
    assert "Keeps the original validation flow." in payload["files"][0]["explanation"]
    assert "Adds token expiration validation." in payload["files"][0]["explanation"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "bad_file",
    [
        {
            "action": "rename",
            "path": "src/auth.py",
            "content": "x = 1\n",
            "explanation": "Unsupported action.",
        },
        {
            "action": "modify",
            "path": "../secrets.py",
            "content": "x = 1\n",
            "explanation": "Unsafe traversal.",
        },
        {
            "action": "create",
            "path": "src/new.py",
            "content": "",
            "explanation": "Empty create content.",
        },
        {
            "action": "delete",
            "path": "src/auth.py",
            "content": "should not be here",
            "explanation": "Delete with content.",
        },
    ],
)
async def test_generate_files_rejects_invalid_model_contract(generated_files_request, bad_file):
    model = FakeGeneratedFilesClient(
        {
            "summary": "Bad output.",
            "files": [bad_file],
        }
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 502
    assert response.json()["code"] == "invalid_generated_files"


@pytest.mark.asyncio
async def test_generate_files_error_identifies_invalid_file_candidate(generated_files_request):
    model = FakeGeneratedFilesClient(
        {
            "summary": "Bad output.",
            "files": [
                {
                    "action": "create",
                    "path": "backend-python/app/__init__.py",
                    "explanation": "Adds a package initialization file.",
                }
            ],
        }
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 502
    detail = response.json()["detail"]
    assert "invalid file candidate at files.0" in detail
    assert "action=create" in detail
    assert "path=backend-python/app/__init__.py" in detail


@pytest.mark.asyncio
async def test_generate_files_repairs_missing_modify_content(generated_files_request):
    model = FakeGeneratedFilesClient(
        contracts=[
            {
                "summary": "Bad output.",
                "files": [
                    {
                        "action": "modify",
                        "path": "src/auth.py",
                        "explanation": "Adds token expiration validation.",
                    }
                ],
            },
            {
                "summary": "Validated token expiration.",
                "files": [
                    {
                        "action": "modify",
                        "path": "src/auth.py",
                        "content": (
                            "def validate(token):\n"
                            "    return token.valid and not token.expired\n"
                        ),
                        "explanation": "Rejects expired tokens before accepting valid tokens.",
                    }
                ],
            },
        ]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert body["files"][0]["content"].strip().endswith("not token.expired")
    assert len(model.messages) == 2
    assert "The previous generated files JSON was invalid" in model.messages[1][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_repairs_compressed_modify_content(generated_files_request):
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        }
    ]
    model = FakeGeneratedFilesClient(
        contracts=[
            {
                "summary": "Bad compressed output.",
                "files": [
                    {
                        "action": "modify",
                        "path": "src/auth.py",
                        "content": (
                            "import logging def validate(token): "
                            "return token.valid and not token.expired"
                        ),
                        "explanation": "Adds token expiration validation.",
                    }
                ],
            },
            {
                "summary": "Validated token expiration.",
                "files": [
                    {
                        "action": "modify",
                        "path": "src/auth.py",
                        "content": (
                            "import logging\n\n"
                            "def validate(token):\n"
                            "    logging.info('validating token')\n"
                            "    return token.valid and not token.expired\n"
                        ),
                        "explanation": "Rejects expired tokens before accepting valid tokens.",
                    }
                ],
            },
        ]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert "\n" in body["files"][0]["content"]
    assert len(model.messages) == 2
    assert "compressed or partial" in model.messages[1][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_returns_safe_original_fallback_after_repeated_partial_modify(
    generated_files_request,
):
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        }
    ]
    compressed = {
        "summary": "Bad compressed output.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging def validate(token): "
                    "return token.valid and not token.expired"
                ),
                "explanation": "Adds token expiration validation.",
            }
        ],
    }
    model = FakeGeneratedFilesClient(
        contracts=[compressed, compressed, compressed, compressed]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    payload = response.json()
    assert payload["files"][0]["path"] == "src/auth.py"
    assert (
        payload["files"][0]["content"]
        == generated_files_request["repository_files"][0]["content"]
    )
    assert "model repeatedly returned partial" in payload["files"][0]["explanation"]
    assert len(model.messages) == 4
    assert "FULL ORIGINAL TARGET FILES START" in model.messages[2][1]["content"]
    assert "Generate a focused generated-files JSON contract" in model.messages[3][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_targeted_repair_fixes_compressed_modify_content(
    generated_files_request,
):
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        }
    ]
    compressed = {
        "summary": "Bad compressed output.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging def validate(token): "
                    "return token.valid and not token.expired"
                ),
                "explanation": "Adds token expiration validation.",
            }
        ],
    }
    repaired = {
        "summary": "Validated token expiration.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging\n\n"
                    "def validate(token):\n"
                    "    logging.info('validating token')\n"
                    "    return token.valid and not token.expired\n"
                ),
                "explanation": "Rejects expired tokens before accepting valid tokens.",
            }
        ],
    }
    model = FakeGeneratedFilesClient(contracts=[compressed, compressed, repaired])
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert body["files"][0]["content"].strip().endswith("not token.expired")
    assert len(model.messages) == 3
    assert "FULL ORIGINAL TARGET FILES START" in model.messages[2][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_focused_fallback_fixes_compressed_modify_content(
    generated_files_request,
):
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        }
    ]
    compressed = {
        "summary": "Bad compressed output.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging def validate(token): "
                    "return token.valid and not token.expired"
                ),
                "explanation": "Adds token expiration validation.",
            }
        ],
    }
    repaired = {
        "summary": "Validated token expiration.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging\n\n"
                    "def validate(token):\n"
                    "    logging.info('validating token')\n"
                    "    return token.valid and not token.expired\n"
                ),
                "explanation": "Rejects expired tokens before accepting valid tokens.",
            }
        ],
    }
    model = FakeGeneratedFilesClient(
        contracts=[compressed, compressed, compressed, repaired]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert body["files"][0]["content"].strip().endswith("not token.expired")
    assert len(model.messages) == 4
    assert "Generate a focused generated-files JSON contract" in model.messages[3][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_focused_fallback_recovers_from_truncated_json(
    generated_files_request,
):
    generated_files_request["approved_plan_markdown"] = valid_plan("src/auth.py")
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        }
    ]
    truncated = '{"summary":"Consolidate auth","files":[{"action":"modify","path":"src/auth.py"'
    repaired = {
        "summary": "Validated token expiration.",
        "files": [
            {
                "action": "modify",
                "path": "src/auth.py",
                "content": (
                    "import logging\n\n"
                    "def validate(token):\n"
                    "    logging.info('validating token')\n"
                    "    return token.valid and not token.expired\n"
                ),
                "explanation": "Rejects expired tokens before accepting valid tokens.",
            }
        ],
    }
    model = FakeGeneratedFilesClient(contracts=[truncated, truncated, truncated, repaired])
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert body["files"][0]["path"] == "src/auth.py"
    assert len(model.messages) == 4
    assert "Generate a focused generated-files JSON contract" in model.messages[3][1]["content"]


@pytest.mark.asyncio
async def test_generate_files_drops_partial_modify_after_targeted_repair(
    generated_files_request,
):
    generated_files_request["repository_files"] = [
        {
            "path": "src/auth.py",
            "content": (
                "import logging\n\n"
                "def validate(token):\n"
                "    logging.info('validating token')\n"
                "    return token.valid\n"
            ),
        },
        {
            "path": "src/config.py",
            "content": "TIMEOUT = 10\n",
        },
    ]
    compressed_auth = {
        "action": "modify",
        "path": "src/auth.py",
        "content": "import logging def validate(token): return token.valid and not token.expired",
        "explanation": "Adds token expiration validation.",
    }
    valid_config = {
        "action": "modify",
        "path": "src/config.py",
        "content": "TIMEOUT = 15\n",
        "explanation": "Raises timeout for slower integrations.",
    }
    partial_contract = {
        "summary": "Mixed generated output.",
        "files": [compressed_auth, valid_config],
    }
    model = FakeGeneratedFilesClient(
        contracts=[partial_contract, partial_contract, partial_contract]
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 200
    body = response.json()
    assert [file["path"] for file in body["files"]] == ["src/config.py"]
    assert len(model.messages) == 3


def test_code_generation_prompt_distinguishes_create_and_modify_actions():
    assert "Never use action" in CODE_GENERATION_SYSTEM_PROMPT
    assert "for a path already present" in CODE_GENERATION_SYSTEM_PROMPT
    assert "`repository_file_inventory`" in CODE_GENERATION_SYSTEM_PROMPT
    assert "use action `modify`" in CODE_GENERATION_SYSTEM_PROMPT
    assert "for existing" in CODE_GENERATION_SYSTEM_PROMPT
    assert "entire final file" in CODE_GENERATION_SYSTEM_PROMPT


def test_plan_validation_feedback_includes_heading_skeleton():
    feedback = build_validation_feedback(["headings are missing, duplicated, or out of order"])

    assert "# Implementation Plan" in feedback
    assert "## Issue Summary" in feedback
    assert "## Risks and Open Questions" in feedback
    assert "no extra headings" in feedback


@pytest.mark.asyncio
async def test_generate_files_rejects_modify_for_unknown_file(generated_files_request):
    model = FakeGeneratedFilesClient(
        {
            "summary": "Bad output.",
            "files": [
                {
                    "action": "modify",
                    "path": "src/missing.py",
                    "content": "x = 1\n",
                    "explanation": "Invented file.",
                }
            ],
        }
    )
    app = create_app(settings=AgentSettings(), model_client=model)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.post("/v1/files/generate", json=generated_files_request)

    assert response.status_code == 502
    assert response.json()["code"] == "invalid_generated_files"
    assert "unknown supplied file" in response.json()["detail"]
