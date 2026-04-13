# Policy Engine

Policies are guardrails that evaluate every task submission. Each policy contains rules that either **hard-block** or **soft-warn** on violations.

## Effects

| Effect | Behavior |
|--------|----------|
| `hard` | Reject the task with `403` |
| `soft` | Allow the task but log a warning + audit entry |

## Built-in rules

| Rule | Description | Example value |
|------|-------------|---------------|
| `allowed_capabilities` | Whitelist of allowed capability types | `["summarize", "translate"]` |
| `blocked_capabilities` | Blacklist of blocked capability types | `["code_execution"]` |
| `max_cost_per_task` | Maximum cost allowed per task | `0.50` |
| `max_timeout_ms` | Maximum timeout allowed per task | `60000` |

## API

### Create policy

```bash
curl -X POST http://localhost:8080/api/v1/orgs/org-123/policies \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-guardrails",
    "enabled": true,
    "rules": [
      {"type": "blocked_capabilities", "value": ["code_execution"], "effect": "hard"},
      {"type": "max_cost_per_task", "value": 0.50, "effect": "hard"},
      {"type": "max_timeout_ms", "value": 60000, "effect": "soft"}
    ]
  }'
```

**Response:**
```json
{
  "id": "pol-abc123",
  "org_id": "org-123",
  "name": "production-guardrails",
  "enabled": true,
  "rules": [
    {"type": "blocked_capabilities", "value": ["code_execution"], "effect": "hard"},
    {"type": "max_cost_per_task", "value": 0.50, "effect": "hard"},
    {"type": "max_timeout_ms", "value": 60000, "effect": "soft"}
  ],
  "created_at": "2026-04-13T10:00:00Z"
}
```

### List policies

```bash
curl http://localhost:8080/api/v1/orgs/org-123/policies \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```

### Get policy

```bash
curl http://localhost:8080/api/v1/orgs/org-123/policies/pol-abc123 \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```

### Update policy

```bash
curl -X PUT http://localhost:8080/api/v1/orgs/org-123/policies/pol-abc123 \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-guardrails",
    "enabled": true,
    "rules": [
      {"type": "blocked_capabilities", "value": ["code_execution"], "effect": "hard"},
      {"type": "max_cost_per_task", "value": 1.00, "effect": "hard"}
    ]
  }'
```

### Delete policy

```bash
curl -X DELETE http://localhost:8080/api/v1/orgs/org-123/policies/pol-abc123 \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```

## Policy violation response

When a `hard` rule blocks a task:

```
HTTP 403 Forbidden
```

```json
{
  "error": "policy_violation",
  "message": "blocked by policy 'production-guardrails'",
  "violations": [
    {"rule": "blocked_capabilities", "detail": "capability 'code_execution' is blocked"}
  ]
}
```
