"""GoogleSheetConnector — read/write orders & customers via Google Sheets API v4.

Auth uses a Google service account (JWT bearer flow via `google-auth`) — the
sheet must be shared with the service account's email. `google-auth` handles
token minting/signing (real RSA crypto); this module only makes the actual
Sheets API calls, over httpx like the other connectors.

Reference: https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets.values
- Read:          GET    /v4/spreadsheets/{id}/values/{range}
- Update:        PUT    /v4/spreadsheets/{id}/values/{range}?valueInputOption=...
- Append a row:  POST   /v4/spreadsheets/{id}/values/{range}:append?valueInputOption=...
- Batch update:  POST   /v4/spreadsheets/{id}/values:batchUpdate
Error body shape is Google's standard `{"error": {"code", "message", "status"}}`.
"""

import asyncio
import json
from typing import Any
from urllib.parse import quote

import httpx
from google.auth.exceptions import GoogleAuthError
from google.auth.transport import Request as GoogleAuthRequestBase
from google.auth.transport import Response as GoogleAuthResponseBase
from google.oauth2 import service_account

from magic_ai_sdk.connectors.base import BaseConnector
from magic_ai_sdk.connectors.errors import AuthError, PermanentError, RateLimitError, TransientError, with_retry

API_BASE = "https://sheets.googleapis.com/v4/spreadsheets"
SCOPES = ["https://www.googleapis.com/auth/spreadsheets"]


class _HttpxAuthResponse(GoogleAuthResponseBase):
    """Adapts an httpx.Response to google.auth.transport.Response."""

    def __init__(self, response: httpx.Response):
        self._response = response

    @property
    def status(self) -> int:
        return self._response.status_code

    @property
    def headers(self) -> dict[str, str]:
        return dict(self._response.headers)

    @property
    def data(self) -> bytes:
        return self._response.content


class _HttpxAuthRequest(GoogleAuthRequestBase):
    """Adapts httpx (sync client) to the google.auth.transport.Request interface,
    so credential refresh doesn't need the separate `requests` library — the rest
    of this connector already standardizes on httpx."""

    def __call__(self, url, method="GET", body=None, headers=None, timeout=None, **kwargs) -> _HttpxAuthResponse:
        with httpx.Client(timeout=timeout or 15.0) as client:
            resp = client.request(method, url, content=body, headers=headers)
        return _HttpxAuthResponse(resp)

# Google's standard error `status` values that mean "back off and retry".
_RATE_LIMIT_STATUSES = {"RESOURCE_EXHAUSTED"}
_TRANSIENT_STATUSES = {"UNAVAILABLE", "INTERNAL", "DEADLINE_EXCEEDED", "ABORTED"}

_CUSTOMER_FIELDS = ["id", "name", "phone", "email", "address"]
_ORDER_FIELDS = ["id", "customer_id", "status", "total_amount", "shipping_fee", "created_at"]


def _safe_float(value: Any) -> float:
    """Malformed sheet cells shouldn't crash the whole mapping — fall back to 0.0."""
    try:
        return float(value) if value else 0.0
    except (TypeError, ValueError):
        return 0.0


def _as_dict(data: Any) -> dict[str, Any]:
    """Workflows commonly pass a Pydantic Common Schema model here, not a raw dict."""
    if isinstance(data, dict):
        return data
    if hasattr(data, "model_dump"):
        return data.model_dump()
    if hasattr(data, "dict"):
        return data.dict()
    raise PermanentError("common_data must be a dict or a Pydantic model")


class GoogleSheetConnector(BaseConnector):
    name = "google_sheet"

    def __init__(self, config: dict[str, Any]):
        super().__init__(config)
        self.spreadsheet_id: str = config["spreadsheet_id"]
        service_account_info = config.get("service_account_info")
        service_account_file = config.get("service_account_file")
        if service_account_info:
            self._credentials = service_account.Credentials.from_service_account_info(
                service_account_info, scopes=SCOPES
            )
        elif service_account_file:
            self._credentials = service_account.Credentials.from_service_account_file(
                service_account_file, scopes=SCOPES
            )
        else:
            raise AuthError("GoogleSheetConnector requires service_account_info or service_account_file")
        self._client: httpx.AsyncClient | None = None
        self._token_lock = asyncio.Lock()

    async def connect(self) -> bool:
        self._client = httpx.AsyncClient(timeout=15.0)
        try:
            await self._ensure_token()
        except Exception:
            await self._client.aclose()
            self._client = None
            raise
        return True

    async def disconnect(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def _ensure_token(self) -> str:
        async with self._token_lock:
            if not self._credentials.valid:
                # google-auth's refresh() is a blocking network call; run off the event loop.
                await asyncio.to_thread(self._credentials.refresh, _HttpxAuthRequest())
            return self._credentials.token

    @with_retry(max_attempts=3, base_delay=1.0)
    async def _request(self, method: str, url: str, **kwargs: Any) -> dict[str, Any]:
        if self._client is None:
            raise PermanentError("connector not connected — call connect() first")

        try:
            token = await self._ensure_token()
            headers = {"Authorization": f"Bearer {token}"}
            resp = await self._client.request(method, url, headers=headers, **kwargs)
        except httpx.RequestError as e:
            raise TransientError(f"network error calling Google Sheets API: {e}") from e
        except GoogleAuthError as e:
            # Covers RefreshError and friends — usually a permanently bad/revoked
            # service account key, not something a retry will fix, so don't retry.
            raise AuthError(f"Google auth error: {e}") from e

        if resp.is_success:
            return resp.json() if resp.content else {}

        try:
            error = resp.json().get("error", {})
        except ValueError:
            error = {}
        status = error.get("status", "")
        message = error.get("message", f"HTTP {resp.status_code}")

        if resp.status_code == 401:
            # Token expired/revoked mid-flight (credentials.valid was true when we
            # checked, but Google invalidated it since) — clear it so the retry
            # forces a fresh refresh instead of reusing the same bad token.
            self._credentials.token = None
            raise TransientError(f"Google Sheets API token expired or invalid: {message}")
        if resp.status_code == 429 or status in _RATE_LIMIT_STATUSES:
            raise RateLimitError(f"Google Sheets API rate limited: {message}")
        if resp.status_code >= 500 or status in _TRANSIENT_STATUSES:
            raise TransientError(f"Google Sheets API transient error: {message}")
        raise PermanentError(f"Google Sheets API error ({status or resp.status_code}): {message}")

    def _range_url(self, a1_range: str) -> str:
        return f"{API_BASE}/{self.spreadsheet_id}/values/{quote(a1_range, safe='')}"

    async def execute(self, operation: str, params: dict[str, Any]) -> Any:
        if operation == "read_range":
            return await self._request("GET", self._range_url(params["range"]))

        if operation == "update_range":
            return await self._request(
                "PUT",
                self._range_url(params["range"]),
                params={"valueInputOption": params.get("value_input_option", "USER_ENTERED")},
                json={"values": params["values"]},
            )

        if operation == "append_row":
            return await self._request(
                "POST",
                self._range_url(params["range"]) + ":append",
                params={"valueInputOption": params.get("value_input_option", "USER_ENTERED")},
                json={"values": params["values"]},
            )

        if operation == "batch_update":
            return await self._request(
                "POST",
                f"{API_BASE}/{self.spreadsheet_id}/values:batchUpdate",
                json={
                    "valueInputOption": params.get("value_input_option", "USER_ENTERED"),
                    "data": [{"range": r["range"], "values": r["values"]} for r in params["updates"]],
                },
            )

        raise PermanentError(f"unsupported operation: {operation}")

    def map_to_common_schema(self, raw_data: Any, entity_type: str) -> dict[str, Any]:
        if not isinstance(raw_data, dict):
            raise PermanentError("raw_data must be a dict with 'header' and 'row' keys")
        header: list[str] = raw_data.get("header") or []
        row: list[str] = raw_data.get("row") or []
        by_col = dict(zip(header, row + [""] * (len(header) - len(row))))

        if entity_type == "customer":
            known = {f: by_col.pop(f, "") or None for f in _CUSTOMER_FIELDS}
            return {
                "id": known["id"] or "",
                "source_connector": self.name,
                "external_id": known["id"] or "",
                "name": known["name"],
                "phone": known["phone"],
                "email": known["email"],
                "address": known["address"],
                "metadata": by_col,
            }

        if entity_type == "order":
            items = []
            items_raw = by_col.pop("items_json", "")
            if items_raw:
                try:
                    parsed = json.loads(items_raw)
                except ValueError:
                    parsed = None
                if isinstance(parsed, list):
                    items = parsed
            known = {f: by_col.pop(f, "") for f in _ORDER_FIELDS}
            return {
                "id": known["id"] or "",
                "source_connector": self.name,
                "external_id": known["id"] or "",
                "customer_id": known["customer_id"] or None,
                "status": known["status"] or "pending",
                "items": items,
                "total_amount": _safe_float(known["total_amount"]),
                "shipping_fee": _safe_float(known["shipping_fee"]),
                "created_at": known["created_at"] or None,
                "metadata": by_col,
            }

        raise PermanentError(f"GoogleSheetConnector cannot map entity_type={entity_type!r}")

    def map_from_common_schema(self, common_data: Any, entity_type: str) -> list[str]:
        common_data = _as_dict(common_data)

        if entity_type == "customer":
            fields = _CUSTOMER_FIELDS
        elif entity_type == "order":
            fields = [*_ORDER_FIELDS, "items_json"]
        else:
            raise PermanentError(f"GoogleSheetConnector cannot map entity_type={entity_type!r}")

        row = []
        for field in fields:
            if field == "items_json":
                row.append(json.dumps(common_data.get("items") or []))
            else:
                value = common_data.get(field, "")
                row.append("" if value is None else str(value))
        return row
