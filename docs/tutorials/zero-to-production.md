# Zero to Production in 30 Minutes

Build a complete MagiC fleet from scratch: local dev → real AI agent → multi-tenant auth → production deployment.

**Estimated time: 30 minutes** (6 phases, 5 min each)

---

## Phase 0: Prerequisites (2 minutes)

You need:
- **Docker** + Docker Compose (or Podman)
- **Python 3.11+**
- **Go 1.25+** (optional if using Docker for the server)
- **curl** (for testing) or a REST client
- **PostgreSQL CLI** (optional, for advanced steps)
- **Helm 3.11+** (optional, for Kubernetes)

Get the code:
```bash
git clone https://github.com/kienbui1995/magic.git
cd magic
```

---

## Phase 1: Local Dev with In-Memory Storage (5 minutes)

Start with the simplest setup — no database, just MagiC in memory.

### Build and run the server

```bash
cd core
go build -o ../bin/magic ./cmd/magic
cd ..
./bin/magic serve
```

You should see:
```
[INFO] MagiC server starting on 0.0.0.0:8080
[INFO] Store: memory (dev mode)
[INFO] Ready
```

### Verify health

In another terminal:
```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "version": "0.8.0",
  "protocol_version": "1.0",
  "uptime_seconds": 5
}
```

### Check metrics

```bash
curl http://localhost:8080/metrics | head -20
```

You'll see Prometheus format: counters, gauges, histograms for tasks, workers, cost tracking.

**Checkpoint: MagiC is running and responding. Proceed to Phase 2.**

---

## Phase 2: Persistence with PostgreSQL (5 minutes)

In-memory mode loses data when you restart. Use Postgres for real persistence.

### Start PostgreSQL (easiest: Docker)

```bash
docker run -d \
  -e POSTGRES_PASSWORD=magic-dev \
  -e POSTGRES_DB=magic \
  -p 5432:5432 \
  postgres:15-pgvector
```

Wait for it to be ready:
```bash
sleep 5 && pg_isready -h localhost
```

### Stop the running MagiC server and restart with Postgres

```bash
export MAGIC_POSTGRES_URL="postgres://postgres:magic-dev@localhost:5432/magic?sslmode=disable"
./bin/magic serve
```

MagiC will:
1. Create the `public` schema
2. Run migrations automatically (from `core/internal/store/migrations/`)
3. Print migration progress to stdout

You should see:
```
[INFO] Applying migration: 001_init
[INFO] Applying migration: 002_add_vectors
[INFO] Applying migration: 003_add_dlq
...
[INFO] Ready
```

### Verify persistence

Submit a simple task:
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "test",
    "input": {"msg": "persisted"}
  }'
```

Copy the `task_id` from the response. Now kill MagiC (`Ctrl+C`) and restart it:

```bash
./bin/magic serve
```

Retrieve the task:
```bash
curl http://localhost:8080/api/v1/tasks/{task_id}
```

**It's still there.** The database survived the restart.

**Checkpoint: Your data persists. Now add a real worker.**

---

## Phase 3: Add a Real AI Agent (5 minutes)

Let's wrap CrewAI (or any agent framework) as a MagiC worker.

### Option A: CrewAI (recommended for learning)

```bash
cd examples/integrations/crewai
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
```

Edit `.env` to set `OPENAI_API_KEY`, or switch to Ollama:
```env
OPENAI_API_KEY=sk-...
# OR for local-only:
OPENAI_API_KEY=ollama
OPENAI_API_BASE=http://localhost:11434/v1
```

Run the worker:
```bash
python main.py
```

You should see:
```
[INFO] Registering with MagiC...
[INFO] CrewAI Worker registered as worker_abc123
[INFO] Serving on 0.0.0.0:9101
```

### Option B: Simple Python worker (if you don't have OpenAI key)

In another terminal, create `test-worker.py`:

```python
from magic_ai_sdk import Worker

worker = Worker(name="SimpleBotter", endpoint="http://localhost:9002")

@worker.capability("analyze", description="Analyzes text")
def analyze(text: str) -> dict:
    return {
        "word_count": len(text.split()),
        "length": len(text),
        "summary": f"Text: {text[:50]}..."
    }

if __name__ == "__main__":
    worker.register("http://localhost:8080")
    worker.serve()
```

```bash
python test-worker.py
```

### Submit a task to your worker

From your main terminal:

**For CrewAI:**
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "research_and_write",
    "input": {"topic": "MagiC Framework for AI Orchestration"},
    "routing": {"required_capabilities": ["research_and_write"]},
    "contract": {"timeout_ms": 120000, "max_cost": 1.0}
  }'
```

**For SimpleBotter:**
```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "type": "analyze",
    "input": {"text": "Hello world from MagiC"}
  }'
```

**Checkpoint: Your AI agent is running and producing real outputs. Check cost tracking next.**

---

## Phase 4: Multi-Tenant Auth and Cost Tracking (5 minutes)

Add API keys and track spending.

### Generate an admin API key

```bash
export MAGIC_API_KEY=$(openssl rand -hex 32)
echo "API_KEY: $MAGIC_API_KEY"
```

Stop and restart MagiC with the key:
```bash
MAGIC_API_KEY="$MAGIC_API_KEY" ./bin/magic serve
```

### Create an organization token

```bash
curl -X POST http://localhost:8080/api/v1/orgs/acme/tokens \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "dev-token"}'
```

Response:
```json
{
  "token": "mct_abc...",
  "org_id": "acme",
  "created_at": "2026-04-18T10:30:00Z"
}
```

### Use the token for worker registration

```bash
curl -X POST http://localhost:8080/api/v1/workers/register \
  -H "Authorization: Bearer mct_abc..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "TestWorker",
    "capabilities": [{"name": "test", "description": "Test", "est_cost_per_call": 0.01}],
    "endpoint": {"type": "http", "url": "http://localhost:9002"}
  }'
```

### Check costs

```bash
curl http://localhost:8080/api/v1/costs \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```

Response:
```json
{
  "org_id": "acme",
  "total_cost_usd": 0.05,
  "total_cost_today_usd": 0.03,
  "budget_limit_usd": 100.0,
  "warning_threshold_usd": 80.0
}
```

### Set a budget limit (optional)

```bash
curl -X POST http://localhost:8080/api/v1/orgs/acme/budget \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"limit_usd": 10.0, "pause_on_exceed": true}'
```

Once spent reaches `$10.00`, new tasks are rejected with `429 Budget Exceeded`.

**Checkpoint: Your fleet is now multi-tenant with cost controls. Deploy it next.**

---

## Phase 5: Deploy to Production (5 minutes)

### Option A: Kubernetes with Helm (recommended)

#### Prerequisites
- Kubernetes 1.24+
- Helm 3.11+
- PostgreSQL instance (RDS, Neon, Supabase, or your own)

#### Install

```bash
# 1. Get dependencies
helm dependency update deploy/helm/magic/

# 2. Prepare your values
export MAGIC_API_KEY=$(openssl rand -hex 32)
export POSTGRES_URL="postgres://user:pass@db.example.com:5432/magic?sslmode=require"

# 3. Install
helm install magic deploy/helm/magic/ \
  --namespace magic \
  --create-namespace \
  --set secrets.apiKey="$MAGIC_API_KEY" \
  --set secrets.postgresUrl="$POSTGRES_URL" \
  --set image.tag=v0.8.0

# 4. Verify rollout
kubectl -n magic rollout status deploy/magic
```

Watch logs:
```bash
kubectl -n magic logs -l app.kubernetes.io/name=magic -f
```

Get the endpoint:
```bash
kubectl -n magic port-forward svc/magic 8080:80 &
curl http://localhost:8080/health
```

### Option B: Docker Compose (for small self-hosted)

Create `docker-compose.yml` in your project directory:

```yaml
version: '3.9'

services:
  postgres:
    image: postgres:15-pgvector
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD:-magic-prod}
      POSTGRES_DB: magic
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  magic:
    image: kienbui1995/magic:v0.8.0
    ports:
      - "8080:8080"
    environment:
      MAGIC_POSTGRES_URL: "postgres://postgres:${DB_PASSWORD:-magic-prod}@postgres:5432/magic?sslmode=disable"
      MAGIC_API_KEY: ${MAGIC_API_KEY}
      MAGIC_CORS_ORIGIN: "https://yourdomain.com"
    depends_on:
      - postgres
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

volumes:
  postgres_data:
```

Deploy:
```bash
export MAGIC_API_KEY=$(openssl rand -hex 32)
docker compose up -d

# Watch startup
docker compose logs -f magic
```

Verify:
```bash
curl http://localhost:8080/health
```

### Production checklist

- [ ] **Secrets**: `MAGIC_API_KEY` ≥ 32 chars, stored in Secret (not in code)
- [ ] **Database**: PostgreSQL 13+ with pgvector extension
- [ ] **TLS**: Ingress or reverse proxy (cert-manager, Let's Encrypt)
- [ ] **CORS**: Set `MAGIC_CORS_ORIGIN` to your frontend domain
- [ ] **Rate limiting**: Optional Redis backend for distributed multi-instance deploys
- [ ] **Monitoring**: Enable Prometheus scraping on `/metrics`
- [ ] **Backups**: Automated nightly Postgres backups
- [ ] **Secrets provider**: Move secrets from env to Vault/Sealed Secrets/External Secrets
- [ ] **Resource limits**: Set CPU/memory requests and limits
- [ ] **Pod disruption budget**: For zero-downtime updates

**Checkpoint: MagiC is production-ready. Now enable observability.**

---

## Phase 6: Observability (3 minutes)

### Prometheus metrics (already exposed)

MagiC exports metrics at `GET /metrics` (unauthenticated).

```bash
curl http://localhost:8080/metrics
```

Key metrics:
- `magic_tasks_completed_total` — tasks finished successfully
- `magic_tasks_failed_total` — tasks failed (requeue or DLQ)
- `magic_workers_online` — active worker count
- `magic_cost_total_usd` — organization spending
- `magic_http_requests_duration_seconds` — request latency histogram

### Grafana dashboard (optional)

1. Point Prometheus at `http://magic:8080/metrics` (or your prod endpoint)
2. Import the dashboard from `deploy/grafana/dashboards/magic-overview.json`
3. Pin it to your ops dashboard

### Slack alerts (optional)

Register a webhook to send budget alerts to Slack:

```bash
curl -X POST http://localhost:8080/api/v1/orgs/acme/webhooks \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "events": ["budget.threshold"],
    "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
  }'
```

When an organization hits 80% of budget, MagiC POSTs to that URL with:
```json
{
  "type": "budget.threshold",
  "org_id": "acme",
  "total_cost_usd": 80.0,
  "budget_limit_usd": 100.0,
  "timestamp": "2026-04-18T15:30:00Z"
}
```

### Distributed tracing (optional)

If you have Jaeger or Tempo running:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger-collector:4318
./bin/magic serve
```

MagiC propagates W3C Trace Context headers (`traceparent`) across all task dispatches. You'll see the full call graph from API → Router → Dispatcher → Worker.

---

## What You Just Built

```
Your App (curl / SDK)
    │
    ├─→ MagiC Gateway (auth, cost, policy)
    │
    ├─→ Worker Registry (track capabilities)
    │
    ├─→ Router (find best worker)
    │
    ├─→ Dispatcher (HTTP to workers)
    │       ├─→ CrewAI Agent
    │       └─→ LangChain Agent
    │
    ├─→ Cost Controller (budget tracking)
    │
    ├─→ Audit Log (who ran what)
    │
    └─→ PostgreSQL (persistence)

Multi-tenant? Yes. Auth? Yes. Observability? Yes.
```

---

## Next Steps

**Scale to 10 workers:**
- Spin up more worker instances (Python, Go, Node — any language with HTTP)
- MagiC's router automatically load-balances across them
- Cost tracking aggregates per worker and per organization

**Add RBAC and policies:**
- See `docs-site/guide/rbac.md` for role definitions
- See `docs-site/guide/policies.md` for guardrails (blocked topics, max cost per task, etc.)

**Wrap your existing agent:**
- Follow `examples/integrations/crewai/` or `examples/integrations/langchain/`
- No changes to your agent code needed

**Publish a worker plugin:**
- Follow the worker token standard in `docs-site/api/reference.md`
- Share it on GitHub with topic `magic-worker`

---

## Troubleshooting

**"Connection refused on localhost:8080"**
- MagiC didn't start. Check `./bin/magic serve` output for errors.
- Port 8080 in use? Set `export MAGIC_PORT=8000` and try again.

**"worker registered but task fails with 'no matching worker'"**
- Check `/api/v1/workers` — is your worker listed?
- Check the task's `required_capabilities` matches what the worker declares.
- Check worker logs for registration errors.

**"database 'magic' does not exist"**
- PostgreSQL isn't running, or connection URL is wrong.
- Verify: `psql "$MAGIC_POSTGRES_URL" -c "SELECT 1"`

**"MAGIC_API_KEY: minimum 32 characters"**
- Generate a new one: `openssl rand -hex 32`

**"migrations failed: pgvector extension not installed"**
- Use a Postgres image with pgvector: `postgres:15-pgvector`

---

## Links

- **GitHub**: https://github.com/kienbui1995/magic
- **Docs**: https://magic-ai.dev
- **API Reference**: `docs-site/api/reference.md`
- **Concepts**: `docs-site/guide/concepts.md`
- **Deployment**: `docs-site/guide/deployment.md`
- **Examples**: `examples/` directory
- **Community**: GitHub Discussions
