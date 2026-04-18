-- Reverse RLS: drop policies, disable RLS, drop helper and supporting indexes.
DROP POLICY IF EXISTS workers_isolation            ON workers;
DROP POLICY IF EXISTS tasks_isolation              ON tasks;
DROP POLICY IF EXISTS workflows_isolation          ON workflows;
DROP POLICY IF EXISTS knowledge_isolation          ON knowledge;
DROP POLICY IF EXISTS webhooks_isolation           ON webhooks;
DROP POLICY IF EXISTS webhook_deliveries_isolation ON webhook_deliveries;
DROP POLICY IF EXISTS policies_isolation           ON policies;
DROP POLICY IF EXISTS role_bindings_isolation      ON role_bindings;
DROP POLICY IF EXISTS worker_tokens_isolation      ON worker_tokens;
DROP POLICY IF EXISTS audit_log_isolation          ON audit_log;

ALTER TABLE workers            DISABLE ROW LEVEL SECURITY;
ALTER TABLE tasks              DISABLE ROW LEVEL SECURITY;
ALTER TABLE workflows          DISABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge          DISABLE ROW LEVEL SECURITY;
ALTER TABLE webhooks           DISABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries DISABLE ROW LEVEL SECURITY;
ALTER TABLE policies           DISABLE ROW LEVEL SECURITY;
ALTER TABLE role_bindings      DISABLE ROW LEVEL SECURITY;
ALTER TABLE worker_tokens      DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log          DISABLE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS idx_webhooks_org;
DROP INDEX IF EXISTS idx_webhook_deliveries_org;
DROP INDEX IF EXISTS idx_policies_org;
DROP INDEX IF EXISTS idx_role_bindings_org;
DROP INDEX IF EXISTS idx_worker_tokens_org;
DROP INDEX IF EXISTS idx_tasks_context_org;
DROP INDEX IF EXISTS idx_workflows_context_org;
DROP INDEX IF EXISTS idx_knowledge_scope;

DROP FUNCTION IF EXISTS magic_current_org();
