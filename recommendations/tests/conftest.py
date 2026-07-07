import json

import httpx
import pytest
import pytest_asyncio

from recommendation_service.app import create_app
from recommendation_service.model_client import InferenceMetrics
from recommendation_service.models import RecommendationResponse
from recommendation_service.settings import Settings

VALID_RESPONSE = RecommendationResponse.model_validate(
    {
        "summary": "One supported issue was found.",
        "recommendations": [
            {
                "severity": "high",
                "category": "security",
                "file": "src/app.py",
                "line": 2,
                "problem": "User input is interpolated into a SQL query.",
                "suggestion": "Use a parameterized query.",
                "confidence": 0.98,
            }
        ],
    }
)


class FakeModelClient:
    def __init__(self, response=VALID_RESPONSE, error: Exception | None = None, ready=True):
        self.response = response
        self.error = error
        self.is_ready = ready
        self.last_user_prompt = None
        self.last_schema = None

    async def ready(self) -> bool:
        return self.is_ready

    async def analyze(self, *, system_prompt, user_prompt, response_schema):
        if self.error:
            raise self.error
        self.last_user_prompt = user_prompt
        self.last_schema = response_schema
        return self.response, InferenceMetrics()


@pytest.fixture
def config_yaml() -> str:
    return """
version: 1
analysis:
  enabled: true
  include:
    - src/**
  exclude:
    - src/generated/**
recommendations:
  enabled: true
  severity_threshold: medium
  categories:
    - security
    - performance
rag:
  max_files: 10
  max_file_size_kb: 20
"""


@pytest.fixture
def request_payload(config_yaml) -> dict:
    return {
        "config_yaml": config_yaml,
        "repo_context": [
            {
                "path": "src/app.py",
                "content": (
                    "def lookup(user):\n"
                    "    return db.execute(f\"SELECT * FROM users WHERE id={user}\")\n"
                ),
            },
            {"path": "src/generated/vendor.py", "content": "ignore = True\n"},
        ],
    }


@pytest.fixture
def fake_model_client() -> FakeModelClient:
    return FakeModelClient()


@pytest_asyncio.fixture
async def api_client(fake_model_client):
    app = create_app(settings=Settings(), model_client=fake_model_client)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        yield client


def openai_response(content: dict, **usage) -> dict:
    return {
        "choices": [{"message": {"content": json.dumps(content)}}],
        "usage": usage,
    }
