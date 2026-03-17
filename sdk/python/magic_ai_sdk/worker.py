import json
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Callable

from magic_ai_sdk.client import MagiCClient

class Worker:
    def __init__(self, name: str, endpoint: str = "http://localhost:9000"):
        self.name = name
        self.endpoint = endpoint
        self._capabilities: dict[str, dict] = {}
        self._handlers: dict[str, Callable] = {}
        self._worker_id: str | None = None
        self._client: MagiCClient | None = None

    def capability(self, name: str, description: str = "", est_cost: float = 0.0):
        def decorator(func):
            self._capabilities[name] = {
                "name": name,
                "description": description or func.__doc__ or "",
                "est_cost_per_call": est_cost,
            }
            self._handlers[name] = func
            return func
        return decorator

    def register(self, magic_url: str):
        self._client = MagiCClient(magic_url)
        payload = {
            "name": self.name,
            "capabilities": list(self._capabilities.values()),
            "endpoint": {"type": "http", "url": self.endpoint},
            "limits": {"max_concurrent_tasks": 5},
        }
        result = self._client.register_worker(payload)
        self._worker_id = result.get("id")
        print(f"Registered as {self._worker_id}")
        return self

    def _start_heartbeat(self, interval: int = 30):
        def loop():
            while True:
                time.sleep(interval)
                if self._client and self._worker_id:
                    try:
                        self._client.heartbeat(self._worker_id)
                    except Exception:
                        pass
        t = threading.Thread(target=loop, daemon=True)
        t.start()

    def handle_task(self, task_type: str, input_data: dict) -> dict:
        handler = self._handlers.get(task_type)
        if not handler:
            raise ValueError(f"No handler for {task_type}")
        result = handler(**input_data)
        if isinstance(result, str):
            return {"result": result}
        return result

    def serve(self, host: str = "0.0.0.0", port: int = 9000):
        worker = self
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                length = int(self.headers.get("Content-Length", 0))
                body = json.loads(self.rfile.read(length))
                msg_type = body.get("type", "")
                payload = body.get("payload", {})

                if msg_type == "task.assign":
                    try:
                        result = worker.handle_task(payload.get("task_type", ""), payload.get("input", {}))
                        response = {"type": "task.complete", "payload": {"task_id": payload.get("task_id"), "output": result}}
                    except Exception as e:
                        response = {"type": "task.fail", "payload": {"task_id": payload.get("task_id"), "error": {"message": str(e)}}}
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(json.dumps(response).encode())
                else:
                    self.send_response(404)
                    self.end_headers()

            def log_message(self, format, *args):
                pass

        self._start_heartbeat()
        parsed = self.endpoint.split(":")
        port = int(parsed[-1].split("/")[0]) if len(parsed) > 2 else port
        server = HTTPServer((host, port), Handler)
        print(f"{self.name} serving on {host}:{port}")
        server.serve_forever()
