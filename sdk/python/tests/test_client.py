from magic_ai_sdk import MagiCClient, AsyncMagiCClient


def test_sync_client_init():
    c = MagiCClient("http://localhost:8080")
    assert c.base_url == "http://localhost:8080"


def test_sync_client_with_api_key():
    c = MagiCClient("http://localhost:8080", api_key="test-key")
    assert c._client.headers["authorization"] == "Bearer test-key"


def test_async_client_init():
    c = AsyncMagiCClient("http://localhost:8080/")
    assert c.base_url == "http://localhost:8080"


def test_exports():
    """Verify all public symbols are exported."""
    from magic_ai_sdk import Worker, MagiCClient, AsyncMagiCClient, capability
    assert Worker is not None
    assert MagiCClient is not None
    assert AsyncMagiCClient is not None
    assert callable(capability)
