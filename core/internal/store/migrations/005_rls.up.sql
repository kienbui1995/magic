-- Row-Level Security (RLS) for multi-tenant isolation.
--
-- Policies use the session variable `app.current_org_id`. When the variable
-- is empty (or unset), all rows are visible — this is the "bypass mode" used
-- by dev/admin contexts and keeps existing code working until the gateway
-- starts calling SetOrgContext. When the variable is set, every query is
-- transparently filtered to rows belonging to that org.
--
-- The current_setting(name, true) form returns NULL if unset; COALESCE makes
-- it behave as empty string so the bypass check works uniformly.
--
-- NOTE: RLS is NOT enforced for table owners or superusers by default. In
-- production, the application should connect as a non-superuser role and
-- that role should NOT have BYPASSRLS. See docs/security/rls.md.

-- Helper: returns the org scope for the current session, or '' if unset.
-- Using a function keeps policy expressions short and consistent.
CREATE OR REPLACE FUNCTION magic_current_org() RETURNS text
    LANGUAGE sql STABLE AS
$$ SELECT COALESCE(current_setting('app.current_org_id', true), '') $$;

-- ---- Enable RLS ----
ALTER TABLE workers             ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks               ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflows           ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge           ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhooks            ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries  ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies            ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_bindings       ENABLE ROW LEVEL SECURITY;
ALTER TABLE worker_tokens       ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log           ENABLE ROW LEVEL SECURITY;

-- ---- Policies: data->>'org_id' at top level of JSONB blob ----
CREATE POLICY workers_isolation ON workers
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY webhooks_isolation ON webhooks
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY webhook_deliveries_isolation ON webhook_deliveries
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY policies_isolation ON policies
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY role_bindings_isolation ON role_bindings
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY worker_tokens_isolation ON worker_tokens
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

CREATE POLICY audit_log_isolation ON audit_log
    USING (magic_current_org() = '' OR data->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->>'org_id' = magic_current_org());

-- ---- Policies: nested data->'context'->>'org_id' ----
CREATE POLICY tasks_isolation ON tasks
    USING (magic_current_org() = '' OR data->'context'->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->'context'->>'org_id' = magic_current_org());

CREATE POLICY workflows_isolation ON workflows
    USING (magic_current_org() = '' OR data->'context'->>'org_id' = magic_current_org())
    WITH CHECK (magic_current_org() = '' OR data->'context'->>'org_id' = magic_current_org());

-- ---- Knowledge: only enforce isolation when scope = 'org' ----
-- Other scopes (team, worker) are left visible under RLS — upstream authZ is
-- responsible for those. Empty org var still bypasses.
CREATE POLICY knowledge_isolation ON knowledge
    USING (
        magic_current_org() = ''
        OR data->>'scope' <> 'org'
        OR data->>'scope_id' = magic_current_org()
    )
    WITH CHECK (
        magic_current_org() = ''
        OR data->>'scope' <> 'org'
        OR data->>'scope_id' = magic_current_org()
    );

-- ---- Supporting indexes for RLS predicate performance ----
CREATE INDEX IF NOT EXISTS idx_webhooks_org          ON webhooks          ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_org ON webhook_deliveries((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_policies_org          ON policies          ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_role_bindings_org     ON role_bindings     ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_worker_tokens_org     ON worker_tokens     ((data->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_tasks_context_org     ON tasks             ((data->'context'->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_workflows_context_org ON workflows         ((data->'context'->>'org_id'));
CREATE INDEX IF NOT EXISTS idx_knowledge_scope       ON knowledge         ((data->>'scope'), (data->>'scope_id'));
