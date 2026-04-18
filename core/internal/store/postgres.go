package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// PostgreSQLStore is a PostgreSQL-backed implementation of the Store interface.
type PostgreSQLStore struct {
	pool *pgxpool.Pool
}

// NewPostgreSQLStore creates a new PostgreSQL store using the given connection string.
func NewPostgreSQLStore(ctx context.Context, connStr string) (*PostgreSQLStore, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
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
// and invokes fn. The variable is reset to empty before returning the
// connection to the pool, so subsequent users of the pool start in bypass
// mode unless they also call WithOrgContext.
//
// When orgID is empty the connection is returned in bypass mode, which sees
// all rows — this is intentional for admin/dev paths. Pass a non-empty
// orgID from request-scoped authenticated handlers to enforce isolation.
//
// RLS defence against application bugs: even if a caller forgets to add a
// WHERE org_id = ... clause, the database will filter rows for them.
func (s *PostgreSQLStore) WithOrgContext(ctx context.Context, orgID string, fn func(conn *pgxpool.Conn) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("pool.Acquire: %w", err)
	}
	defer conn.Release()

	// set_config(name, value, is_local=false) is session-scoped and works on
	// any PG version; the literal name must be passed as text.
	if _, err := conn.Exec(ctx, "SELECT set_config('app.current_org_id', $1, false)", orgID); err != nil {
		return fmt.Errorf("set app.current_org_id: %w", err)
	}
	// Reset on return to keep pooled connections neutral.
	defer func() {
		_, _ = conn.Exec(ctx, "SELECT set_config('app.current_org_id', '', false)")
	}()

	return fn(conn)
}

// — Generic helpers —

func pgPut(pool *pgxpool.Pool, table, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = pool.Exec(context.Background(),
		"INSERT INTO "+table+" (id, data) VALUES ($1, $2::jsonb)"+
			" ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data",
		id, data)
	return err
}

func pgGet[T any](pool *pgxpool.Pool, table, id string) (*T, error) {
	var data []byte
	err := pool.QueryRow(context.Background(),
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

func pgDelete(pool *pgxpool.Pool, table, id string) error {
	result, err := pool.Exec(context.Background(),
		"DELETE FROM "+table+" WHERE id = $1", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func pgList[T any](pool *pgxpool.Pool, query string, args ...any) ([]*T, error) {
	rows, err := pool.Query(context.Background(), query, args...)
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

func (s *PostgreSQLStore) AddWorker(w *protocol.Worker) error {
	return pgPut(s.pool, "workers", w.ID, w)
}

func (s *PostgreSQLStore) GetWorker(id string) (*protocol.Worker, error) {
	return pgGet[protocol.Worker](s.pool, "workers", id)
}

func (s *PostgreSQLStore) UpdateWorker(w *protocol.Worker) error {
	if _, err := s.GetWorker(w.ID); err != nil {
		return err
	}
	return pgPut(s.pool, "workers", w.ID, w)
}

func (s *PostgreSQLStore) RemoveWorker(id string) error {
	return pgDelete(s.pool, "workers", id)
}

func (s *PostgreSQLStore) ListWorkers() []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](s.pool, "SELECT data FROM workers ORDER BY id")
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapability(capability string) []*protocol.Worker {
	workers, _ := pgList[protocol.Worker](s.pool,
		`SELECT data FROM workers
         WHERE EXISTS (
             SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
             WHERE cap->>'name' = $1
         )`, capability)
	return workers
}

func (s *PostgreSQLStore) ListWorkersByOrg(orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.ListWorkers()
	}
	workers, _ := pgList[protocol.Worker](s.pool,
		"SELECT data FROM workers WHERE data->>'org_id' = $1 ORDER BY id", orgID)
	return workers
}

func (s *PostgreSQLStore) FindWorkersByCapabilityAndOrg(capability, orgID string) []*protocol.Worker {
	if orgID == "" {
		return s.FindWorkersByCapability(capability)
	}
	workers, _ := pgList[protocol.Worker](s.pool,
		`SELECT data FROM workers
         WHERE data->>'org_id' = $1
         AND EXISTS (
             SELECT 1 FROM jsonb_array_elements(data->'capabilities') AS cap
             WHERE cap->>'name' = $2
         )`, orgID, capability)
	return workers
}

// — Tasks —

func (s *PostgreSQLStore) AddTask(t *protocol.Task) error {
	return pgPut(s.pool, "tasks", t.ID, t)
}

func (s *PostgreSQLStore) GetTask(id string) (*protocol.Task, error) {
	return pgGet[protocol.Task](s.pool, "tasks", id)
}

func (s *PostgreSQLStore) UpdateTask(t *protocol.Task) error {
	if _, err := s.GetTask(t.ID); err != nil {
		return err
	}
	return pgPut(s.pool, "tasks", t.ID, t)
}

func (s *PostgreSQLStore) ListTasks() []*protocol.Task {
	tasks, _ := pgList[protocol.Task](s.pool, "SELECT data FROM tasks ORDER BY id")
	return tasks
}

func (s *PostgreSQLStore) ListTasksByOrg(orgID string) []*protocol.Task {
	if orgID == "" {
		return s.ListTasks()
	}
	// Tasks without context.org_id are excluded (they have no org association).
	tasks, _ := pgList[protocol.Task](s.pool,
		"SELECT data FROM tasks WHERE data->'context'->>'org_id' = $1 ORDER BY id", orgID)
	return tasks
}

// — Workflows —

func (s *PostgreSQLStore) AddWorkflow(w *protocol.Workflow) error {
	return pgPut(s.pool, "workflows", w.ID, w)
}

func (s *PostgreSQLStore) GetWorkflow(id string) (*protocol.Workflow, error) {
	return pgGet[protocol.Workflow](s.pool, "workflows", id)
}

func (s *PostgreSQLStore) UpdateWorkflow(w *protocol.Workflow) error {
	if _, err := s.GetWorkflow(w.ID); err != nil {
		return err
	}
	return pgPut(s.pool, "workflows", w.ID, w)
}

func (s *PostgreSQLStore) ListWorkflows() []*protocol.Workflow {
	workflows, _ := pgList[protocol.Workflow](s.pool, "SELECT data FROM workflows ORDER BY id")
	return workflows
}

// — Teams —

func (s *PostgreSQLStore) AddTeam(t *protocol.Team) error {
	return pgPut(s.pool, "teams", t.ID, t)
}

func (s *PostgreSQLStore) GetTeam(id string) (*protocol.Team, error) {
	return pgGet[protocol.Team](s.pool, "teams", id)
}

func (s *PostgreSQLStore) UpdateTeam(t *protocol.Team) error {
	if _, err := s.GetTeam(t.ID); err != nil {
		return err
	}
	return pgPut(s.pool, "teams", t.ID, t)
}

func (s *PostgreSQLStore) RemoveTeam(id string) error {
	return pgDelete(s.pool, "teams", id)
}

func (s *PostgreSQLStore) ListTeams() []*protocol.Team {
	teams, _ := pgList[protocol.Team](s.pool, "SELECT data FROM teams ORDER BY id")
	return teams
}

// — Knowledge —

func (s *PostgreSQLStore) AddKnowledge(k *protocol.KnowledgeEntry) error {
	return pgPut(s.pool, "knowledge", k.ID, k)
}

func (s *PostgreSQLStore) GetKnowledge(id string) (*protocol.KnowledgeEntry, error) {
	return pgGet[protocol.KnowledgeEntry](s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) UpdateKnowledge(k *protocol.KnowledgeEntry) error {
	if _, err := s.GetKnowledge(k.ID); err != nil {
		return err
	}
	return pgPut(s.pool, "knowledge", k.ID, k)
}

func (s *PostgreSQLStore) DeleteKnowledge(id string) error {
	return pgDelete(s.pool, "knowledge", id)
}

func (s *PostgreSQLStore) ListKnowledge() []*protocol.KnowledgeEntry {
	entries, _ := pgList[protocol.KnowledgeEntry](s.pool, "SELECT data FROM knowledge ORDER BY id")
	return entries
}

func (s *PostgreSQLStore) SearchKnowledge(query string) []*protocol.KnowledgeEntry {
	if query == "" {
		return s.ListKnowledge()
	}
	entries, _ := pgList[protocol.KnowledgeEntry](s.pool,
		"SELECT data FROM knowledge WHERE data->>'title' ILIKE $1 OR data->>'content' ILIKE $1",
		"%"+query+"%")
	return entries
}

// — Worker Tokens —
// worker_tokens has a dedicated token_hash column (TokenHash has json:"-" so it is not in JSONB).

func (s *PostgreSQLStore) AddWorkerToken(t *protocol.WorkerToken) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO worker_tokens (id, data, token_hash)
         VALUES ($1, $2::jsonb, $3)
         ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, token_hash = EXCLUDED.token_hash`,
		t.ID, data, t.TokenHash)
	return err
}

func (s *PostgreSQLStore) GetWorkerToken(id string) (*protocol.WorkerToken, error) {
	return pgGetToken(s.pool, "id = $1", id)
}

// GetWorkerTokenByHash looks up a token by its hash.
// NOTE: Returns token regardless of validity state (expired or revoked).
// Callers MUST call token.IsValid() before using the token.
func (s *PostgreSQLStore) GetWorkerTokenByHash(hash string) (*protocol.WorkerToken, error) {
	return pgGetToken(s.pool, "token_hash = $1", hash)
}

func pgGetToken(pool *pgxpool.Pool, where string, arg any) (*protocol.WorkerToken, error) {
	var data []byte
	var hash string
	err := pool.QueryRow(context.Background(),
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
func (s *PostgreSQLStore) UpdateWorkerToken(t *protocol.WorkerToken) error {
	ctx := context.Background()
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

func (s *PostgreSQLStore) ListWorkerTokensByOrg(orgID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(context.Background(),
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'org_id' = $1", orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func (s *PostgreSQLStore) ListWorkerTokensByWorker(workerID string) []*protocol.WorkerToken {
	rows, err := s.pool.Query(context.Background(),
		"SELECT data, token_hash FROM worker_tokens WHERE data->>'worker_id' = $1", workerID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanTokenRows(rows)
}

func (s *PostgreSQLStore) HasAnyWorkerTokens() bool {
	var count int
	s.pool.QueryRow(context.Background(), //nolint:errcheck
		"SELECT COUNT(*) FROM worker_tokens LIMIT 1").Scan(&count)
	return count > 0
}

// — Audit Log —

func (s *PostgreSQLStore) AppendAudit(e *protocol.AuditEntry) error {
	return pgPut(s.pool, "audit_log", e.ID, e)
}

func (s *PostgreSQLStore) QueryAudit(filter AuditFilter) []*protocol.AuditEntry {
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

	entries, _ := pgList[protocol.AuditEntry](s.pool, query, args...)
	return entries
}

// --- Webhook stubs (full implementation in Phase 3b Task 4) ---
func (s *PostgreSQLStore) AddWebhook(w *protocol.Webhook) error { return pgPut(s.pool, "webhooks", w.ID, w) }
func (s *PostgreSQLStore) GetWebhook(id string) (*protocol.Webhook, error) {
	return pgGet[protocol.Webhook](s.pool, "webhooks", id)
}
func (s *PostgreSQLStore) UpdateWebhook(w *protocol.Webhook) error { return pgPut(s.pool, "webhooks", w.ID, w) }
func (s *PostgreSQLStore) DeleteWebhook(id string) error           { return pgDelete(s.pool, "webhooks", id) }
func (s *PostgreSQLStore) ListWebhooksByOrg(orgID string) []*protocol.Webhook {
	hooks, _ := pgList[protocol.Webhook](s.pool, "SELECT data FROM webhooks WHERE data->>'org_id' = $1 ORDER BY id", orgID)
	return hooks
}
func (s *PostgreSQLStore) FindWebhooksByEvent(eventType string) []*protocol.Webhook {
	// Use json.Marshal to safely build the JSONB array — never concat eventType directly.
	eventJSON, _ := json.Marshal([]string{eventType})
	hooks, _ := pgList[protocol.Webhook](s.pool,
		`SELECT data FROM webhooks WHERE data->>'active' = 'true' AND data->'events' @> $1::jsonb`,
		string(eventJSON))
	return hooks
}
func (s *PostgreSQLStore) AddWebhookDelivery(d *protocol.WebhookDelivery) error {
	return pgPut(s.pool, "webhook_deliveries", d.ID, d)
}
func (s *PostgreSQLStore) UpdateWebhookDelivery(d *protocol.WebhookDelivery) error {
	return pgPut(s.pool, "webhook_deliveries", d.ID, d)
}
func (s *PostgreSQLStore) ListPendingWebhookDeliveries() []*protocol.WebhookDelivery {
	deliveries, _ := pgList[protocol.WebhookDelivery](s.pool,
		`SELECT data FROM webhook_deliveries
         WHERE data->>'status' IN ('pending', 'failed')
         AND (data->>'next_retry' IS NULL OR (data->>'next_retry')::timestamptz <= NOW())`)
	return deliveries
}

// Interface compliance check — compile-time assertion.
var _ Store = (*PostgreSQLStore)(nil)

// --- Role Bindings ---

func (s *PostgreSQLStore) AddRoleBinding(rb *protocol.RoleBinding) error {
	return pgPut(s.pool, "role_bindings", rb.ID, rb)
}
func (s *PostgreSQLStore) GetRoleBinding(id string) (*protocol.RoleBinding, error) {
	return pgGet[protocol.RoleBinding](s.pool, "role_bindings", id)
}
func (s *PostgreSQLStore) RemoveRoleBinding(id string) error {
	return pgDelete(s.pool, "role_bindings", id)
}
func (s *PostgreSQLStore) ListRoleBindingsByOrg(orgID string) []*protocol.RoleBinding {
	items, _ := pgList[protocol.RoleBinding](s.pool,
		`SELECT data FROM role_bindings WHERE data->>'org_id' = $1`, orgID)
	return items
}
func (s *PostgreSQLStore) FindRoleBinding(orgID, subject string) (*protocol.RoleBinding, error) {
	items, _ := pgList[protocol.RoleBinding](s.pool,
		`SELECT data FROM role_bindings WHERE data->>'org_id' = $1 AND data->>'subject' = $2`, orgID, subject)
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

// --- Policies ---

func (s *PostgreSQLStore) AddPolicy(p *protocol.Policy) error {
	return pgPut(s.pool, "policies", p.ID, p)
}
func (s *PostgreSQLStore) GetPolicy(id string) (*protocol.Policy, error) {
	return pgGet[protocol.Policy](s.pool, "policies", id)
}
func (s *PostgreSQLStore) UpdatePolicy(p *protocol.Policy) error {
	return pgPut(s.pool, "policies", p.ID, p)
}
func (s *PostgreSQLStore) RemovePolicy(id string) error {
	return pgDelete(s.pool, "policies", id)
}
func (s *PostgreSQLStore) ListPoliciesByOrg(orgID string) []*protocol.Policy {
	items, _ := pgList[protocol.Policy](s.pool,
		`SELECT data FROM policies WHERE data->>'org_id' = $1`, orgID)
	return items
}

func (s *PostgreSQLStore) AddDLQEntry(e *protocol.DLQEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO dlq (id, data) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, e.ID, string(data))
	return err
}

func (s *PostgreSQLStore) ListDLQ() []*protocol.DLQEntry {
	items, _ := pgList[protocol.DLQEntry](s.pool, `SELECT data FROM dlq ORDER BY data->>'created_at' DESC`)
	return items
}

func (s *PostgreSQLStore) AddPrompt(p *protocol.PromptTemplate) error {
	return pgPut(s.pool, "prompts", p.ID, p)
}

func (s *PostgreSQLStore) ListPrompts() []*protocol.PromptTemplate {
	items, _ := pgList[protocol.PromptTemplate](s.pool, `SELECT data FROM prompts ORDER BY data->>'created_at'`)
	return items
}

func (s *PostgreSQLStore) AddMemoryTurn(sessionID string, turn *protocol.MemoryTurn) error {
	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO memory_turns (session_id, data) VALUES ($1, $2)`, sessionID, string(data))
	return err
}

func (s *PostgreSQLStore) GetMemoryTurns(sessionID string) []*protocol.MemoryTurn {
	rows, err := s.pool.Query(context.Background(),
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
