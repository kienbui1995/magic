-- RLS for AI-module tables (dlq, prompts, memory_turns).
--
-- These tables were added in migrations 003 and 004 but missed by 005_rls.
-- Policies differ by table because their data models differ:
--
--   dlq           — admin-only view of all permanently-failed tasks. No org_id
--                   field in DLQEntry. Deny access when org context is set;
--                   only admin (no org context) may list DLQ entries. Full
--                   per-org isolation planned for v1.1 (requires org_id field).
--
--   prompts       — global/shared prompt template registry. Intentionally not
--                   org-scoped; all authenticated callers can read/write.
--                   RLS is enabled (so the table is subject to future policy
--                   additions) but the policy permits all access.
--
--   memory_turns  — session-scoped conversation history. session_id is expected
--                   to be prefixed with the caller's org_id (enforced by
--                   handleGetTurns/handleAddTurn at the HTTP layer). RLS
--                   policy mirrors the HTTP-layer check: allow when org context
--                   is empty, or when session_id begins with the org prefix.

ALTER TABLE dlq           ENABLE ROW LEVEL SECURITY;
ALTER TABLE prompts       ENABLE ROW LEVEL SECURITY;
ALTER TABLE memory_turns  ENABLE ROW LEVEL SECURITY;

-- dlq: admin-only. Deny all row access when an org context is active.
-- Workers/org-scoped tokens should not be able to read cross-org DLQ entries.
CREATE POLICY dlq_admin_only ON dlq
    USING (magic_current_org() = '');

-- prompts: intentionally global (shared template registry, no org scope).
CREATE POLICY prompts_global ON prompts
    USING (true)
    WITH CHECK (true);

-- memory_turns: session_id prefix isolation.
-- Allows access when: no org context (admin) OR session_id starts with org prefix.
CREATE POLICY memory_turns_isolation ON memory_turns
    USING (
        magic_current_org() = ''
        OR data->>'session_id' LIKE magic_current_org() || '%'
    )
    WITH CHECK (
        magic_current_org() = ''
        OR data->>'session_id' LIKE magic_current_org() || '%'
    );
