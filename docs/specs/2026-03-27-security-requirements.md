# Requirements: Zero-Trust Worker Security

Date: 2026-03-27

## Problem

MagiC hiện chỉ có 1 shared API key cho toàn bộ server (env `MAGIC_API_KEY`).
Điều này có nghĩa: mọi worker dùng chung key — không thể phân biệt "worker nào làm gì",
không có audit trail, không có isolation giữa organizations.

Competitors (AutoGen, CrewAI, Agno) không có auth cho agent-to-agent communication.
Đây là competitive moat quan trọng nhất của MagiC.

## Users

- **Org Admin**: cấp và thu hồi worker credentials
- **Worker**: authenticate khi register và gửi heartbeat
- **MagiC Server**: validate mọi request, enforce org isolation

## Requirements

### Must Have

- WHEN worker registers, the system SHALL require a valid `worker_token` in the request body
- WHEN worker_token is invalid or missing, the system SHALL reject with 401
- WHEN worker A submits a heartbeat, the system SHALL verify token belongs to worker A
- WHEN task is routed, the system SHALL only consider workers in the SAME org as the task
- WHEN any action occurs (register/heartbeat/route/complete), the system SHALL append an Event to audit log
- WHEN audit log is queried, events SHALL be filterable by worker_id, org_id, time range
- WHEN org admin calls `POST /orgs/{id}/workers/{wid}/tokens`, the system SHALL issue a new worker_token
- WHEN org admin calls `DELETE /orgs/{id}/workers/{wid}/tokens/{tid}`, the system SHALL revoke token immediately

### Should Have

- Worker tokens have configurable TTL (default: no expiry)
- Token rotation: issue new token without downtime (old token stays valid for 60s grace period)
- Audit log exportable as JSON

### Out of Scope

- Multi-factor auth
- OAuth / SSO
- Role-based access control (RBAC) beyond org isolation
- Token encryption at rest (SQLite plaintext is acceptable for MVP)
- Rate limiting per worker

## Success Criteria

- [ ] Worker cannot register without a valid token
- [ ] Worker A cannot heartbeat on behalf of Worker B (cross-worker impersonation blocked)
- [ ] Task from Org A is never routed to Worker from Org B
- [ ] Every register/heartbeat/route/complete action creates an audit Event in store
- [ ] Revoked token is rejected within 1 request (no caching delay)
- [ ] All existing tests still pass after implementation
- [ ] New security tests cover all Must Have scenarios
