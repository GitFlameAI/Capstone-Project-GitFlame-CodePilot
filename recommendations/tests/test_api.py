import pytest

from recommendation_service.model_client import (
    ModelOutputError,
    ModelTimeoutError,
    ModelUnavailableError,
)
from recommendation_service.models import RecommendationResponse


@pytest.mark.asyncio
async def test_health_and_ready(api_client):
    health = await api_client.get("/health")
    ready = await api_client.get("/ready")

    assert health.status_code == 200
    assert health.json()["status"] == "ok"
    assert ready.status_code == 200
    assert ready.json()["status"] == "ready"


@pytest.mark.asyncio
async def test_analyze_returns_strict_response(api_client, fake_model_client, request_payload):
    response = await api_client.post("/v1/recommendations/analyze", json=request_payload)

    assert response.status_code == 200
    assert response.json()["recommendations"][0]["file"] == "src/app.py"
    assert "src/generated/vendor.py" not in fake_model_client.last_user_prompt
    assert fake_model_client.last_schema["additionalProperties"] is False


@pytest.mark.asyncio
async def test_invalid_model_file_reference_returns_502(
    api_client, fake_model_client, request_payload
):
    fake_model_client.response = RecommendationResponse.model_validate(
        {
            "summary": "Invalid reference.",
            "recommendations": [
                {
                    "severity": "high",
                    "category": "security",
                    "file": "missing.py",
                    "line": 1,
                    "problem": "Invented.",
                    "suggestion": "Do not invent.",
                    "confidence": 0.5,
                }
            ],
        }
    )

    response = await api_client.post("/v1/recommendations/analyze", json=request_payload)

    assert response.status_code == 502
    assert "unknown or excluded file" in response.json()["detail"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("error", "status_code"),
    [
        (ModelUnavailableError("offline"), 503),
        (ModelTimeoutError("slow"), 504),
        (ModelOutputError("invalid"), 502),
    ],
)
async def test_model_errors_have_no_fallback(
    api_client, fake_model_client, request_payload, error, status_code
):
    fake_model_client.error = error

    response = await api_client.post("/v1/recommendations/analyze", json=request_payload)

    assert response.status_code == status_code
    assert set(response.json()) == {"detail"}


@pytest.mark.asyncio
async def test_invalid_config_returns_422(api_client, request_payload):
    request_payload["config_yaml"] = "recommendations:\n  enabled: false\n"

    response = await api_client.post("/v1/recommendations/analyze", json=request_payload)

    assert response.status_code == 422

