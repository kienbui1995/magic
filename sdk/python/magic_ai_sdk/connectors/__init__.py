"""MagiC Connector Framework — build platform integrations (Zalo, Nhanh.vn, Base.vn, ...).

Requires the `connectors` extra: `pip install magic-ai-sdk[connectors]`
"""

from magic_ai_sdk.connectors.auth import ApiKeyAuth, AuthHandler, OAuth2Auth, WebhookSignatureAuth
from magic_ai_sdk.connectors.base import BaseConnector
from magic_ai_sdk.connectors.errors import (
    AuthError,
    ConnectorError,
    PermanentError,
    RateLimitError,
    TransientError,
    with_retry,
)
from magic_ai_sdk.connectors.registry import ConnectorRegistry

__all__ = [
    "BaseConnector",
    "ConnectorRegistry",
    "AuthHandler",
    "ApiKeyAuth",
    "OAuth2Auth",
    "WebhookSignatureAuth",
    "ConnectorError",
    "AuthError",
    "RateLimitError",
    "TransientError",
    "PermanentError",
    "with_retry",
]
