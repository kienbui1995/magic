# RBAC (Role-Based Access Control)

MagiC supports three roles per organization.

## Roles

| Role | Read | Write | Delete | Manage roles |
|------|------|-------|--------|--------------|
| `owner` | ✅ | ✅ | ✅ | ✅ |
| `admin` | ✅ | ✅ | ✅ | ❌ |
| `viewer` | ✅ | ❌ | ❌ | ❌ |

- **owner** — full access including role management
- **admin** — create/update/delete workers, tasks, policies, but cannot manage roles
- **viewer** — read-only access to all resources

## Dev mode

When no role bindings exist for an org, all requests are allowed. RBAC activates automatically when you create the first role binding.

## API

### Create role binding

```bash
curl -X POST http://localhost:8080/api/v1/orgs/org-123/roles \
  -H "Authorization: Bearer $MAGIC_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "subject": "user-alice",
    "role": "admin"
  }'
```

**Response:**
```json
{
  "id": "rb-abc123",
  "org_id": "org-123",
  "subject": "user-alice",
  "role": "admin",
  "created_at": "2026-04-13T10:00:00Z"
}
```

### List role bindings

```bash
curl http://localhost:8080/api/v1/orgs/org-123/roles \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```

### Delete role binding

```bash
curl -X DELETE http://localhost:8080/api/v1/orgs/org-123/roles/rb-abc123 \
  -H "Authorization: Bearer $MAGIC_API_KEY"
```
