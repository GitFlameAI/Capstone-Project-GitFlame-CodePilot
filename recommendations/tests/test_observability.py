import importlib
import json
import logging

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from prometheus_client import REGISTRY

from observability.context import (
    current_llm_operation,
    llm_operation_scope,
    request_id_scope,
    sanitize_request_id,
)
from observability.logging import JsonFormatter, setup_logging
from observability.metrics import metrics_payload
from observability.middleware import ObservabilityMiddleware


def _record(**extra):
    record = logging.LogRecord(
        name="test", level=logging.INFO, pathname=__file__, lineno=1,
        msg="hello", args=(), exc_info=None,
    )
    for key, value in extra.items():
        setattr(record, key, value)
    return record


def test_json_formatter_emits_one_object_with_service_and_extras():
    payload = json.loads(JsonFormatter("agent-engine").format(_record(event="rag_index", files=3)))

    assert payload["service"] == "agent-engine"
    assert payload["level"] == "INFO"
    assert payload["msg"] == "hello"
    assert payload["event"] == "rag_index"
    assert payload["files"] == 3
    assert payload["time"].endswith("Z")
    assert "request_id" not in payload


def test_json_formatter_picks_up_the_ambient_request_id():
    formatter = JsonFormatter("agent-engine")
    with request_id_scope("trace-1"):
        payload = json.loads(formatter.format(_record()))
    assert payload["request_id"] == "trace-1"


def test_setup_logging_is_idempotent():
    setup_logging("agent-engine")
    setup_logging("agent-engine")
    # Two applications built in one process (as the tests do) must not produce
    # duplicated log lines.
    assert len(logging.getLogger().handlers) == 1


@pytest.mark.parametrize(
    ("module_name", "app_path", "default_port"),
    [
        ("agent_engine.app", "agent_engine.app:app", 8001),
        ("recommendation_service.app", "recommendation_service.app:app", 8000),
    ],
)
def test_service_runner_keeps_uvicorn_on_structured_logging(
    monkeypatch, module_name, app_path, default_port
):
    module = importlib.import_module(module_name)
    captured = {}

    def fake_run(app, **kwargs):
        captured["app"] = app
        captured.update(kwargs)

    monkeypatch.delenv("PORT", raising=False)
    monkeypatch.setattr("uvicorn.run", fake_run)

    module.run()

    assert captured == {
        "app": app_path,
        "host": "0.0.0.0",
        "port": default_port,
        "log_config": None,
        "access_log": False,
    }


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("trace-1", "trace-1"),
        ("  spaced  ", "spaced"),
        ('bad\nline"quote', "badlinequote"),
        ("x" * 200, "x" * 64),
        (None, ""),
    ],
)
def test_sanitize_request_id(value, expected):
    assert sanitize_request_id(value) == expected


def test_llm_operation_scope_restores_the_previous_value():
    assert current_llm_operation() == "unknown"
    with llm_operation_scope("plan"):
        assert current_llm_operation() == "plan"
    assert current_llm_operation() == "unknown"


def _app() -> FastAPI:
    app = FastAPI()

    @app.get("/items/{item_id}")
    async def item(item_id: str) -> dict[str, str]:
        return {"item_id": item_id}

    app.add_middleware(ObservabilityMiddleware, service="test-service")
    return app


def test_middleware_echoes_the_request_id_and_logs_one_line(caplog):
    client = TestClient(_app())
    with caplog.at_level(logging.INFO, logger="observability.http"):
        response = client.get("/items/42", headers={"X-Request-ID": "trace-1"})

    assert response.status_code == 200
    assert response.headers["x-request-id"] == "trace-1"

    records = [record for record in caplog.records if getattr(record, "event", "") == "http_request"]
    assert len(records) == 1
    assert records[0].status == 200
    assert records[0].path == "/items/42"
    # The metric label must be the route template, never the concrete path.
    assert records[0].route == "/items/{item_id}"


def test_middleware_records_http_metrics_under_the_route_template():
    labels = {"method": "GET", "route": "/items/{item_id}", "status": "200"}
    before = REGISTRY.get_sample_value("codepilot_http_requests_total", labels) or 0.0

    client = TestClient(_app())
    client.get("/items/1")
    client.get("/items/2")

    after = REGISTRY.get_sample_value("codepilot_http_requests_total", labels)
    assert after == before + 2

    # Concrete identifiers must never reach a metric label: one series per
    # repository or item id would eventually take Prometheus down.
    exposition = metrics_payload()[0].decode()
    assert "/items/1" not in exposition
