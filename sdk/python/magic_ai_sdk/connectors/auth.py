"""Auth abstractions shared across connectors.

Each connector picks the AuthHandler that matches its platform instead of
hand-rolling header/signature logic. All handlers are stateless and safe
to share across requests.
"""

import hashlib
import hmac
import time
from abc import ABC, abstractmethod
from typing import Any

import httpx


class AuthHandler(ABC):
    """Base class for connector authentication strategies."""

    @abstractmethod
    async def apply(self, request_kwargs: dict[str, Any]) -> dict[str, Any]:
        """Mutate/return httpx request kwargs (headers, params...) with credentials applied.

        Async so handlers that need to fetch/refresh a token first (OAuth2Auth)
        share the exact same interface as ones that don't (ApiKeyAuth) — callers
        can `await handler.apply(kwargs)` polymorphically either way.
        """


class ApiKeyAuth(AuthHandler):
    """Static API key sent as a header or query param."""

    def __init__(self, key: str, header: str = "Authorization", prefix: str = ""):
        self.key = key
        self.header = header
        self.prefix = prefix

    async def apply(self, request_kwargs: dict[str, Any]) -> dict[str, Any]:
        headers = dict(request_kwargs.get("headers") or {})
        headers[self.header] = f"{self.prefix}{self.key}"
        request_kwargs["headers"] = headers
        return request_kwargs


class OAuth2Auth(AuthHandler):
    """Client-credentials OAuth2 with lazy token fetch + refresh.

    Zalo/Nhanh/Base all use some flavor of bearer-token-with-expiry;
    this covers the common case without pulling in a full OAuth library.
    """

    def __init__(self, token_url: str, client_id: str, client_secret: str, leeway_seconds: float = 30.0):
        self.token_url = token_url
        self.client_id = client_id
        self.client_secret = client_secret
        self.leeway_seconds = leeway_seconds
        self._access_token: str | None = None
        self._expires_at: float = 0.0

    async def _refresh(self) -> None:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                self.token_url,
                data={
                    "grant_type": "client_credentials",
                    "client_id": self.client_id,
                    "client_secret": self.client_secret,
                },
            )
            resp.raise_for_status()
            data = resp.json()
            self._access_token = data["access_token"]
            self._expires_at = time.monotonic() + float(data.get("expires_in", 3600)) - self.leeway_seconds

    async def token(self) -> str:
        if self._access_token is None or time.monotonic() >= self._expires_at:
            await self._refresh()
        assert self._access_token is not None
        return self._access_token

    async def apply(self, request_kwargs: dict[str, Any]) -> dict[str, Any]:
        headers = dict(request_kwargs.get("headers") or {})
        headers["Authorization"] = f"Bearer {await self.token()}"
        request_kwargs["headers"] = headers
        return request_kwargs


class WebhookSignatureAuth:
    """Verifies inbound webhook signatures (HMAC-SHA256 over the raw body).

    Not an AuthHandler (nothing to "apply" to outbound requests) — used by
    connectors on the receiving side to validate that a webhook payload
    really came from the platform and wasn't spoofed/tampered with.
    """

    def __init__(self, secret: str, header: str = "X-Signature"):
        self.secret = secret
        self.header = header

    def sign(self, raw_body: bytes) -> str:
        return hmac.new(self.secret.encode(), raw_body, hashlib.sha256).hexdigest()

    def verify(self, raw_body: bytes, signature: str) -> bool:
        expected = self.sign(raw_body)
        return hmac.compare_digest(expected, signature)
