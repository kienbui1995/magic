# Webhooks

Receive real-time notifications when events happen in MagiC.

## Register a webhook

```bash
curl -X POST http://localhost:8080/api/v1/orgs/org-123/webhooks \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://myapp.com/webhooks/magic",
    "events": ["task.completed", "task.failed", "budget.threshold"],
    "secret": "my-webhook-secret"
  }'
```

## Supported events

| Event | When |
|-------|------|
| `task.completed` | Task finished successfully |
| `task.failed` | Task failed |
| `task.dispatched` | Task sent to a worker |
| `worker.registered` | New worker joined |
| `worker.deregistered` | Worker left |
| `workflow.completed` | All workflow steps done |
| `workflow.failed` | Workflow failed |
| `budget.threshold` | Spending hit 80% of budget |
| `budget.exceeded` | Budget fully consumed |

## Payload format

```json
{
  "type": "task.completed",
  "timestamp": "2026-04-09T10:00:00Z",
  "data": {
    "task_id": "t-abc123",
    "worker_id": "w-xyz",
    "cost": 0.003
  }
}
```

## Verifying signatures

MagiC signs every delivery with HMAC-SHA256:

```python
import hmac, hashlib

def verify_signature(payload: bytes, signature: str, secret: str) -> bool:
    expected = "sha256=" + hmac.new(
        secret.encode(), payload, hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)

# In your webhook handler:
sig = request.headers.get("X-MagiC-Signature")
if not verify_signature(request.body, sig, "my-webhook-secret"):
    return 401
```

## Delivery guarantees

- **At-least-once** — MagiC retries failed deliveries
- **Retry schedule:** 30s → 5min → 30min → 2h → 8h
- **Max attempts:** 5 (then marked `dead`)
- **Timeout:** 10 seconds per attempt
