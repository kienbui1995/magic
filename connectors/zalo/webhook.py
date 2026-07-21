"""Minimal HTTP server that receives Zalo OA webhook events, verifies their
signature, and forwards parsed Common Schema records to a callback.

The callback is where integration with MagiC happens — e.g. submit a task
to the `classify_intent` capability, or hand off to a human inbox. This
module only handles the Zalo-specific transport (signature verification,
JSON parsing); it doesn't know anything about MagiC itself.
"""

import json
import logging
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable

from .connector import ZaloConnector

logger = logging.getLogger("magic_connectors.zalo")

OnEvent = Callable[[list[dict[str, Any]]], None]


def serve(connector: ZaloConnector, on_event: OnEvent, host: str = "0.0.0.0", port: int = 9100) -> None:
    """Start a blocking HTTP server for Zalo OA webhook callbacks."""

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            content_length = self.headers.get("Content-Length")
            if not content_length:
                self.send_error(400, "Missing Content-Length")
                return

            try:
                length = int(content_length)
            except ValueError:
                self.send_error(400, "Invalid Content-Length")
                return

            if length > 1 * 1024 * 1024:
                self.send_error(413, "Request too large")
                return

            raw_body = self.rfile.read(length)

            if not connector.verify_webhook_signature(self.headers, raw_body):
                logger.warning("Rejected webhook with invalid signature")
                self.send_error(401, "Invalid signature")
                return

            try:
                payload = json.loads(raw_body)
            except (json.JSONDecodeError, ValueError):
                self.send_error(400, "Invalid JSON")
                return

            try:
                records = connector.parse_webhook_event(payload)
                if records:
                    on_event(records)
            except Exception:
                logger.exception("on_event callback failed")

            # Zalo expects a 200 quickly regardless of downstream processing outcome.
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')

        def log_message(self, format, *args):
            logger.debug("HTTP %s", format % args)

    server = ThreadingHTTPServer((host, port), Handler)
    logger.info("Zalo webhook server listening on %s:%d", host, port)
    try:
        server.serve_forever()  # NOSONAR python:S5332 — plain HTTP intentional; TLS terminates at the reverse proxy (see README)
    except KeyboardInterrupt:
        logger.info("Shutting down Zalo webhook server")
        server.shutdown()
