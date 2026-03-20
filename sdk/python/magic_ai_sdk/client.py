"""MagiC HTTP client for communicating with the server."""

import logging
import httpx

logger = logging.getLogger("magic_ai_sdk")


class MagiCClient:
    """HTTP client for the MagiC server API."""

    def __init__(self, base_url: str, api_key: str = ""):
        self.base_url = base_url.rstrip("/")
        headers = {}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=httpx.Timeout(connect=5, read=30, write=10, pool=5),
            headers=headers,
        )

    # Health
    def health(self) -> dict:
        return self._client.get("/health").raise_for_status().json()

    # Workers
    def register_worker(self, payload: dict) -> dict:
        return self._client.post("/api/v1/workers/register", json=payload).raise_for_status().json()

    def heartbeat(self, worker_id: str, current_load: int = 0) -> dict:
        return self._client.post("/api/v1/workers/heartbeat", json={
            "worker_id": worker_id, "current_load": current_load, "status": "active",
        }).raise_for_status().json()

    def list_workers(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return self._client.get("/api/v1/workers", params={"limit": limit, "offset": offset}).raise_for_status().json()

    def get_worker(self, worker_id: str) -> dict:
        return self._client.get(f"/api/v1/workers/{worker_id}").raise_for_status().json()

    def delete_worker(self, worker_id: str) -> dict:
        return self._client.delete(f"/api/v1/workers/{worker_id}").raise_for_status().json()

    # Tasks
    def submit_task(self, task: dict) -> dict:
        return self._client.post("/api/v1/tasks", json=task).raise_for_status().json()

    def get_task(self, task_id: str) -> dict:
        return self._client.get(f"/api/v1/tasks/{task_id}").raise_for_status().json()

    def list_tasks(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return self._client.get("/api/v1/tasks", params={"limit": limit, "offset": offset}).raise_for_status().json()

    # Workflows
    def submit_workflow(self, workflow: dict) -> dict:
        return self._client.post("/api/v1/workflows", json=workflow).raise_for_status().json()

    def get_workflow(self, workflow_id: str) -> dict:
        return self._client.get(f"/api/v1/workflows/{workflow_id}").raise_for_status().json()

    def list_workflows(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return self._client.get("/api/v1/workflows", params={"limit": limit, "offset": offset}).raise_for_status().json()

    def approve_step(self, workflow_id: str, step_id: str) -> dict:
        return self._client.post(f"/api/v1/workflows/{workflow_id}/approve/{step_id}").raise_for_status().json()

    # Teams
    def create_team(self, name: str, org_id: str = "", daily_budget: float = 0) -> dict:
        return self._client.post("/api/v1/teams", json={
            "name": name, "org_id": org_id, "daily_budget": daily_budget,
        }).raise_for_status().json()

    def list_teams(self) -> list[dict]:
        return self._client.get("/api/v1/teams").raise_for_status().json()

    # Costs & Metrics
    def get_costs(self) -> dict:
        return self._client.get("/api/v1/costs").raise_for_status().json()

    def get_metrics(self) -> dict:
        return self._client.get("/api/v1/metrics").raise_for_status().json()

    # Knowledge
    def add_knowledge(self, title: str, content: str, tags: list[str] | None = None, scope: str = "org", scope_id: str = "") -> dict:
        return self._client.post("/api/v1/knowledge", json={
            "title": title, "content": content, "tags": tags or [], "scope": scope, "scope_id": scope_id,
        }).raise_for_status().json()

    def search_knowledge(self, query: str = "") -> list[dict]:
        params = {"q": query} if query else {}
        return self._client.get("/api/v1/knowledge", params=params).raise_for_status().json()


class AsyncMagiCClient:
    """Async client for the MagiC server API."""

    def __init__(self, base_url: str, api_key: str | None = None):
        self.base_url = base_url.rstrip("/")
        headers = {}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._client = httpx.AsyncClient(base_url=self.base_url, timeout=30, headers=headers)

    async def close(self):
        await self._client.aclose()

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        await self.close()

    # Health
    async def health(self) -> dict:
        return (await self._client.get("/health")).raise_for_status().json()

    # Workers
    async def register_worker(self, payload: dict) -> dict:
        return (await self._client.post("/api/v1/workers/register", json=payload)).raise_for_status().json()

    async def list_workers(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return (await self._client.get("/api/v1/workers", params={"limit": limit, "offset": offset})).raise_for_status().json()

    async def get_worker(self, worker_id: str) -> dict:
        return (await self._client.get(f"/api/v1/workers/{worker_id}")).raise_for_status().json()

    # Tasks
    async def submit_task(self, task: dict) -> dict:
        return (await self._client.post("/api/v1/tasks", json=task)).raise_for_status().json()

    async def get_task(self, task_id: str) -> dict:
        return (await self._client.get(f"/api/v1/tasks/{task_id}")).raise_for_status().json()

    async def list_tasks(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return (await self._client.get("/api/v1/tasks", params={"limit": limit, "offset": offset})).raise_for_status().json()

    # Workflows
    async def submit_workflow(self, workflow: dict) -> dict:
        return (await self._client.post("/api/v1/workflows", json=workflow)).raise_for_status().json()

    async def get_workflow(self, workflow_id: str) -> dict:
        return (await self._client.get(f"/api/v1/workflows/{workflow_id}")).raise_for_status().json()

    async def list_workflows(self, limit: int = 100, offset: int = 0) -> list[dict]:
        return (await self._client.get("/api/v1/workflows", params={"limit": limit, "offset": offset})).raise_for_status().json()

    # Costs & Metrics
    async def get_costs(self) -> dict:
        return (await self._client.get("/api/v1/costs")).raise_for_status().json()

    async def get_metrics(self) -> dict:
        return (await self._client.get("/api/v1/metrics")).raise_for_status().json()
