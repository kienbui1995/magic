from magic_ai import Worker

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
