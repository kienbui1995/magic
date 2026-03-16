"""
Multi-worker example: 2 workers + workflow submission.

Usage:
    Terminal 1: cd core && go run ./cmd/magic serve
    Terminal 2: python examples/multi-worker/main.py
    Terminal 3: curl commands below
"""
import json
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
import httpx

MAGIC_URL = "http://localhost:8080"

# --- Worker 1: ContentBot (port 9001) ---

def content_handler(input_data: dict) -> dict:
    topic = input_data.get("topic", "general")
    return {
        "title": f"Blog Post: {topic}",
        "body": f"This is a well-researched article about {topic}.",
        "word_count": 150,
    }

# --- Worker 2: SEOBot (port 9002) ---

def seo_handler(input_data: dict) -> dict:
    title = input_data.get("title", "")
    return {
        "optimized_title": f"{title} | Best Guide 2026",
        "meta_description": f"Learn everything about {title} in this comprehensive guide.",
        "seo_score": 85,
    }

HANDLERS = {
    9001: ("ContentBot", "content_writing", content_handler),
    9002: ("SEOBot", "seo_optimization", seo_handler),
}

def make_worker_server(port: int):
    name, capability, handler = HANDLERS[port]

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length", 0))
            body = json.loads(self.rfile.read(length))
            payload = body.get("payload", {})

            if body.get("type") == "task.assign":
                try:
                    result = handler(payload.get("input", {}))
                    response = {"type": "task.complete", "payload": {"task_id": payload.get("task_id"), "output": result, "cost": 0.05}}
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

    return HTTPServer(("0.0.0.0", port), Handler), name, capability

def register_worker(name: str, capability: str, port: int):
    client = httpx.Client(base_url=MAGIC_URL, timeout=10)
    result = client.post("/api/v1/workers/register", json={
        "name": name,
        "capabilities": [{"name": capability, "description": f"{name} capability", "est_cost_per_call": 0.05}],
        "endpoint": {"type": "http", "url": f"http://localhost:{port}"},
        "limits": {"max_concurrent_tasks": 5, "max_cost_per_day": 10.0},
    }).json()
    print(f"  Registered {name} as {result['id']}")
    return result["id"]

def main():
    print("=== MagiC Multi-Worker Example ===\n")

    # Start workers
    for port in [9001, 9002]:
        server, name, _ = make_worker_server(port)
        t = threading.Thread(target=server.serve_forever, daemon=True)
        t.start()
        print(f"  {name} serving on :{port}")

    time.sleep(0.5)

    # Register workers
    print()
    register_worker("ContentBot", "content_writing", 9001)
    register_worker("SEOBot", "seo_optimization", 9002)

    client = httpx.Client(base_url=MAGIC_URL, timeout=30)

    # Submit a single task
    print("\n--- Single Task ---")
    task = client.post("/api/v1/tasks", json={
        "type": "content_writing",
        "input": {"topic": "AI Agent Management"},
        "routing": {"strategy": "best_match", "required_capabilities": ["content_writing"]},
        "contract": {"timeout_ms": 30000, "max_cost": 1.0},
    }).json()
    print(f"  Task {task['id']} -> {task['assigned_worker']} (status: {task['status']})")

    time.sleep(1)  # wait for async dispatch

    result = client.get(f"/api/v1/tasks/{task['id']}").json()
    print(f"  Result: status={result['status']}, output={json.dumps(result.get('output', {}))}")

    # Submit a workflow
    print("\n--- Workflow (content -> seo) ---")
    wf = client.post("/api/v1/workflows", json={
        "name": "Blog Pipeline",
        "steps": [
            {"id": "write", "task_type": "content_writing", "input": {"topic": "MagiC Framework"}},
            {"id": "optimize", "task_type": "seo_optimization", "depends_on": ["write"], "input": {"title": "MagiC Framework"}},
        ],
    }).json()
    print(f"  Workflow {wf['id']} started ({len(wf['steps'])} steps)")

    for step in wf["steps"]:
        print(f"    Step '{step['id']}': {step['status']}")

    time.sleep(3)  # wait for workflow to complete

    wf = client.get(f"/api/v1/workflows/{wf['id']}").json()
    print(f"  Workflow final status: {wf['status']}")
    for step in wf["steps"]:
        print(f"    Step '{step['id']}': {step['status']}")

    # Show metrics
    print("\n--- Metrics ---")
    metrics = client.get("/api/v1/metrics").json()
    print(f"  Events: {metrics['total_events']}, Tasks routed: {metrics['tasks_routed']}, Done: {metrics['tasks_done']}")

    costs = client.get("/api/v1/costs").json()
    print(f"  Total cost: ${costs['total_cost']:.2f} ({costs['task_count']} tasks)")

    print("\nDone! Press Ctrl+C to exit.")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        pass

if __name__ == "__main__":
    main()
