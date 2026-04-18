"""Echo worker for MagiC benchmarks.

Implements the minimal MagiC worker contract:
- On boot: registers with the gateway advertising an `echo` capability.
- On dispatch (POST /dispatch): sleeps `--latency-ms`, optionally fails with
  probability `--fail-rate`, otherwise returns `{type: "complete", ...}`.

This worker is intentionally dependency-light: stdlib + httpx + a small
asyncio HTTP server via `aiohttp` if available, else falls back to
`http.server` in a thread.

Example:
    python3 worker.py --port 9100
    python3 worker.py --port 9101 --fail-rate 0.1 --latency-ms 50
"""

from __future__ import annotations

import argparse
import asyncio
import json
import random
import sys
import threading
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Optional

try:
    import httpx
except ImportError:  # pragma: no cover
    print("ERROR: httpx not installed. Run: pip install httpx", file=sys.stderr)
    sys.exit(1)


@dataclass
class WorkerCfg:
    gateway: str
    token: str
    port: int
    name: str
    latency_ms: int
    fail_rate: float
    concurrency: int


CFG: Optional[WorkerCfg] = None
_SEM: Optional[threading.Semaphore] = None


class Handler(BaseHTTPRequestHandler):
    """Tiny sync handler; MagiC's dispatcher is HTTP POST /dispatch."""

    def log_message(self, fmt: str, *args: object) -> None:  # silence access logs
        return

    def do_POST(self) -> None:  # noqa: N802 (stdlib naming)
        assert CFG is not None and _SEM is not None
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            msg = json.loads(raw)
        except json.JSONDecodeError:
            self.send_response(400)
            self.end_headers()
            return

        task_id = msg.get("payload", {}).get("task", {}).get("id") or msg.get(
            "payload", {}
        ).get("id", "unknown")

        with _SEM:
            # Simulate work.
            if CFG.latency_ms > 0:
                import time as _t

                _t.sleep(CFG.latency_ms / 1000.0)

            # Optional fault injection.
            if CFG.fail_rate > 0 and random.random() < CFG.fail_rate:
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(
                    json.dumps(
                        {
                            "type": "fail",
                            "payload": {
                                "task_id": task_id,
                                "error": {"code": "INJECTED", "message": "fault"},
                            },
                        }
                    ).encode()
                )
                return

            resp = {
                "type": "complete",
                "payload": {
                    "task_id": task_id,
                    "output": msg.get("payload", {}).get("task", {}).get("input", {}),
                    "cost": 0.001,
                },
            }
            body = json.dumps(resp).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)


async def register(cfg: WorkerCfg) -> None:
    payload = {
        "name": cfg.name,
        "capabilities": [
            {
                "name": "echo",
                "est_cost_per_call": 0.001,
                "avg_response_ms": max(cfg.latency_ms, 1),
            }
        ],
        "endpoint": {"type": "http", "url": f"http://localhost:{cfg.port}"},
        "limits": {"max_concurrent_tasks": cfg.concurrency},
    }
    async with httpx.AsyncClient(timeout=10.0) as client:
        r = await client.post(
            f"{cfg.gateway}/api/v1/workers/register",
            json=payload,
            headers={"Authorization": f"Bearer {cfg.token}"},
        )
        r.raise_for_status()
        print(f"registered: {r.json().get('id', '?')} on :{cfg.port}")


def serve(cfg: WorkerCfg) -> None:
    srv = ThreadingHTTPServer(("0.0.0.0", cfg.port), Handler)
    print(f"echo worker listening on :{cfg.port}")
    srv.serve_forever()


def parse_args() -> WorkerCfg:
    p = argparse.ArgumentParser(description="MagiC echo worker")
    p.add_argument("--gateway", default="http://localhost:8080")
    p.add_argument("--token", default="dev-token")
    p.add_argument("--port", type=int, default=9100)
    p.add_argument("--name", default="echo-bench")
    p.add_argument("--latency-ms", type=int, default=10)
    p.add_argument("--fail-rate", type=float, default=0.0)
    p.add_argument("--concurrency", type=int, default=100)
    a = p.parse_args()
    return WorkerCfg(
        gateway=a.gateway.rstrip("/"),
        token=a.token,
        port=a.port,
        name=a.name,
        latency_ms=a.latency_ms,
        fail_rate=a.fail_rate,
        concurrency=a.concurrency,
    )


def main() -> None:
    global CFG, _SEM
    CFG = parse_args()
    _SEM = threading.Semaphore(CFG.concurrency)
    try:
        asyncio.run(register(CFG))
    except Exception as exc:  # pylint: disable=broad-except
        print(f"WARN: registration failed: {exc} (continuing anyway)", file=sys.stderr)
    serve(CFG)


if __name__ == "__main__":
    main()
