"""Unit tests for ZaloConnector — HTTP calls are mocked via respx, no network."""

import asyncio
import hashlib
import json

import httpx
import pytest
import respx
from magic_ai_sdk.connectors.errors import AuthError, PermanentError, RateLimitError, TransientError

from zalo.connector import REFRESH_TOKEN_URL, SEND_MESSAGE_URL, ZaloConnector

BASE_CONFIG = {
    "app_id": "app-123",
    "app_secret": "app-secret",
    "oa_secret_key": "oa-secret",
    "access_token": "initial-access-token",
    "refresh_token": "initial-refresh-token",
}


def make_connector(**overrides):
    return ZaloConnector({**BASE_CONFIG, **overrides})


async def test_connect_requires_access_token():
    conn = ZaloConnector({**BASE_CONFIG, "access_token": ""})
    with pytest.raises(AuthError):
        await conn.connect()


@respx.mock
async def test_send_text_message_success():
    respx.post(SEND_MESSAGE_URL).mock(
        return_value=httpx.Response(200, json={"error": 0, "message": "Success", "data": {"message_id": "m1"}})
    )
    conn = make_connector()
    async with conn:
        result = await conn.execute("send_text_message", {"user_id": "u1", "text": "xin chào"})
    assert result["data"]["message_id"] == "m1"

    sent = json.loads(respx.calls.last.request.content)
    assert sent == {"recipient": {"user_id": "u1"}, "message": {"text": "xin chào"}}
    assert respx.calls.last.request.headers["access_token"] == "initial-access-token"


@respx.mock
async def test_send_message_rate_limited_raises():
    respx.post(SEND_MESSAGE_URL).mock(
        return_value=httpx.Response(200, json={"error": -32, "message": "rate limited"})
    )
    conn = make_connector()
    async with conn:
        with pytest.raises(RateLimitError):
            await conn.execute("send_text_message", {"user_id": "u1", "text": "hi"})


@respx.mock
async def test_send_message_permanent_error_raises():
    respx.post(SEND_MESSAGE_URL).mock(
        return_value=httpx.Response(200, json={"error": -201, "message": "invalid user id"})
    )
    conn = make_connector()
    async with conn:
        with pytest.raises(PermanentError):
            await conn.execute("send_text_message", {"user_id": "bad", "text": "hi"})


async def test_execute_unsupported_operation():
    conn = make_connector()
    with pytest.raises(PermanentError):
        await conn.execute("delete_everything", {})


@respx.mock
async def test_access_token_refreshed_when_expired():
    respx.post(REFRESH_TOKEN_URL).mock(
        return_value=httpx.Response(200, json={"access_token": "new-token", "refresh_token": "new-refresh", "expires_in": 3600})
    )
    respx.post(SEND_MESSAGE_URL).mock(return_value=httpx.Response(200, json={"error": 0}))

    conn = make_connector()
    conn._expires_at = 0.0  # force refresh on next call, monotonic() is always > 0
    async with conn:
        await conn.execute("send_text_message", {"user_id": "u1", "text": "hi"})

    assert conn._access_token == "new-token"
    assert respx.calls.last.request.headers["access_token"] == "new-token"


def _sign(app_id: str, raw_body: bytes, timestamp: str, secret: str) -> str:
    mac_input = f"{app_id}{raw_body.decode()}{timestamp}{secret}"
    return hashlib.sha256(mac_input.encode()).hexdigest()


def test_verify_webhook_signature_valid():
    conn = make_connector()
    body = json.dumps({"event_name": "user_send_text", "timestamp": "1234567890"}).encode()
    sig = _sign("app-123", body, "1234567890", "oa-secret")
    assert conn.verify_webhook_signature({"X-ZEvent-Signature": sig}, body) is True


def test_verify_webhook_signature_invalid():
    conn = make_connector()
    body = json.dumps({"event_name": "user_send_text", "timestamp": "1234567890"}).encode()
    assert conn.verify_webhook_signature({"X-ZEvent-Signature": "wrong"}, body) is False


def test_verify_webhook_signature_missing_secret_raises():
    conn = make_connector(oa_secret_key="")
    with pytest.raises(AuthError):
        conn.verify_webhook_signature({"X-ZEvent-Signature": "x"}, b"{}")


def test_parse_webhook_event_user_send_text():
    conn = make_connector()
    payload = {
        "event_name": "user_send_text",
        "sender": {"id": "user-1"},
        "recipient": {"id": "oa-1"},
        "message": {"text": "hello", "msg_id": "msg-1"},
        "timestamp": "1700000000",
    }
    records = conn.parse_webhook_event(payload)
    assert len(records) == 1
    assert records[0]["text"] == "hello"
    assert records[0]["sender_id"] == "user-1"


def test_parse_webhook_event_ignores_other_events():
    conn = make_connector()
    payload = {"event_name": "follow"}
    assert conn.parse_webhook_event(payload) == []


def test_map_from_common_schema():
    conn = make_connector()
    outbound = conn.map_from_common_schema({"sender_id": "user-1", "text": "reply"}, "message")
    assert outbound == {"user_id": "user-1", "text": "reply"}


def test_map_to_common_schema_rejects_unknown_entity():
    conn = make_connector()
    with pytest.raises(PermanentError):
        conn.map_to_common_schema({}, "order")


@respx.mock
async def test_expired_token_error_triggers_transient_retry_with_refresh():
    """Zalo error -203 (invalid/expired token) must force a refresh and retry,
    not fail permanently — see connector.py's _TOKEN_EXPIRED_ERROR_CODES."""
    respx.post(REFRESH_TOKEN_URL).mock(
        return_value=httpx.Response(200, json={"access_token": "refreshed-token", "expires_in": 3600})
    )
    route = respx.post(SEND_MESSAGE_URL)
    route.side_effect = [
        httpx.Response(200, json={"error": -203, "message": "token expired"}),
        httpx.Response(200, json={"error": 0}),
    ]

    conn = make_connector()
    async with conn:
        result = await conn.execute("send_text_message", {"user_id": "u1", "text": "hi"})

    assert result == {"error": 0}
    assert conn._access_token == "refreshed-token"
    assert route.call_count == 2
    assert route.calls[0].request.headers["access_token"] == "initial-access-token"
    assert route.calls[1].request.headers["access_token"] == "refreshed-token"


async def test_ensure_token_refresh_is_serialized_under_concurrency():
    """Concurrent callers must not each trigger their own refresh — Zalo refresh
    tokens are single-use, so a second concurrent refresh would fail."""
    conn = make_connector()
    conn._expires_at = 0.0
    refresh_calls = 0

    async def fake_refresh():
        nonlocal refresh_calls
        refresh_calls += 1
        await asyncio.sleep(0.01)
        conn._access_token = "refreshed-once"
        conn._expires_at = 999999999.0

    conn._refresh_access_token = fake_refresh

    results = await asyncio.gather(*(conn._ensure_token() for _ in range(5)))

    assert refresh_calls == 1
    assert all(token == "refreshed-once" for token in results)


@respx.mock
async def test_token_refresh_invokes_persistence_callback():
    """Zalo refresh tokens are single-use — the new pair must be handed to the
    caller so it can be persisted, or a restart reloads the stale refresh_token."""
    respx.post(REFRESH_TOKEN_URL).mock(
        return_value=httpx.Response(200, json={"access_token": "new-tok", "refresh_token": "new-refresh", "expires_in": 3600})
    )
    saved = {}
    conn = ZaloConnector({**BASE_CONFIG}, on_token_refreshed=saved.update)

    await conn._refresh_access_token()

    assert saved == {"access_token": "new-tok", "refresh_token": "new-refresh"}


@respx.mock
async def test_network_error_wrapped_as_transient():
    respx.post(SEND_MESSAGE_URL).mock(side_effect=httpx.ConnectError("connection refused"))
    conn = make_connector()
    async with conn:
        with pytest.raises(TransientError):
            await conn.execute("send_text_message", {"user_id": "u1", "text": "hi"})


def test_verify_webhook_signature_case_insensitive_header():
    conn = make_connector()
    body = json.dumps({"event_name": "user_send_text", "timestamp": "1234567890"}).encode()
    sig = _sign("app-123", body, "1234567890", "oa-secret")
    # lowercase header name, as some HTTP frameworks normalize to
    assert conn.verify_webhook_signature({"x-zevent-signature": sig}, body) is True


def test_verify_webhook_signature_rejects_non_dict_json():
    conn = make_connector()
    body = json.dumps(["not", "an", "object"]).encode()
    assert conn.verify_webhook_signature({"X-ZEvent-Signature": "whatever"}, body) is False
