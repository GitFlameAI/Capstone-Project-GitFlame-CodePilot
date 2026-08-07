"""ASGI middleware that correlates, logs and measures every HTTP request.

It is written directly against the ASGI interface rather than Starlette's
``BaseHTTPMiddleware`` because that wrapper spawns a task per request and loses
context variables set inside it, which is exactly what has to survive here.
"""

from __future__ import annotations

import logging
import time

from observability.context import (
    REQUEST_ID_HEADER,
    new_request_id,
    request_id_scope,
    sanitize_request_id,
)
from observability.metrics import HTTP_REQUEST_DURATION, HTTP_REQUESTS

logger = logging.getLogger("observability.http")


class ObservabilityMiddleware:
    def __init__(self, app, *, service: str) -> None:
        self.app = app
        self.service = service

    async def __call__(self, scope, receive, send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        request_id = sanitize_request_id(_header(scope, REQUEST_ID_HEADER)) or new_request_id()
        state: dict[str, int] = {"status": 500}

        async def send_with_request_id(message) -> None:
            if message["type"] == "http.response.start":
                state["status"] = message["status"]
                message.setdefault("headers", [])
                message["headers"].append(
                    (REQUEST_ID_HEADER.encode("latin-1"), request_id.encode("latin-1"))
                )
            await send(message)

        started = time.perf_counter()
        with request_id_scope(request_id):
            try:
                await self.app(scope, receive, send_with_request_id)
            finally:
                duration = time.perf_counter() - started
                method = scope.get("method", "GET")
                # The matched route template keeps the label cardinality bounded;
                # the raw path with its identifiers only ever goes to the log.
                route = getattr(scope.get("route"), "path", None) or "unmatched"
                status = state["status"]
                HTTP_REQUESTS.labels(method=method, route=route, status=str(status)).inc()
                HTTP_REQUEST_DURATION.labels(method=method, route=route).observe(duration)
                logger.log(
                    _level_for(status),
                    "http_request",
                    extra={
                        "event": "http_request",
                        "method": method,
                        "path": scope.get("path", ""),
                        "route": route,
                        "status": status,
                        "duration_ms": int(duration * 1000),
                    },
                )


def _header(scope, name: str) -> str:
    target = name.lower().encode("latin-1")
    for key, value in scope.get("headers", []):
        if key.lower() == target:
            return value.decode("latin-1")
    return ""


def _level_for(status: int) -> int:
    if status >= 500:
        return logging.ERROR
    if status >= 400:
        return logging.WARNING
    return logging.INFO
