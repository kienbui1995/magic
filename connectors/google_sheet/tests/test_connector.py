"""Unit tests for GoogleSheetConnector — HTTP mocked via respx, auth mocked (no real RSA key)."""

from unittest.mock import MagicMock, patch

import httpx
import pytest
import respx

from google_sheet.connector import API_BASE, GoogleSheetConnector
from magic_ai_sdk.connectors.errors import PermanentError, RateLimitError, TransientError

CONFIG = {"spreadsheet_id": "sheet-123", "service_account_info": {"type": "service_account"}}


def _fake_credentials(token="tok-1", valid=True):
    creds = MagicMock()
    creds.valid = valid
    creds.token = token

    def _refresh(request):
        creds.valid = True
        creds.token = "refreshed-tok"

    creds.refresh.side_effect = _refresh
    return creds


@pytest.fixture(autouse=True)
def mock_auth():
    with patch("google_sheet.connector.service_account.Credentials.from_service_account_info") as m:
        m.return_value = _fake_credentials()
        yield m


def make_connector(**overrides):
    return GoogleSheetConnector({**CONFIG, **overrides})


async def test_connect_ensures_token():
    conn = make_connector()
    async with conn:
        assert conn._credentials.token == "tok-1"


@respx.mock
async def test_read_range_success():
    respx.get(f"{API_BASE}/sheet-123/values/Customers%21A1%3AE100").mock(
        return_value=httpx.Response(200, json={"values": [["id", "name"], ["C1", "Anh"]]})
    )
    async with make_connector() as conn:
        data = await conn.execute("read_range", {"range": "Customers!A1:E100"})
    assert data["values"][1] == ["C1", "Anh"]


@respx.mock
async def test_append_row_sends_correct_body():
    route = respx.post(f"{API_BASE}/sheet-123/values/Orders%21A1%3AH1:append").mock(
        return_value=httpx.Response(200, json={"updates": {"updatedRows": 1}})
    )
    async with make_connector() as conn:
        await conn.execute("append_row", {"range": "Orders!A1:H1", "values": [["O1", "C1", "pending"]]})

    assert route.calls.last.request.url.params["valueInputOption"] == "USER_ENTERED"
    assert route.calls.last.request.headers["Authorization"] == "Bearer tok-1"


@respx.mock
async def test_batch_update_sends_all_ranges():
    route = respx.post(f"{API_BASE}/sheet-123/values:batchUpdate").mock(
        return_value=httpx.Response(200, json={"totalUpdatedRows": 2})
    )
    async with make_connector() as conn:
        await conn.execute(
            "batch_update",
            {"updates": [{"range": "A1:A1", "values": [["x"]]}, {"range": "B1:B1", "values": [["y"]]}]},
        )
    import json as jsonlib

    body = jsonlib.loads(route.calls.last.request.content)
    assert len(body["data"]) == 2


@respx.mock
async def test_rate_limit_error():
    respx.get(f"{API_BASE}/sheet-123/values/A1").mock(
        return_value=httpx.Response(429, json={"error": {"code": 429, "status": "RESOURCE_EXHAUSTED", "message": "quota"}})
    )
    async with make_connector() as conn:
        with pytest.raises(RateLimitError):
            await conn.execute("read_range", {"range": "A1"})


@respx.mock
async def test_transient_error_on_unavailable():
    respx.get(f"{API_BASE}/sheet-123/values/A1").mock(
        return_value=httpx.Response(503, json={"error": {"code": 503, "status": "UNAVAILABLE", "message": "down"}})
    )
    async with make_connector() as conn:
        with pytest.raises(TransientError):
            await conn.execute("read_range", {"range": "A1"})


@respx.mock
async def test_permanent_error_on_permission_denied():
    respx.get(f"{API_BASE}/sheet-123/values/A1").mock(
        return_value=httpx.Response(403, json={"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "nope"}})
    )
    async with make_connector() as conn:
        with pytest.raises(PermanentError):
            await conn.execute("read_range", {"range": "A1"})


@respx.mock
async def test_network_error_wrapped_as_transient():
    respx.get(f"{API_BASE}/sheet-123/values/A1").mock(side_effect=httpx.ConnectError("refused"))
    async with make_connector() as conn:
        with pytest.raises(TransientError):
            await conn.execute("read_range", {"range": "A1"})


async def test_expired_credentials_are_refreshed(mock_auth):
    mock_auth.return_value = _fake_credentials(valid=False)
    conn = make_connector()
    async with conn:
        pass
    assert conn._credentials.token == "refreshed-tok"


def test_map_to_common_schema_customer():
    conn = make_connector()
    raw = {"header": ["id", "name", "phone", "email", "address", "note"], "row": ["C1", "Anh", "090", "a@x.com", "HN", "VIP"]}
    result = conn.map_to_common_schema(raw, "customer")
    assert result["id"] == "C1"
    assert result["name"] == "Anh"
    assert result["metadata"] == {"note": "VIP"}


def test_map_to_common_schema_order_with_items():
    conn = make_connector()
    raw = {
        "header": ["id", "customer_id", "status", "total_amount", "shipping_fee", "created_at", "items_json"],
        "row": ["O1", "C1", "confirmed", "100000", "15000", "2026-01-01", '[{"name": "Ao", "quantity": 2}]'],
    }
    result = conn.map_to_common_schema(raw, "order")
    assert result["total_amount"] == 100000.0
    assert result["items"] == [{"name": "Ao", "quantity": 2}]


def test_map_from_common_schema_order_roundtrip():
    conn = make_connector()
    row = conn.map_from_common_schema(
        {"id": "O1", "customer_id": "C1", "status": "pending", "total_amount": 50.0, "items": [{"name": "x"}]},
        "order",
    )
    parsed = conn.map_to_common_schema(
        {"header": ["id", "customer_id", "status", "total_amount", "shipping_fee", "created_at", "items_json"], "row": row},
        "order",
    )
    assert parsed["id"] == "O1"
    assert parsed["items"] == [{"name": "x"}]


def test_map_to_common_schema_rejects_unknown_entity():
    conn = make_connector()
    with pytest.raises(PermanentError):
        conn.map_to_common_schema({"header": [], "row": []}, "case")
