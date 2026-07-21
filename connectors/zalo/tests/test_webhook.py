"""Tests for the Zalo webhook HTTP server: signature verification + malformed requests."""

import http.client
import json
import threading
import time

import pytest

from zalo.connector import ZaloConnector
from zalo.webhook import serve

CONFIG = {
    "app_id": "app-123",
    "oa_secret_key": "oa-secret",
    "access_token": "token",
}


@pytest.fixture
def running_server():
    connector = ZaloConnector(CONFIG)
    events: list[list[dict]] = []
    server_holder: dict = {}

    def on_event(records):
        events.append(records)

    def run():
        from http.server import ThreadingHTTPServer

        # Reimplement serve()'s server setup but capture the server so we can shut it down.
        orig_serve_forever = ThreadingHTTPServer.serve_forever

        def patched_serve_forever(self, *a, **kw):
            server_holder["server"] = self
            server_holder["ready"].set()
            orig_serve_forever(self, *a, **kw)

        ThreadingHTTPServer.serve_forever = patched_serve_forever
        try:
            serve(connector, on_event, host="127.0.0.1", port=0)
        finally:
            ThreadingHTTPServer.serve_forever = orig_serve_forever

    server_holder["ready"] = threading.Event()
    thread = threading.Thread(target=run, daemon=True)
    thread.start()
    server_holder["ready"].wait(timeout=5)
    server = server_holder["server"]
    port = server.server_address[1]

    yield connector, events, port

    server.shutdown()
    thread.join(timeout=5)


def _sign(app_id: str, body: bytes, timestamp: str, secret: str) -> str:
    import hashlib

    mac_input = f"{app_id}{body.decode()}{timestamp}{secret}"
    return hashlib.sha256(mac_input.encode()).hexdigest()


def test_valid_signed_webhook_calls_on_event(running_server):
    connector, events, port = running_server
    body = json.dumps(
        {
            "event_name": "user_send_text",
            "sender": {"id": "user-1"},
            "recipient": {"id": "oa-1"},
            "message": {"text": "hi", "msg_id": "m1"},
            "timestamp": "1700000000",
        }
    ).encode()
    sig = _sign("app-123", body, "1700000000", "oa-secret")

    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
    conn.request("POST", "/", body=body, headers={"X-ZEvent-Signature": sig, "Content-Length": str(len(body))})
    resp = conn.getresponse()
    resp.read()
    conn.close()

    assert resp.status == 200
    time.sleep(0.05)
    assert len(events) == 1
    assert events[0][0]["text"] == "hi"


def test_invalid_signature_rejected(running_server):
    connector, events, port = running_server
    body = json.dumps({"event_name": "user_send_text", "timestamp": "1"}).encode()

    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
    conn.request("POST", "/", body=body, headers={"X-ZEvent-Signature": "bad", "Content-Length": str(len(body))})
    resp = conn.getresponse()
    resp.read()
    conn.close()

    assert resp.status == 401
    assert events == []


def test_invalid_content_length_returns_400_not_crash(running_server):
    connector, events, port = running_server
    body = b"{}"

    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
    conn.putrequest("POST", "/")
    conn.putheader("Content-Length", "not-a-number")
    conn.endheaders()
    conn.send(body)
    resp = conn.getresponse()
    resp.read()
    conn.close()

    assert resp.status == 400
