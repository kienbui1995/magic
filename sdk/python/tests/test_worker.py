from magic_ai_sdk import Worker, __version__


def test_worker_capability_registration():
    w = Worker(name="TestBot")

    @w.capability("greeting", description="Says hello")
    def greet(name: str) -> str:
        return f"Hello, {name}!"

    assert "greeting" in w._capabilities
    assert w._capabilities["greeting"]["name"] == "greeting"


def test_worker_handle_task():
    w = Worker(name="TestBot")

    @w.capability("greeting")
    def greet(name: str) -> str:
        return f"Hello, {name}!"

    result = w.handle_task("greeting", {"name": "Kien"})
    assert result == {"result": "Hello, Kien!"}


def test_worker_handle_unknown_task():
    w = Worker(name="TestBot")
    try:
        w.handle_task("nonexistent", {})
        assert False, "should raise"
    except ValueError:
        pass


def test_version():
    assert __version__ == "0.2.0"


def test_worker_max_workers():
    w = Worker(name="TestBot", max_workers=10)
    assert w.max_workers == 10


def test_worker_with_token():
    w = Worker(name="TestBot", worker_token="mct_abc123")
    assert w._worker_token == "mct_abc123"


def test_worker_without_token():
    w = Worker(name="TestBot")
    assert w._worker_token == ""


def test_worker_token_in_register_payload():
    """When token is set, register payload includes worker_token."""
    import unittest.mock as mock

    w = Worker(name="TestBot", worker_token="mct_abc123")

    @w.capability("greet")
    def greet(name: str) -> str:
        return f"Hello, {name}!"

    captured = {}

    def fake_register_worker(payload):
        captured["payload"] = payload
        return {"id": "worker-1"}

    with mock.patch("magic_ai_sdk.worker.MagiCClient") as MockClient:
        instance = MockClient.return_value
        instance.register_worker.side_effect = fake_register_worker
        w.register("http://localhost:8080")

    assert "worker_token" in captured["payload"]
    assert captured["payload"]["worker_token"] == "mct_abc123"


def test_worker_no_token_in_register_payload():
    """When token is empty, worker_token is NOT in payload (backward compat)."""
    import unittest.mock as mock

    w = Worker(name="TestBot")

    @w.capability("greet")
    def greet(name: str) -> str:
        return f"Hello, {name}!"

    captured = {}

    def fake_register_worker(payload):
        captured["payload"] = payload
        return {"id": "worker-1"}

    with mock.patch("magic_ai_sdk.worker.MagiCClient") as MockClient:
        instance = MockClient.return_value
        instance.register_worker.side_effect = fake_register_worker
        w.register("http://localhost:8080")

    assert "worker_token" not in captured["payload"]
