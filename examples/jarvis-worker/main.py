"""
Jarvis Worker — wraps my-jarvis personal AI assistant as a MagiC worker.

Usage:
    MAGIC_URL=http://localhost:18080 \
    JARVIS_URL=http://localhost:8000 \
    JARVIS_API_KEY=your-64-char-key \
    python main.py
"""

import logging
import os

import httpx
from magic_ai_sdk import Worker, capability

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("jarvis-worker")

JARVIS_URL = os.environ["JARVIS_URL"].rstrip("/")
JARVIS_API_KEY = os.environ["JARVIS_API_KEY"]
MAGIC_URL = os.environ.get("MAGIC_URL", "http://localhost:18080")
MAGIC_WORKER_TOKEN = os.environ.get("MAGIC_WORKER_TOKEN", "")

_headers = {"X-API-Key": JARVIS_API_KEY}


def _call(path: str, args: dict = None) -> dict:
    """Call my-jarvis public API."""
    url = f"{JARVIS_URL}/api/public/v1{path}"
    with httpx.Client(timeout=30) as client:
        resp = client.post(url, headers=_headers, json=args or {})
        resp.raise_for_status()
        return resp.json()


class JarvisWorker(Worker):
    """MagiC worker backed by my-jarvis personal AI assistant."""

    @capability(
        name="chat",
        description="Vietnamese personal AI assistant. General questions, task planning, memory access. "
                    "Args: message (str), conversation_id (str, optional)",
    )
    def chat(self, message: str = "", conversation_id: str = None, **_) -> dict:
        result = _call("/chat", {"message": message, "conversation_id": conversation_id})
        log.info("chat → model=%s", result.get("model"))
        return {"response": result["response"], "model": result.get("model")}

    @capability(name="task_create",
                description="Create a personal task. Args: title (str), due_date (YYYY-MM-DD), priority (low/medium/high/urgent)")
    def task_create(self, **kwargs) -> dict:
        return _call("/tools/task_create/invoke", {"args": kwargs})

    @capability(name="task_list",
                description="List personal tasks. Args: status (todo/in_progress/done/all)")
    def task_list(self, **kwargs) -> dict:
        return _call("/tools/task_list/invoke", {"args": kwargs})

    @capability(name="task_update",
                description="Update a task. Args: task_id (str), status (str), title (str)")
    def task_update(self, **kwargs) -> dict:
        return _call("/tools/task_update/invoke", {"args": kwargs})

    @capability(name="calendar_create",
                description="Create calendar event. Args: title (str), start_time (ISO), end_time (ISO), location (str)")
    def calendar_create(self, **kwargs) -> dict:
        return _call("/tools/calendar_create/invoke", {"args": kwargs})

    @capability(name="calendar_list",
                description="List upcoming calendar events. Args: days (int)")
    def calendar_list(self, **kwargs) -> dict:
        return _call("/tools/calendar_list/invoke", {"args": kwargs})

    @capability(name="memory_search",
                description="Search personal memory. Args: query (str), limit (int)")
    def memory_search(self, **kwargs) -> dict:
        return _call("/tools/memory_search/invoke", {"args": kwargs})

    @capability(name="memory_save",
                description="Save to personal memory. Args: content (str)")
    def memory_save(self, **kwargs) -> dict:
        return _call("/tools/memory_save/invoke", {"args": kwargs})

    @capability(name="web_search",
                description="Search the web. Args: query (str)")
    def web_search(self, **kwargs) -> dict:
        return _call("/tools/web_search/invoke", {"args": kwargs})

    @capability(name="summarize_url",
                description="Summarize a webpage. Args: url (str)")
    def summarize_url(self, **kwargs) -> dict:
        return _call("/tools/summarize_url/invoke", {"args": kwargs})

    @capability(name="note_save",
                description="Save a note. Args: content (str), title (str)")
    def note_save(self, **kwargs) -> dict:
        return _call("/tools/note_save/invoke", {"args": kwargs})

    @capability(name="note_search",
                description="Search notes. Args: query (str)")
    def note_search(self, **kwargs) -> dict:
        return _call("/tools/note_search/invoke", {"args": kwargs})

    @capability(name="weather_vn",
                description="Vietnam weather. Args: city (str)")
    def weather_vn(self, **kwargs) -> dict:
        return _call("/tools/weather_vn/invoke", {"args": kwargs})

    @capability(name="news_vn",
                description="Vietnam news. Args: topic (str, optional)")
    def news_vn(self, **kwargs) -> dict:
        return _call("/tools/news_vn/invoke", {"args": kwargs})


if __name__ == "__main__":
    PORT = int(os.environ.get("WORKER_PORT", "9001"))
    ENDPOINT = os.environ.get("WORKER_ENDPOINT", f"http://localhost:{PORT}")

    worker = JarvisWorker(
        name="JarvisWorker",
        endpoint=ENDPOINT,
        worker_token=MAGIC_WORKER_TOKEN,
    )
    worker.run(MAGIC_URL, port=PORT)  # auto-discover + register + serve
