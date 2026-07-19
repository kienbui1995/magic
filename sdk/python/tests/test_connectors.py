"""Unit tests for the Connector Framework (base, registry, auth, errors, schema)."""

import time
from typing import Any

import pytest

from magic_ai_sdk.connectors import (
    ApiKeyAuth,
    BaseConnector,
    ConnectorRegistry,
    PermanentError,
    RateLimitError,
    TransientError,
    WebhookSignatureAuth,
    with_retry,
)
from magic_ai_sdk.connectors.schema import ChannelType, Conversation, Message


class FakeConnector(BaseConnector):
    name = "fake"

    def __init__(self, config: dict[str, Any]):
        super().__init__(config)
        self.connected = False

    async def connect(self) -> bool:
        self.connected = True
        return True

    async def disconnect(self) -> None:
        self.connected = False

    async def execute(self, operation: str, params: dict[str, Any]) -> Any:
        if operation == "echo":
            return params
        raise PermanentError(f"unsupported operation: {operation}")

    def map_to_common_schema(self, raw_data: Any, entity_type: str) -> dict[str, Any]:
        return {"entity_type": entity_type, **raw_data}

    def map_from_common_schema(self, common_data: dict[str, Any], entity_type: str) -> Any:
        return {"entity_type": entity_type, **common_data}


# ---- BaseConnector ----


async def test_connector_context_manager_connects_and_disconnects():
    conn = FakeConnector({})
    async with conn as c:
        assert c.connected is True
    assert conn.connected is False


async def test_connector_execute_dispatches_to_operation():
    conn = FakeConnector({})
    result = await conn.execute("echo", {"a": 1})
    assert result == {"a": 1}


async def test_connector_execute_raises_on_unknown_operation():
    conn = FakeConnector({})
    with pytest.raises(PermanentError):
        await conn.execute("does_not_exist", {})


async def test_default_health_check_is_true():
    conn = FakeConnector({})
    assert await conn.health_check() is True


def test_webhook_methods_raise_by_default():
    conn = FakeConnector({})
    with pytest.raises(NotImplementedError):
        conn.verify_webhook_signature({}, b"")
    with pytest.raises(NotImplementedError):
        conn.parse_webhook_event({})


# ---- ConnectorRegistry ----


@pytest.fixture(autouse=True)
def _reset_registry():
    ConnectorRegistry.reset()
    yield
    ConnectorRegistry.reset()


def test_registry_get_creates_and_caches_per_tenant():
    ConnectorRegistry.register("fake", FakeConnector)

    a1 = ConnectorRegistry.get("fake", tenant_id="shop-a", config={})
    a2 = ConnectorRegistry.get("fake", tenant_id="shop-a", config={})
    b1 = ConnectorRegistry.get("fake", tenant_id="shop-b", config={})

    assert a1 is a2  # same tenant -> cached instance
    assert a1 is not b1  # different tenant -> isolated instance


def test_registry_get_unknown_connector_raises():
    with pytest.raises(ValueError):
        ConnectorRegistry.get("nope", tenant_id="shop-a", config={})


def test_registry_evict_drops_cached_instance():
    ConnectorRegistry.register("fake", FakeConnector)
    first = ConnectorRegistry.get("fake", tenant_id="shop-a", config={})
    ConnectorRegistry.evict("fake", tenant_id="shop-a")
    second = ConnectorRegistry.get("fake", tenant_id="shop-a", config={})
    assert first is not second


# ---- AuthHandler ----


def test_api_key_auth_sets_header():
    auth = ApiKeyAuth(key="secret123", header="Authorization", prefix="Bearer ")
    kwargs = auth.apply({})
    assert kwargs["headers"]["Authorization"] == "Bearer secret123"


def test_webhook_signature_auth_verifies_hmac():
    auth = WebhookSignatureAuth(secret="topsecret")
    body = b'{"event":"message"}'
    sig = auth.sign(body)
    assert auth.verify(body, sig) is True
    assert auth.verify(body, "deadbeef") is False


# ---- errors / retry ----


async def test_with_retry_succeeds_after_transient_failures():
    calls = {"n": 0}

    @with_retry(max_attempts=3, base_delay=0.001, max_delay=0.001)
    async def flaky():
        calls["n"] += 1
        if calls["n"] < 3:
            raise TransientError("temporary")
        return "ok"

    result = await flaky()
    assert result == "ok"
    assert calls["n"] == 3


async def test_with_retry_gives_up_after_max_attempts():
    @with_retry(max_attempts=2, base_delay=0.001, max_delay=0.001)
    async def always_fails():
        raise TransientError("nope")

    with pytest.raises(TransientError):
        await always_fails()


async def test_with_retry_does_not_retry_permanent_errors():
    calls = {"n": 0}

    @with_retry(max_attempts=5, base_delay=0.001)
    async def fails_permanently():
        calls["n"] += 1
        raise PermanentError("bad request")

    with pytest.raises(PermanentError):
        await fails_permanently()
    assert calls["n"] == 1


async def test_with_retry_honors_rate_limit_retry_after():
    calls = {"n": 0}

    @with_retry(max_attempts=2, base_delay=5.0)  # base_delay huge so test would time out if not honored
    async def rate_limited_then_ok():
        calls["n"] += 1
        if calls["n"] < 2:
            raise RateLimitError("slow down", retry_after=0.001)
        return "ok"

    start = time.monotonic()
    result = await rate_limited_then_ok()
    elapsed = time.monotonic() - start
    assert result == "ok"
    assert elapsed < 1.0  # honored the short retry_after, not the huge base_delay


# ---- Common Data Schema ----


def test_conversation_schema_roundtrip():
    conv = Conversation(
        id="c1",
        source_connector="zalo",
        channel=ChannelType.ZALO,
        external_id="zalo-user-123",
        messages=[
            Message(id="m1", conversation_id="c1", direction="inbound", sender_id="zalo-user-123", text="hi"),
        ],
    )
    dumped = conv.model_dump()
    assert dumped["channel"] == "zalo"
    assert dumped["messages"][0]["text"] == "hi"
