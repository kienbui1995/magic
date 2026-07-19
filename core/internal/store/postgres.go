package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// PostgreSQLStore is a PostgreSQL-backed implementation of the Store interface.
type PostgreSQLStore struct {
	pool *pgxpool.Pool
}

// orgIDCtxKey is the context key used by WithOrgIDContext / OrgIDFromContext
// and by the pgxpool BeforeAcquire hook that engages RLS.
type orgIDCtxKey struct{}

// WithOrgIDContext stamps the given orgID onto ctx so that any postgres query
// executed with this ctx runs with app.current_org_id set to orgID (engaging
// RLS policies from migration 005). Empty orgID is a no-op — the caller falls
// back to RLS bypass mode.
//
// For non-postgres backends (Memory, SQLite) this value is ignored.
func WithOrgIDContext(ctx context.Context, orgID string) context.Context {
	if orgID == "" {
		return ctx
	}
	return context.WithValue(ctx, orgIDCtxKey{}, orgID)
}

// OrgIDFromContext returns the orgID previously stamped via WithOrgIDContext,
// or "" when absent.
func OrgIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(orgIDCtxKey{}).(string)
	return v
}

// NewPostgreSQLStore creates a new PostgreSQL store using the given connection string.
//
// The pool is configured with BeforeAcquire / AfterRelease hooks that read
// the orgID from the request context (see WithOrgIDContext) and engage RLS
// by setting the app.current_org_id session variable on the acquired
// connection. AfterRelease always resets the variable so pooled connections
// do not leak the scope to the next caller.
func NewPostgreSQLStore(ctx context.Context, connStr string) (*PostgreSQLStore, error) {
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}
	cfg.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		orgID := OrgIDFromContext(ctx)
		if orgID == "" {
			return true // bypass mode — leave var unset
		}
		if _, err := conn.Exec(ctx, "SELECT set_config('app.current_org_id', $1, false)", orgID); err != nil {
			// If we can't set the session var, don't hand out this conn —
			// that would silently drop tenant isolation.
			return false
		}
		return true
	}
	cfg.AfterRelease = func(conn *pgx.Conn) bool {
		// Reset without propagating request ctx (may be cancelled). Use a
		// short background timeout so a broken conn can't stall the pool.
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := conn.Exec(rctx, "SELECT set_config('app.current_org_id', '', false)"); err != nil {
			// Reset failed — drop the conn rather than risk leaking the
			// previous orgID to a future request.
			return false
		}
		return true
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &PostgreSQLStore{pool: pool}, nil
}

// Pool returns the underlying connection pool.
func (s *PostgreSQLStore) Pool() *pgxpool.Pool { return s.pool }

// Close closes the connection pool.
func (s *PostgreSQLStore) Close() { s.pool.Close() }

// WithOrgContext acquires a connection from the pool, sets the session
// variable `app.current_org_id` (consumed by RLS policies in migration 005),
// and invokes fn.
func (s *PostgreSQLStore) WithOrgContext(ctx context.Context, orgID string, fn func(conn *pgxpool.Conn) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("pool.Acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_org_id', $1, false)", orgID); err != nil {
		return fmt.Errorf("set app.current_org_id: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(ctx, "SELECT set_config('app.current_org_id', '', false)")
	}()

	return fn(conn)
}

// — Generic helpers —

func pgPut(ctx context.Context, pool *pgxpool.Pool, table, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		"INSERT INTO "+table+" (id, data) VALUES ($1, $2::jsonb)"+
			" ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data",
		id, data)
	return err
}

func pgGet[T any](ctx context.Context, pool *pgxpool.Pool, table, id string) (*T, error) {
	var data []byte
	err := pool.QueryRow(ctx,
		"SELECT data FROM "+table+" WHERE id = $1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func pgDelete(ctx context.Context, pool *pgxpool.Pool, table, id string) error {
	result, err := pool.Exec(ctx,
		"DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func pgList[T any](ctx context.Context, pool *pgxpool.Pool, query string, args ...any) ([]*T, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*T
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		results = append(results, &v)
	}
	return results, nil
}

// — Workers —

func (s *PostgreSQLStore) AddWorker(ctx context.Context, w *protocol.Worker) error {
	return pgPut(ctx, s.pool, "workers", w.ID, w)
}

func (s *PostgreSQLStore) GetWorker(ctx context.Context, id string) (*protocol.Worker, error) {
	return pgGet[protocol.Worker](ctx, s.pool, "workers", id)
}

func (s *PostgreSQLStore) UpdateWorker(ctx context.Context, w *protocol.Worker) error {
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `UPDATE workers SET data = $2::jsonb WHERE id = $1`, w.ID, data)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("worker %s not found", w.ID)
	}
	return nil
}

func (s *PostgreSQLStore) RemoveWorker(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "workers", id)
}

func (s *PostgreSQLStore) ListWorkers(ctx context.Context) []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](ctx, s.pool, "SELECT data FROM workers ORDER BY id")
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapability(ctx context.Context, capability string) []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](ctx, s.pool,
		`SELECT data FROM workers
         WHERE EXISTS (
             SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
             WHERE cap->>'name' = $1
         )`, capability)
	return workers
}

func (s *PostgreSQLStore) ListWorkersByOrg(ctx context.Context, orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.ListWorkers(ctx)
	}
	workers, _ := pgList[protocol.Worker](ctx, s.pool,
		"SELECT data FROM workers WHERE data->>'org_id' = $1 ORDER BY id", orgID)
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapabilityAndOrg(ctx context.Context, capability, orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.FindWorkersByCapability(ctx, capability)
	}
	workers, _ := pgList[protocol.Worker](ctx, s.pool,
		`SELECT data FROM workers
         WHERE data->>'org_id' = $1
         AND EXISTS (
             SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
             WHERE cap->>'name' = $2
         )`, orgID, capability)
	return workers
}

// — Tasks —

func (s *PostgreSQLStore) AddTask(ctx context.Context, t *protocol.Task) error {
	return pgPut(ctx, s.pool, "tasks", t.ID, t)
}

func (s *PostgreSQLStore) GetTask(ctx context.Context, id string) (*protocol.Task, error) {
	return pgGet[protocol.Task](ctx, s.pool, "tasks", id)
}

func (s *PostgreSQLStore) UpdateTask(ctx context.Context, t *protocol.Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx,
		`UPDATE tasks SET data = $2::jsonb WHERE id = $1`,
		t.ID, data,
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("task %s not found", t.ID)
	}
	return nil
}

// CancelTask uses a single conditional UPDATE that only succeeds when the task
// is not in a terminal state, eliminating the TOCTOU race between a dispatcher
// completion and a concurrent user-initiated cancel.
func (s *PostgreSQLStore) CancelTask(ctx context.Context, id string) (*protocol.Task, error) {
	// First, check the task exists and surface the current status so we can
	// return the right sentinel error.
	existing, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err // ErrNotFound or DB error
	}
	switch existing.Status {
	case protocol.TaskCompleted, protocol.TaskFailed, protocol.TaskCancelled:
		return nil, ErrTaskTerminal
	}

	now := time.Now()
	existing.Status = protocol.TaskCancelled
	existing.CompletedAt = &now
	if existing.Error == nil {
		existing.Error = &protocol.TaskError{Code: "cancelled", Message: "cancelled by user"}
	}

	data, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}

	// Conditional UPDATE: only write if status is still non-terminal.
	// If a dispatcher races and marks it completed/failed between our GetTask
	// and this UPDATE, RowsAffected == 0 and we return ErrTaskTerminal.
	result, err := s.pool.Exec(ctx,
		`UPDATE tasks SET data = $1::jsonb
		 WHERE id = $2
		   AND data->>'status' NOT IN ('completed', 'failed', 'cancelled')`,
		data, id)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, ErrTaskTerminal
	}
	return existing, nil
}

func (s *PostgreSQLStore) ListTasks(ctx context.Context) []*protocol.Task {
	tasks, _ := pgList[protocol.Task](ctx, s.pool, "SELECT data FROM tasks ORDER BY id")
	return tasks
}

func (s *PostgreSQLStore) ListTasksByOrg(ctx context.Context, orgID string) []*protocol.Task {
	if orgID == "" {
		return s.ListTasks(ctx)
	}
	tasks, _ := pgList[protocol.Task](ctx, s.pool,
		"SELECT data FROM tasks WHERE data->'context'->>'org_id' = $1 ORDER BY id", orgID)
	return tasks
}

// — Workflows —

func (s *PostgreSQLStore) AddWorkflow(ctx context.Context, w *protocol.Workflow) error {
	return pgPut(ctx, s.pool, "workflows", w.ID, w)
}

func (s *PostgreSQLStore) GetWorkflow(ctx context.Context, id string) (*protocol.Workflow, error) {
	return pgGet[protocol.Workflow](ctx, s.pool, "workflows", id)
}

func (s *PostgreSQLStore) UpdateWorkflow(ctx context.Context, w *protocol.Workflow) error {
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `UPDATE workflows SET data = $2::jsonb WHERE id = $1`, w.ID, data)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("workflow %s not found", w.ID)
	}
	return nil
}

func (s *PostgreSQLStore) ListWorkflows(ctx context.Context) []*protocol.Workflow {
	workflows, _ := pgList[protocol.Workflow](ctx, s.pool, "SELECT data FROM workflows ORDER BY id")
	return workflows
}

// — Teams —

func (s *PostgreSQLStore) AddTeam(ctx context.Context, t *protocol.Team) error {
	return pgPut(ctx, s.pool, "teams", t.ID, t)
}

func (s *PostgreSQLStore) GetTeam(ctx context.Context, id string) (*protocol.Team, error) {
	return pgGet[protocol.Team](ctx, s.pool, "teams", id)
}

func (s *PostgreSQLStore) UpdateTeam(ctx context.Context, t *protocol.Team) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `UPDATE teams SET data = $2::jsonb WHERE id = $1`, t.ID, data)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("team %s not found", t.ID)
	}
	return nil
}

func (s *PostgreSQLStore) RemoveTeam(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "teams", id)
}

func (s *PostgreSQLStore) ListTeams(ctx context.Context) []*protocol.Team {
	teams, _ := pgList[protocol.Team](ctx, s.pool, "SELECT data FROM teams ORDER BY id")
	return teams
}

// — Knowledge —

func (s *PostgreSQLStore) AddKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error {
	return pgPut(ctx, s.pool, "knowledge", k.ID, k)
}

func (s *PostgreSQLStore) GetKnowledge(ctx context.Context, id string) (*protocol.KnowledgeEntry, error) {
	return pgGet[protocol.KnowledgeEntry](ctx, s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) UpdateKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error {
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `UPDATE knowledge SET data = $2::jsonb WHERE id = $1`, k.ID, data)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("knowledge entry %s not found", k.ID)
	}
	return nil
}

func (s *PostgreSQLStore) DeleteKnowledge(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) ListKnowledge(ctx context.Context) []*protocol.KnowledgeEntry {
	entries, _ := pgList[protocol.KnowledgeEntry](ctx, s.pool, "SELECT data FROM knowledge ORDER BY id")
	return entries
}

func (s *PostgreSQLStore) SearchKnowledge(ctx context.Context, query string) []*protocol.KnowledgeEntry {
	if query == "" {
		return s.ListKnowledge(ctx)
	}
	entries, _ := pgList[protocol.KnowledgeEntry](ctx, s.pool,
		"SELECT data FROM knowledge WHERE data->>'title' ILIKE $1 OR data->>'content' ILIKE $1",
		"%"+query+"%")
	return entries
}

// — Worker Tokens —

func (s *PostgreSQLStore) AddWorkerToken(ctx context.Context, t *protocol.WorkerToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO worker_tokens (id, data, token_hash)
         VALUES ($1, $2::jsonb, $3)
         ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, token_hash = EXCLUDED.token_hash`,
		t.ID, data, t.TokenHash)
	return err
}

func (s *PostgreSQLStore) GetWorkerToken(ctx context.Context, id string) (*protocol.WorkerToken, error) {
	return pgGetToken(ctx, s.pool, "id = $1", id)
}

func (s *PostgreSQLStore) GetWorkerTokenByHash(ctx context.Context, hash string) (*protocol.WorkerToken, error) {
	return pgGetToken(ctx, s.pool, "token_hash = $1", hash)
}

func pgGetToken(ctx context.Context, pool *pgxpool.Pool, where string, arg any) (*protocol.WorkerToken, error) {
	var data []byte
	var hash string
	err := pool.QueryRow(ctx,
		"SELECT data, token_hash FROM worker_tokens WHERE "+where, arg).Scan(&data, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t protocol.WorkerToken
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	t.TokenHash = hash
	return &t, nil
}

// UpdateWorkerToken performs a CAS update: rejects if the token is already bound
// to a different worker.
func (s *PostgreSQLStore) UpdateWorkerToken(ctx context.Context, t *protocol.WorkerToken) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var existingData []byte
	err = tx.QueryRow(ctx,
		"SELECT data FROM worker_tokens WHERE id = $1", t.ID).Scan(&existingData)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var existing protocol.WorkerToken
	if err := json.Unmarshal(existingData, &existing); err != nil {
		return err
	}
	if existing.WorkerID != "" && t.WorkerID != existing.WorkerID {
		return ErrTokenAlreadyBound
	}

	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		"UPDATE worker_tokens SET data = $2::jsonb, token_hash = $3 WHERE id = $1",
		t.ID, data, t.TokenHash)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanTokenRows(rows pgx.Rows) []*protocol.WorkerToken {
	var result []*protocol.WorkerToken
	for rows.Next() {
		var data []byte
		var hash string
		if err := rows.Scan(&data, &hash); err != nil {
			continue
		}
		var t protocol.WorkerToken
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		t.TokenHash = hash
		result = append(result, &t)
	}
	return result
}

func (s *PostgreSQLStore) ListWorkerTokensByOrg(ctx context.Context, orgID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(ctx,
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'org_id' = $1", orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func (s *PostgreSQLStore) ListWorkerTokensByWorker(ctx context.Context, workerID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(ctx,
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'worker_id' = $1", workerID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func (s *PostgreSQLStore) HasAnyWorkerTokens(ctx context.Context) bool {
	var count int
	s.pool.QueryRow(ctx, //nolint:errcheck
		"SELECT COUNT(*) FROM worker_tokens LIMIT 1").Scan(&count)
	return count > 0
}

// — Audit Log —

func (s *PostgreSQLStore) AppendAudit(ctx context.Context, e *protocol.AuditEntry) error {
	return pgPut(ctx, s.pool, "audit_log", e.ID, e)
}

func (s *PostgreSQLStore) QueryAudit(ctx context.Context, filter AuditFilter) []*protocol.AuditEntry {
	query := "SELECT data FROM audit_log WHERE 1=1"
	args := []any{}
	i := 1

	if filter.OrgID != "" {
		query += fmt.Sprintf(" AND data->>'org_id' = $%d", i)
		args = append(args, filter.OrgID)
		i++
	}
	if filter.WorkerID != "" {
		query += fmt.Sprintf(" AND data->>'worker_id' = $%d", i)
		args = append(args, filter.WorkerID)
		i++
	}
	if filter.Action != "" {
		query += fmt.Sprintf(" AND data->>'action' = $%d", i)
		args = append(args, filter.Action)
		i++
	}
	if filter.StartTime != nil {
		query += fmt.Sprintf(" AND (data->>'timestamp')::timestamptz >= $%d", i)
		args = append(args, *filter.StartTime)
		i++
	}
	if filter.EndTime != nil {
		query += fmt.Sprintf(" AND (data->>'timestamp')::timestamptz <= $%d", i)
		args = append(args, *filter.EndTime)
		i++
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", i, i+1)
	args = append(args, limit, filter.Offset)

	entries, _ := pgList[protocol.AuditEntry](ctx, s.pool, query, args...)
	return entries
}

// --- Webhooks ---
func (s *PostgreSQLStore) AddWebhook(ctx context.Context, w *protocol.Webhook) error {
	return pgPut(ctx, s.pool, "webhooks", w.ID, w)
}
func (s *PostgreSQLStore) GetWebhook(ctx context.Context, id string) (*protocol.Webhook, error) {
	return pgGet[protocol.Webhook](ctx, s.pool, "webhooks", id)
}
func (s *PostgreSQLStore) UpdateWebhook(ctx context.Context, w *protocol.Webhook) error {
	return pgPut(ctx, s.pool, "webhooks", w.ID, w)
}
func (s *PostgreSQLStore) DeleteWebhook(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "webhooks", id)
}
func (s *PostgreSQLStore) ListWebhooksByOrg(ctx context.Context, orgID string) []*protocol.Webhook {
	hooks, _ := pgList[protocol.Webhook](ctx, s.pool, "SELECT data FROM webhooks WHERE data->>'org_id' = $1 ORDER BY id", orgID)
	return hooks
}
func (s *PostgreSQLStore) FindWebhooksByEvent(ctx context.Context, eventType string) []*protocol.Webhook {
	eventJSON, _ := json.Marshal([]string{eventType})
	hooks, _ := pgList[protocol.Webhook](ctx, s.pool,
		`SELECT data FROM webhooks WHERE data->>'active' = 'true' AND data->'events' @> $1::jsonb`,
		string(eventJSON))
	return hooks
}
func (s *PostgreSQLStore) AddWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error {
	return pgPut(ctx, s.pool, "webhook_deliveries", d.ID, d)
}
func (s *PostgreSQLStore) UpdateWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error {
	return pgPut(ctx, s.pool, "webhook_deliveries", d.ID, d)
}
func (s *PostgreSQLStore) ListPendingWebhookDeliveries(ctx context.Context) []*protocol.WebhookDelivery {
	deliveries, _ := pgList[protocol.WebhookDelivery](ctx, s.pool,
		`SELECT data FROM webhook_deliveries
         WHERE data->>'status' IN ('pending', 'failed')
         AND (data->>'next_retry' IS NULL OR (data->>'next_retry')::timestamptz <= NOW())`)
	return deliveries
}

// Interface compliance check — compile-time assertion.
var _ Store = (*PostgreSQLStore)(nil)

// --- Role Bindings ---

func (s *PostgreSQLStore) AddRoleBinding(ctx context.Context, rb *protocol.RoleBinding) error {
	return pgPut(ctx, s.pool, "role_bindings", rb.ID, rb)
}
func (s *PostgreSQLStore) GetRoleBinding(ctx context.Context, id string) (*protocol.RoleBinding, error) {
	return pgGet[protocol.RoleBinding](ctx, s.pool, "role_bindings", id)
}
func (s *PostgreSQLStore) RemoveRoleBinding(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "role_bindings", id)
}
func (s *PostgreSQLStore) ListRoleBindingsByOrg(ctx context.Context, orgID string) []*protocol.RoleBinding {
	items, _ := pgList[protocol.RoleBinding](ctx, s.pool,
		`SELECT data FROM role_bindings WHERE data->>'org_id' = $1`, orgID)
	return items
}
func (s *PostgreSQLStore) FindRoleBinding(ctx context.Context, orgID, subject string) (*protocol.RoleBinding, error) {
	items, _ := pgList[protocol.RoleBinding](ctx, s.pool,
		`SELECT data FROM role_bindings WHERE data->>'org_id' = $1 AND data->>'subject' = $2`, orgID, subject)
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

// --- Policies ---

func (s *PostgreSQLStore) AddPolicy(ctx context.Context, p *protocol.Policy) error {
	return pgPut(ctx, s.pool, "policies", p.ID, p)
}
func (s *PostgreSQLStore) GetPolicy(ctx context.Context, id string) (*protocol.Policy, error) {
	return pgGet[protocol.Policy](ctx, s.pool, "policies", id)
}
func (s *PostgreSQLStore) UpdatePolicy(ctx context.Context, p *protocol.Policy) error {
	return pgPut(ctx, s.pool, "policies", p.ID, p)
}
func (s *PostgreSQLStore) RemovePolicy(ctx context.Context, id string) error {
	return pgDelete(ctx, s.pool, "policies", id)
}
func (s *PostgreSQLStore) ListPoliciesByOrg(ctx context.Context, orgID string) []*protocol.Policy {
	items, _ := pgList[protocol.Policy](ctx, s.pool,
		`SELECT data FROM policies WHERE data->>'org_id' = $1`, orgID)
	return items
}

func (s *PostgreSQLStore) AddDLQEntry(ctx context.Context, e *protocol.DLQEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO dlq (id, data) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, e.ID, string(data))
	return err
}

func (s *PostgreSQLStore) ListDLQ(ctx context.Context) []*protocol.DLQEntry {
	items, _ := pgList[protocol.DLQEntry](ctx, s.pool, `SELECT data FROM dlq ORDER BY data->>'created_at' DESC`)
	return items
}

func (s *PostgreSQLStore) AddPrompt(ctx context.Context, p *protocol.PromptTemplate) error {
	return pgPut(ctx, s.pool, "prompts", p.ID, p)
}

func (s *PostgreSQLStore) ListPrompts(ctx context.Context) []*protocol.PromptTemplate {
	items, _ := pgList[protocol.PromptTemplate](ctx, s.pool, `SELECT data FROM prompts ORDER BY data->>'created_at'`)
	return items
}

func (s *PostgreSQLStore) AddMemoryTurn(ctx context.Context, sessionID string, turn *protocol.MemoryTurn) error {
	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO memory_turns (session_id, data) VALUES ($1, $2)`, sessionID, string(data))
	return err
}

func (s *PostgreSQLStore) GetMemoryTurns(ctx context.Context, sessionID string) []*protocol.MemoryTurn {
	rows, err := s.pool.Query(ctx,
		`SELECT data FROM memory_turns WHERE session_id = $1 ORDER BY id`, sessionID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*protocol.MemoryTurn
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var t protocol.MemoryTurn
		if json.Unmarshal([]byte(data), &t) == nil {
			result = append(result, &t)
		}
	}
	return result
}
