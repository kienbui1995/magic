"""Connector error hierarchy + retry helper.

Connectors talk to flaky third-party APIs (Zalo, Nhanh.vn, Base.vn...).
This module gives every connector the same vocabulary for failure and
the same backoff behavior, instead of each one inventing its own.
"""

import asyncio
import logging
import random
from functools import wraps
from typing import Callable, TypeVar

logger = logging.getLogger("magic_ai_sdk.connectors")

T = TypeVar("T")


class ConnectorError(Exception):
    """Base class for all connector errors."""


class AuthError(ConnectorError):
    """Authentication/authorization with the external platform failed."""


class RateLimitError(ConnectorError):
    """The external platform rejected the call due to rate limiting."""

    def __init__(self, message: str = "rate limited", retry_after: float | None = None):
        super().__init__(message)
        self.retry_after = retry_after


class TransientError(ConnectorError):
    """A retryable error — network blip, 5xx, timeout."""


class PermanentError(ConnectorError):
    """A non-retryable error — bad request, not found, unsupported operation."""


def with_retry(
    max_attempts: int = 3,
    base_delay: float = 0.5,
    max_delay: float = 8.0,
    retry_on: tuple[type[Exception], ...] = (TransientError, RateLimitError),
) -> Callable[[Callable[..., T]], Callable[..., T]]:
    """Decorator: retry an async connector method with exponential backoff + jitter.

    Only retries exceptions in `retry_on`. `RateLimitError.retry_after`, when set,
    is honored instead of the computed backoff.
    """

    def decorator(func: Callable[..., T]) -> Callable[..., T]:
        @wraps(func)
        async def wrapper(*args, **kwargs):
            attempt = 0
            while True:
                try:
                    return await func(*args, **kwargs)
                except retry_on as e:
                    attempt += 1
                    if attempt >= max_attempts:
                        raise
                    delay = getattr(e, "retry_after", None)
                    if delay is None:
                        delay = min(max_delay, base_delay * (2 ** (attempt - 1)))
                        delay += random.uniform(0, base_delay)
                    logger.warning(
                        "%s failed (attempt %d/%d): %s — retrying in %.1fs",
                        func.__qualname__, attempt, max_attempts, e, delay,
                    )
                    await asyncio.sleep(delay)

        return wrapper

    return decorator
