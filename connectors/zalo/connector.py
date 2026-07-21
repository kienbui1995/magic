"""ZaloConnector — basic Zalo Official Account integration.

Supports sending text messages and receiving inbound user messages via
webhook. Implements `BaseConnector` from the magic Connector Framework
(`magic_ai_sdk.connectors`).

Reference (verified against Zalo's official docs, 2026):
- Send message:  POST https://openapi.zalo.me/v3.0/oa/message/cs
                 header `access_token`, body {"recipient": {...}, "message": {...}}
- Refresh token: POST https://oauth.zaloapp.com/v4/oa/access_token
                 header `secret_key`, form body {app_id, refresh_token, grant_type=refresh_token}
- Webhook signature: header `X-ZEvent-Signature` = sha256(app_id + raw_body + timestamp + oa_secret_key)
  (plain SHA256 over the concatenated string, NOT HMAC — this is Zalo's own scheme)

Access tokens expire after ~1 hour; refresh tokens are single-use and valid ~3 months.
Always re-check field names/limits against https://developers.zalo.me before production use —
Zalo's API has changed shape across versions.
"""

import asyncio
import hashlib
import hmac
import json
import time
from typing import Any, Callable, Mapping

import httpx

from magic_ai_sdk.connectors.base import BaseConnector
from magic_ai_sdk.connectors.errors import AuthError, PermanentError, RateLimitError, TransientError, with_retry

SEND_MESSAGE_URL = "https://openapi.zalo.me/v3.0/oa/message/cs"
REFRESH_TOKEN_URL = "https://oauth.zaloapp.com/v4/oa/access_token"

# Zalo error codes that mean "you're calling too fast" — check against the
# current error code table at https://developers.zalo.me before relying on this.
_RATE_LIMIT_ERROR_CODES = {-32, -128}

# Zalo error codes meaning the access token is invalid/expired — worth a retry
# with a freshly refreshed token rather than failing the caller outright.
_TOKEN_EXPIRED_ERROR_CODES = {-203, -216}


class ZaloConnector(BaseConnector):
    name = "zalo"

    def __init__(self, config: dict[str, Any], on_token_refreshed: Callable[[dict[str, str]], None] | None = None):
        super().__init__(config)
        self.app_id: str = config["app_id"]
        self.app_secret: str = config.get("app_secret", "")
        self.oa_secret_key: str = config.get("oa_secret_key", "")
        self._access_token: str = config.get("access_token", "")
        self._refresh_token: str = config.get("refresh_token", "")
        # Zalo refresh tokens are single-use (rotated on every refresh): without
        # persisting the new pair, a process restart reloads the stale refresh_token
        # from static config and permanently breaks auth. Caller wires this to
        # whatever storage it uses (DB, secret manager, config file...).
        self._on_token_refreshed = on_token_refreshed
        # None = expiry unknown (token supplied directly via config, never refreshed by us yet).
        # We only proactively refresh once we've fetched a token ourselves and know its expiry.
        self._expires_at: float | None = None
        self._client: httpx.AsyncClient | None = None
        # Zalo refresh tokens are single-use: two concurrent refreshes would have
        # the second one fail against an already-invalidated token. Serialize.
        self._token_lock = asyncio.Lock()

    async def connect(self) -> bool:
        if not self._access_token:
            raise AuthError("no access_token configured; obtain one via Zalo OA app authorization first")
        self._client = httpx.AsyncClient(timeout=10.0)
        return True

    async def disconnect(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _refresh_access_token(self) -> None:
        if not (self._refresh_token and self.app_secret):
            raise AuthError("cannot refresh access_token: missing refresh_token/app_secret")
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                REFRESH_TOKEN_URL,
                headers={"secret_key": self.app_secret},
                data={
                    "app_id": self.app_id,
                    "refresh_token": self._refresh_token,
                    "grant_type": "refresh_token",
                },
            )
            resp.raise_for_status()
            data = resp.json()
            if "access_token" not in data:
                raise AuthError(f"token refresh failed: {data}")
            self._access_token = data["access_token"]
            self._refresh_token = data.get("refresh_token", self._refresh_token)
            self._expires_at = time.monotonic() + float(data.get("expires_in", 3600)) - 30
            if self._on_token_refreshed is not None:
                self._on_token_refreshed({"access_token": self._access_token, "refresh_token": self._refresh_token})

    async def _ensure_token(self) -> str:
        async with self._token_lock:
            if self._expires_at is not None and time.monotonic() >= self._expires_at:
                await self._refresh_access_token()
            return self._access_token

    @with_retry(max_attempts=3, base_delay=1.0)
    async def _post(self, url: str, body: dict[str, Any]) -> dict[str, Any]:
        assert self._client is not None, "call connect() first"
        token = await self._ensure_token()
        try:
            resp = await self._client.post(url, headers={"access_token": token}, json=body)
        except httpx.RequestError as e:
            raise TransientError(f"network error calling Zalo API: {e}") from e
        if resp.status_code == 429:
            raise RateLimitError("Zalo API rate limited (HTTP 429)")
        if resp.status_code >= 500:
            raise TransientError(f"Zalo API server error: HTTP {resp.status_code}")
        data = resp.json()
        error_code = data.get("error", 0)
        if error_code in _RATE_LIMIT_ERROR_CODES:
            raise RateLimitError(f"Zalo API rate limited: {data.get('message')}")
        if error_code in _TOKEN_EXPIRED_ERROR_CODES:
            async with self._token_lock:
                self._expires_at = 0.0  # force refresh on next _ensure_token() call
            raise TransientError(f"Zalo access token expired (error {error_code}): {data.get('message')}")
        if error_code:
            raise PermanentError(f"Zalo API error {error_code}: {data.get('message')}")
        return data

    async def execute(self, operation: str, params: dict[str, Any]) -> Any:
        if operation == "send_text_message":
            return await self._post(
                SEND_MESSAGE_URL,
                {
                    "recipient": {"user_id": params["user_id"]},
                    "message": {"text": params["text"]},
                },
            )
        if operation == "send_template_message":
            return await self._post(
                SEND_MESSAGE_URL,
                {
                    "recipient": {"user_id": params["user_id"]},
                    "message": {"attachment": params["attachment"]},
                },
            )
        raise PermanentError(f"unsupported operation: {operation}")

    def map_to_common_schema(self, raw_data: Any, entity_type: str) -> dict[str, Any]:
        if entity_type != "message":
            raise PermanentError(f"ZaloConnector cannot map entity_type={entity_type!r}")

        sender_id = raw_data.get("sender", {}).get("id", "")
        recipient_id = raw_data.get("recipient", {}).get("id", "")
        message = raw_data.get("message", {})
        return {
            "id": message.get("msg_id", ""),
            "conversation_id": sender_id,
            "direction": "inbound",
            "sender_id": sender_id,
            "text": message.get("text"),
            "attachments": message.get("attachments", []),
            "sent_at": raw_data.get("timestamp"),
            "metadata": {"oa_id": recipient_id, "event_name": raw_data.get("event_name")},
        }

    def map_from_common_schema(self, common_data: dict[str, Any], entity_type: str) -> Any:
        if entity_type != "message":
            raise PermanentError(f"ZaloConnector cannot map entity_type={entity_type!r}")
        return {
            "user_id": common_data.get("sender_id") or common_data.get("conversation_id"),
            "text": common_data.get("text", ""),
        }

    def verify_webhook_signature(self, headers: Mapping[str, str], raw_body: bytes) -> bool:
        if not self.oa_secret_key:
            raise AuthError("oa_secret_key not configured — cannot verify webhook signatures")

        # headers.get() is exact-case; HTTP header names are case-insensitive and
        # callers may pass a plain dict (not an email.message.Message), so scan.
        signature = next((v for k, v in headers.items() if k.lower() == "x-zevent-signature"), "")
        if not signature:
            return False

        try:
            body_str = raw_body.decode("utf-8")
            payload = json.loads(body_str)
            if not isinstance(payload, dict):
                return False
            timestamp = str(payload.get("timestamp", ""))
        except (UnicodeDecodeError, ValueError):
            return False

        mac_input = f"{self.app_id}{body_str}{timestamp}{self.oa_secret_key}"
        expected = hashlib.sha256(mac_input.encode()).hexdigest()
        return hmac.compare_digest(expected, signature)

    def parse_webhook_event(self, payload: dict[str, Any]) -> list[dict[str, Any]]:
        event_name = payload.get("event_name", "")
        if event_name != "user_send_text":
            return []
        return [self.map_to_common_schema(payload, "message")]
