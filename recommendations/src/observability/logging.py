"""Structured logging for the Python services.

Before this module existed, none of the Python services configured logging at
all: the root logger had no handler, so every ``logger.info(...)`` in the agent
engine and the recommendation service was silently discarded, and warnings were
printed by the logging fallback without a timestamp or a level. Calling
``setup_logging`` at application startup is what makes those lines visible.

The output format matches the Go services exactly (one JSON object per line with
``time``, ``level``, ``service``, ``event``, ``request_id``), so a single query
can follow one user action across the whole system.
"""

from __future__ import annotations

import json
import logging
import os
import sys
from datetime import datetime, timezone

from observability.context import current_request_id

# Attributes present on every LogRecord. Anything else was passed by the caller
# through ``extra=`` and belongs in the output.
_STANDARD_RECORD_FIELDS = frozenset(
    {
        "args", "asctime", "created", "exc_info", "exc_text", "filename",
        "funcName", "levelname", "levelno", "lineno", "module", "msecs",
        "message", "msg", "name", "pathname", "process", "processName",
        "relativeCreated", "stack_info", "taskName", "thread", "threadName",
    }
)

_TEXT_FORMAT = "%(asctime)s %(levelname)-5s %(name)s %(message)s"


class JsonFormatter(logging.Formatter):
    def __init__(self, service: str) -> None:
        super().__init__()
        self.service = service

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, object] = {
            "time": datetime.fromtimestamp(record.created, timezone.utc).isoformat(
                timespec="milliseconds"
            ).replace("+00:00", "Z"),
            "level": record.levelname,
            "msg": record.getMessage(),
            "service": self.service,
            "logger": record.name,
        }
        request_id = getattr(record, "request_id", "") or current_request_id()
        if request_id:
            payload["request_id"] = request_id
        for key, value in record.__dict__.items():
            if key in _STANDARD_RECORD_FIELDS or key.startswith("_") or key == "request_id":
                continue
            payload[key] = value if isinstance(value, (str, int, float, bool, type(None))) else str(value)
        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        return json.dumps(payload, ensure_ascii=False, default=str)


def setup_logging(service: str, *, level: str | None = None, log_format: str | None = None) -> None:
    """Install the process-wide logging configuration.

    Safe to call more than once (tests build several apps in one process): the
    handler is replaced, never stacked, so log lines are never duplicated.
    """
    resolved_level = _parse_level(level or os.getenv("LOG_LEVEL", "info"))
    resolved_format = (log_format or os.getenv("LOG_FORMAT", "json")).strip().lower()

    handler = logging.StreamHandler(sys.stdout)
    if resolved_format == "text":
        handler.setFormatter(logging.Formatter(_TEXT_FORMAT))
    else:
        handler.setFormatter(JsonFormatter(service))

    root = logging.getLogger()
    for existing in list(root.handlers):
        root.removeHandler(existing)
    root.addHandler(handler)
    root.setLevel(resolved_level)

    # uvicorn installs its own handlers and its own access log. Route everything
    # through the root handler instead, and silence the access log: the
    # observability middleware already emits one structured line per request,
    # and two lines per request in two formats is worse than one.
    for name in ("uvicorn", "uvicorn.error", "uvicorn.access", "httpx", "httpcore"):
        logger = logging.getLogger(name)
        logger.handlers.clear()
        logger.propagate = True
    logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)


def _parse_level(value: str) -> int:
    # An unknown value must not stop the service from starting: a typo in an
    # environment variable is not a reason to take a deployment down.
    return {
        "debug": logging.DEBUG,
        "info": logging.INFO,
        "warn": logging.WARNING,
        "warning": logging.WARNING,
        "error": logging.ERROR,
    }.get(value.strip().lower(), logging.INFO)
