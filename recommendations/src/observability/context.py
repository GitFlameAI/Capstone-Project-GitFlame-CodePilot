"""Request correlation shared by every Python service.

The backend puts an ``X-Request-ID`` header on every call it makes. Keeping that
identifier in a context variable means any log line written while handling the
request carries it automatically, without threading a parameter through the
whole call stack.
"""

from __future__ import annotations

import secrets
from contextlib import contextmanager
from contextvars import ContextVar
from typing import Iterator

REQUEST_ID_HEADER = "x-request-id"

# A caller-supplied identifier is echoed into logs and metrics, so it is bounded
# and stripped of anything that could break a log line.
_MAX_REQUEST_ID_LENGTH = 64
_ALLOWED_REQUEST_ID_CHARACTERS = frozenset(
    "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
)

_request_id: ContextVar[str] = ContextVar("request_id", default="")
_llm_operation: ContextVar[str] = ContextVar("llm_operation", default="unknown")


def new_request_id() -> str:
    return secrets.token_hex(16)


def sanitize_request_id(value: str | None) -> str:
    if not value:
        return ""
    trimmed = value.strip()[:_MAX_REQUEST_ID_LENGTH]
    return "".join(symbol for symbol in trimmed if symbol in _ALLOWED_REQUEST_ID_CHARACTERS)


def current_request_id() -> str:
    return _request_id.get()


@contextmanager
def request_id_scope(request_id: str) -> Iterator[str]:
    token = _request_id.set(request_id)
    try:
        yield request_id
    finally:
        _request_id.reset(token)


def current_llm_operation() -> str:
    return _llm_operation.get()


@contextmanager
def llm_operation_scope(operation: str) -> Iterator[str]:
    """Label the model calls made inside this block.

    The model client is shared by plan generation, code generation, and
    recommendation analysis, and it is several layers below the endpoint that
    knows which of them is running. A context variable labels the metrics
    without adding an argument to eight call sites.
    """
    token = _llm_operation.set(operation)
    try:
        yield operation
    finally:
        _llm_operation.reset(token)
