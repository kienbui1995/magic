"""MagiC Worker — concurrent task handling with threading."""

import inspect
import json
import logging
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Callable

from magic_ai_sdk.client import MagiCClient

logger = logging.getLogger("magic_ai_sdk")


class Worker:
    """A MagiC worker that registers capabilities and handles tasks concurrently."""

    def __init__(self, name: str, endpoint: str = "http://localhost:9000", worker_token: str = "", max_workers: int = 5):
        self.name = name
        self.endpoint = endpoint
        self._worker_token = worker_token
        self.max_workers = max_workers
        self._capabilities: dict[str, dict] = {}
        self._handlers: dict[str, Callable] = {}
        self._worker_id: str | None = None
        self._client: MagiCClient | None = None
        self._semaphore = threading.Semaphore(max_workers)

    def capability(self, name: str, description: str = "", est_cost: float = 0.0):
        """Decorator to register a function as a worker capability."""
        def decorator(func):
            self._capabilities[name] = {
                "name": name,
                "description": description or func.__doc__ or "",
                "est_cost_per_call": est_cost,
            }
            self._handlers[name] = func
            return func
        return decorator

    def _discover_capabilities(self):
        """Auto-discover methods decorated with @capability and register them.

        Works with both the standalone @capability decorator (sets _magic_capability
        attribute) and the instance .capability() decorator (populates _capabilities
        dict directly). Safe to call multiple times — won't duplicate entries.
        """
        for _, method in inspect.getmembers(self, predicate=inspect.ismethod):
            cap = getattr(method, "_magic_capability", None)
            if cap and cap["name"] not in self._capabilities:
                self._capabilities[cap["name"]] = cap
                self._handlers[cap["name"]] = method

    def register(self, magic_url: str):
        """Register this worker with the MagiC server.

        Auto-discovers @capability decorated methods before registering,
        so subclasses don't need to call _discover_capabilities() manually.
        """
        self._discover_capabilities()
        self._client = MagiCClient(magic_url)
        payload = {
            "name": self.name,
            "capabilities": list(self._capabilities.values()),
            "endpoint": {"type": "http", "url": self.endpoint},
            "limits": {"max_concurrent_tasks": self.max_workers},
        }
        result = self._client.register_worker(payload, worker_token=self._worker_token)
        self._worker_id = result.get("id")
        logger.info("Registered as %s with %d capabilities", self._worker_id, len(self._capabilities))
        return self

    def _start_heartbeat(self, interval: int = 30):
        """Start background heartbeat thread with exponential backoff on failure."""
        def loop():
            backoff = 0
            while True:
                time.sleep(interval + backoff)
                if self._client and self._worker_id:
                    try:
                        self._client.heartbeat(self._worker_id, worker_token=self._worker_token)
                        backoff = 0  # reset on success
                    except Exception as e:
                        backoff = min(backoff * 2 + 1, 60)
                        logger.warning("Heartbeat failed (retry in %ds): %s", interval + backoff, e)
        t = threading.Thread(target=loop, daemon=True)
        t.start()

    def handle_task(self, task_type: str, input_data: dict) -> dict:
        """Execute a task handler by capability name."""
        handler = self._handlers.get(task_type)
        if not handler:
            raise ValueError(f"No handler for {task_type}")
        result = handler(**input_data)
        if isinstance(result, str):
            return {"result": result}
        return result

    def run(self, magic_url: str, port: int = 9000, host: str = "0.0.0.0"):
        """Register with MagiC and start serving — one-call convenience method.

        Equivalent to: worker.register(magic_url); worker.serve(port=port)

        Example::

            worker = MyWorker(name="MyBot", endpoint="http://mybot:9000")
            worker.run("http://magic:18080", port=9000)
        """
        self.register(magic_url)
        self.serve(host=host, port=port)

    def serve(self, host: str = "0.0.0.0", port: int = 9000):
        """Start the worker HTTP server with concurrent task handling."""
        worker_ref = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                content_length = self.headers.get("Content-Length")
                if not content_length:
                    self.send_error(400, "Missing Content-Length")
                    return

                length = int(content_length)
                if length > 10 * 1024 * 1024:
                    self.send_error(413, "Request too large")
                    return

                try:
                    body = json.loads(self.rfile.read(length))
                except (json.JSONDecodeError, ValueError):
                    self.send_error(400, "Invalid JSON")
                    return

                msg_type = body.get("type", "")
                payload = body.get("payload", {})

                if msg_type == "task.assign":
                    task_id = payload.get("task_id", "unknown")
                    task_type = payload.get("task_type", "")
                    logger.info("Task %s received (type: %s)", task_id, task_type)

                    acquired = worker_ref._semaphore.acquire(timeout=5)
                    if not acquired:
                        response = {
                            "type": "task.fail",
                            "payload": {"task_id": task_id, "error": {"code": "overloaded", "message": "worker at max capacity"}},
                        }
                        logger.warning("Task %s rejected: at max capacity", task_id)
                    else:
                        try:
                            result = worker_ref.handle_task(task_type, payload.get("input", {}))
                            response = {
                                "type": "task.complete",
                                "payload": {"task_id": task_id, "output": result, "cost": 0.0},
                            }
                            logger.info("Task %s completed", task_id)
                        except Exception as e:
                            response = {
                                "type": "task.fail",
                                "payload": {"task_id": task_id, "error": {"code": "handler_error", "message": str(e)}},
                            }
                            logger.error("Task %s failed: %s", task_id, e)
                        finally:
                            worker_ref._semaphore.release()

                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    self.wfile.write(json.dumps(response).encode())
                else:
                    self.send_error(404, f"Unknown message type: {msg_type}")

            def log_message(self, format, *args):
                logger.debug("HTTP %s", format % args)

        self._start_heartbeat()

        parsed = self.endpoint.split(":")
        if len(parsed) > 2:
            port = int(parsed[-1].split("/")[0])

        server = ThreadingHTTPServer((host, port), Handler)
        logger.info("%s serving on %s:%d (max_workers=%d)", self.name, host, port, self.max_workers)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            logger.info("Shutting down %s", self.name)
            server.shutdown()
