package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// SQLiteStore is a SQLite-backed implementation of the Store interface.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store at the given path.
// Use ":memory:" for in-memory SQLite or a file path for persistent storage.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Create tables
	tables := []string{
		`CREATE TABLE IF NOT EXISTS workers (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS teams (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS knowledge (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS worker_tokens (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS audit_log (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS webhooks (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS webhook_deliveries (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS role_bindings (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS policies (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS dlq (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS prompts (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS memory_turns (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, data TEXT NOT NULL)`,
	}
	for _, ddl := range tables {
		if _, err := db.Exec(ddl); err != nil {
			return nil, err
		}
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Generic helpers
func putJSON(ctx context.Context, db *sql.DB, table, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		"INSERT OR REPLACE INTO "+table+" (id, data) VALUES (?, ?)",
		id, string(data),
	)
	return err
}

func getJSON[T any](ctx context.Context, db *sql.DB, table, id string) (*T, error) {
	var data string
	err := db.QueryRowContext(ctx, "SELECT data FROM "+table+" WHERE id = ?", id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func deleteRow(ctx context.Context, db *sql.DB, table, id string) error {
	result, err := db.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func listJSON[T any](ctx context.Context, db *sql.DB, table string) ([]*T, error) {
	rows, err := db.QueryContext(ctx, "SELECT data FROM "+table+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*T
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		result = append(result, &v)
	}
	return result, nil
}

// Workers
func (s *SQLiteStore) AddWorker(ctx context.Context, w *protocol.Worker) error {
	return putJSON(ctx, s.db, "workers", w.ID, w)
}
func (s *SQLiteStore) GetWorker(ctx context.Context, id string) (*protocol.Worker, error) {
	return getJSON[protocol.Worker](ctx, s.db, "workers", id)
}
func (s *SQLiteStore) UpdateWorker(ctx context.Context, w *protocol.Worker) error {
	if _, err := s.GetWorker(ctx, w.ID); err != nil {
		return err
	}
	return putJSON(ctx, s.db, "workers", w.ID, w)
}
func (s *SQLiteStore) RemoveWorker(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "workers", id)
}
func (s *SQLiteStore) ListWorkers(ctx context.Context) []*protocol.Worker {
	r, _ := listJSON[protocol.Worker](ctx, s.db, "workers")
	return r
}
func (s *SQLiteStore) FindWorkersByCapability(ctx context.Context, capability string) []*protocol.Worker {
	workers := s.ListWorkers(ctx)
	var result []*protocol.Worker
	for _, w := range workers {
		if w.Status != protocol.StatusActive {
			continue
		}
		for _, cap := range w.Capabilities {
			if cap.Name == capability {
				result = append(result, w)
				break
			}
		}
	}
	return result
}

// Tasks
func (s *SQLiteStore) AddTask(ctx context.Context, t *protocol.Task) error {
	return putJSON(ctx, s.db, "tasks", t.ID, t)
}
func (s *SQLiteStore) GetTask(ctx context.Context, id string) (*protocol.Task, error) {
	return getJSON[protocol.Task](ctx, s.db, "tasks", id)
}
func (s *SQLiteStore) UpdateTask(ctx context.Context, t *protocol.Task) error {
	if _, err := s.GetTask(ctx, t.ID); err != nil {
		return err
	}
	return putJSON(ctx, s.db, "tasks", t.ID, t)
}

// CancelTask atomically transitions the task to cancelled using a transaction
// with a conditional check so that concurrent completions are not overwritten.
func (s *SQLiteStore) CancelTask(ctx context.Context, id string) (*protocol.Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var raw string
	err = tx.QueryRowContext(ctx, "SELECT data FROM tasks WHERE id = ?", id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var t protocol.Task
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, err
	}
	switch t.Status {
	case protocol.TaskCompleted, protocol.TaskFailed, protocol.TaskCancelled:
		return nil, ErrTaskTerminal
	}

	now := time.Now()
	t.Status = protocol.TaskCancelled
	t.CompletedAt = &now
	if t.Error == nil {
		t.Error = &protocol.TaskError{Code: "cancelled", Message: "cancelled by user"}
	}

	updated, err := json.Marshal(&t)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE tasks SET data = ? WHERE id = ?", string(updated), id); err != nil {
		return nil, err
	}
	return &t, tx.Commit()
}

func (s *SQLiteStore) ListTasks(ctx context.Context) []*protocol.Task {
	r, _ := listJSON[protocol.Task](ctx, s.db, "tasks")
	return r
}

// Workflows
func (s *SQLiteStore) AddWorkflow(ctx context.Context, w *protocol.Workflow) error {
	return putJSON(ctx, s.db, "workflows", w.ID, w)
}
func (s *SQLiteStore) GetWorkflow(ctx context.Context, id string) (*protocol.Workflow, error) {
	return getJSON[protocol.Workflow](ctx, s.db, "workflows", id)
}
func (s *SQLiteStore) UpdateWorkflow(ctx context.Context, w *protocol.Workflow) error {
	if _, err := s.GetWorkflow(ctx, w.ID); err != nil {
		return err
	}
	return putJSON(ctx, s.db, "workflows", w.ID, w)
}
func (s *SQLiteStore) ListWorkflows(ctx context.Context) []*protocol.Workflow {
	r, _ := listJSON[protocol.Workflow](ctx, s.db, "workflows")
	return r
}

// Teams
func (s *SQLiteStore) AddTeam(ctx context.Context, t *protocol.Team) error {
	return putJSON(ctx, s.db, "teams", t.ID, t)
}
func (s *SQLiteStore) GetTeam(ctx context.Context, id string) (*protocol.Team, error) {
	return getJSON[protocol.Team](ctx, s.db, "teams", id)
}
func (s *SQLiteStore) UpdateTeam(ctx context.Context, t *protocol.Team) error {
	if _, err := s.GetTeam(ctx, t.ID); err != nil {
		return err
	}
	return putJSON(ctx, s.db, "teams", t.ID, t)
}
func (s *SQLiteStore) RemoveTeam(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "teams", id)
}
func (s *SQLiteStore) ListTeams(ctx context.Context) []*protocol.Team {
	r, _ := listJSON[protocol.Team](ctx, s.db, "teams")
	return r
}

// Knowledge
func (s *SQLiteStore) AddKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error {
	return putJSON(ctx, s.db, "knowledge", k.ID, k)
}
func (s *SQLiteStore) GetKnowledge(ctx context.Context, id string) (*protocol.KnowledgeEntry, error) {
	return getJSON[protocol.KnowledgeEntry](ctx, s.db, "knowledge", id)
}
func (s *SQLiteStore) UpdateKnowledge(ctx context.Context, k *protocol.KnowledgeEntry) error {
	if _, err := s.GetKnowledge(ctx, k.ID); err != nil {
		return err
	}
	return putJSON(ctx, s.db, "knowledge", k.ID, k)
}
func (s *SQLiteStore) DeleteKnowledge(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "knowledge", id)
}
func (s *SQLiteStore) ListKnowledge(ctx context.Context) []*protocol.KnowledgeEntry {
	r, _ := listJSON[protocol.KnowledgeEntry](ctx, s.db, "knowledge")
	return r
}
func (s *SQLiteStore) SearchKnowledge(ctx context.Context, query string) []*protocol.KnowledgeEntry {
	rows, err := s.db.QueryContext(ctx,
		"SELECT data FROM knowledge WHERE LOWER(data) LIKE '%' || LOWER(?) || '%' ORDER BY id",
		query,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*protocol.KnowledgeEntry
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var k protocol.KnowledgeEntry
		if err := json.Unmarshal([]byte(data), &k); err != nil {
			continue
		}
		result = append(result, &k)
	}
	return result
}

// Worker tokens
func (s *SQLiteStore) AddWorkerToken(ctx context.Context, t *protocol.WorkerToken) error {
	return putJSON(ctx, s.db, "worker_tokens", t.ID, t)
}
func (s *SQLiteStore) GetWorkerToken(ctx context.Context, id string) (*protocol.WorkerToken, error) {
	return getJSON[protocol.WorkerToken](ctx, s.db, "worker_tokens", id)
}

// GetWorkerTokenByHash looks up a token by its hash.
func (s *SQLiteStore) GetWorkerTokenByHash(ctx context.Context, hash string) (*protocol.WorkerToken, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT data FROM worker_tokens ORDER BY id")
	if err != nil {
		return nil, ErrNotFound
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var t protocol.WorkerToken
		if err := json.Unmarshal([]byte(data), &t); err != nil {
			continue
		}
		if t.TokenHash == hash {
			return &t, nil
		}
	}
	return nil, ErrNotFound
}

func (s *SQLiteStore) UpdateWorkerToken(ctx context.Context, t *protocol.WorkerToken) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var data string
	err = tx.QueryRowContext(ctx, "SELECT data FROM worker_tokens WHERE id = ?", t.ID).Scan(&data)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var existing protocol.WorkerToken
	if err := json.Unmarshal([]byte(data), &existing); err != nil {
		return err
	}
	if existing.WorkerID != "" && t.WorkerID != existing.WorkerID {
		return fmt.Errorf("token already in use")
	}

	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO worker_tokens (id, data) VALUES (?, ?)", t.ID, string(b))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *SQLiteStore) ListWorkerTokensByOrg(ctx context.Context, orgID string) []*protocol.WorkerToken {
	all, _ := listJSON[protocol.WorkerToken](ctx, s.db, "worker_tokens")
	var result []*protocol.WorkerToken
	for _, t := range all {
		if t.OrgID == orgID {
			result = append(result, t)
		}
	}
	return result
}
func (s *SQLiteStore) ListWorkerTokensByWorker(ctx context.Context, workerID string) []*protocol.WorkerToken {
	all, _ := listJSON[protocol.WorkerToken](ctx, s.db, "worker_tokens")
	var result []*protocol.WorkerToken
	for _, t := range all {
		if t.WorkerID == workerID {
			result = append(result, t)
		}
	}
	return result
}
func (s *SQLiteStore) HasAnyWorkerTokens(ctx context.Context) bool {
	var count int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_tokens LIMIT 1").Scan(&count) //nolint:errcheck
	return count > 0
}

// Audit log
func (s *SQLiteStore) AppendAudit(ctx context.Context, e *protocol.AuditEntry) error {
	return putJSON(ctx, s.db, "audit_log", e.ID, e)
}
func (s *SQLiteStore) QueryAudit(ctx context.Context, filter AuditFilter) []*protocol.AuditEntry {
	all, _ := listJSON[protocol.AuditEntry](ctx, s.db, "audit_log")
	var result []*protocol.AuditEntry
	for _, e := range all {
		if filter.OrgID != "" && e.OrgID != filter.OrgID {
			continue
		}
		if filter.WorkerID != "" && e.WorkerID != filter.WorkerID {
			continue
		}
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		if filter.StartTime != nil && e.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && e.Timestamp.After(*filter.EndTime) {
			continue
		}
		result = append(result, e)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset >= len(result) {
		return nil
	}
	result = result[offset:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// Org-scoped queries
func (s *SQLiteStore) ListWorkersByOrg(ctx context.Context, orgID string) []*protocol.Worker {
	all := s.ListWorkers(ctx)
	if orgID == "" {
		return all
	}
	var result []*protocol.Worker
	for _, w := range all {
		if w.OrgID == orgID {
			result = append(result, w)
		}
	}
	return result
}
func (s *SQLiteStore) ListTasksByOrg(ctx context.Context, orgID string) []*protocol.Task {
	all := s.ListTasks(ctx)
	if orgID == "" {
		return all
	}
	var result []*protocol.Task
	for _, t := range all {
		if t.Context.OrgID == orgID {
			result = append(result, t)
		}
	}
	return result
}
func (s *SQLiteStore) FindWorkersByCapabilityAndOrg(ctx context.Context, capability, orgID string) []*protocol.Worker {
	all := s.FindWorkersByCapability(ctx, capability)
	if orgID == "" {
		return all
	}
	var result []*protocol.Worker
	for _, w := range all {
		if w.OrgID == orgID {
			result = append(result, w)
		}
	}
	return result
}

// --- Webhook methods ---
func (s *SQLiteStore) AddWebhook(ctx context.Context, w *protocol.Webhook) error {
	return putJSON(ctx, s.db, "webhooks", w.ID, w)
}
func (s *SQLiteStore) GetWebhook(ctx context.Context, id string) (*protocol.Webhook, error) {
	return getJSON[protocol.Webhook](ctx, s.db, "webhooks", id)
}
func (s *SQLiteStore) UpdateWebhook(ctx context.Context, w *protocol.Webhook) error {
	return putJSON(ctx, s.db, "webhooks", w.ID, w)
}
func (s *SQLiteStore) DeleteWebhook(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "webhooks", id)
}
func (s *SQLiteStore) ListWebhooksByOrg(ctx context.Context, orgID string) []*protocol.Webhook {
	all, _ := listJSON[protocol.Webhook](ctx, s.db, "webhooks")
	var result []*protocol.Webhook
	for _, w := range all {
		if w.OrgID == orgID {
			result = append(result, w)
		}
	}
	return result
}
func (s *SQLiteStore) FindWebhooksByEvent(ctx context.Context, eventType string) []*protocol.Webhook {
	all, _ := listJSON[protocol.Webhook](ctx, s.db, "webhooks")
	var result []*protocol.Webhook
	for _, w := range all {
		if !w.Active {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				result = append(result, w)
				break
			}
		}
	}
	return result
}
func (s *SQLiteStore) AddWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error {
	return putJSON(ctx, s.db, "webhook_deliveries", d.ID, d)
}
func (s *SQLiteStore) UpdateWebhookDelivery(ctx context.Context, d *protocol.WebhookDelivery) error {
	return putJSON(ctx, s.db, "webhook_deliveries", d.ID, d)
}
func (s *SQLiteStore) ListPendingWebhookDeliveries(ctx context.Context) []*protocol.WebhookDelivery {
	all, _ := listJSON[protocol.WebhookDelivery](ctx, s.db, "webhook_deliveries")
	now := time.Now()
	var result []*protocol.WebhookDelivery
	for _, d := range all {
		if d.Status == protocol.DeliveryPending || d.Status == protocol.DeliveryFailed {
			if d.NextRetry == nil || d.NextRetry.Before(now) {
				result = append(result, d)
			}
		}
	}
	return result
}

// --- Role Bindings ---

func (s *SQLiteStore) AddRoleBinding(ctx context.Context, rb *protocol.RoleBinding) error {
	return putJSON(ctx, s.db, "role_bindings", rb.ID, rb)
}
func (s *SQLiteStore) GetRoleBinding(ctx context.Context, id string) (*protocol.RoleBinding, error) {
	return getJSON[protocol.RoleBinding](ctx, s.db, "role_bindings", id)
}
func (s *SQLiteStore) RemoveRoleBinding(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "role_bindings", id)
}
func (s *SQLiteStore) ListRoleBindingsByOrg(ctx context.Context, orgID string) []*protocol.RoleBinding {
	all, _ := listJSON[protocol.RoleBinding](ctx, s.db, "role_bindings")
	var result []*protocol.RoleBinding
	for _, rb := range all {
		if rb.OrgID == orgID {
			result = append(result, rb)
		}
	}
	return result
}
func (s *SQLiteStore) FindRoleBinding(ctx context.Context, orgID, subject string) (*protocol.RoleBinding, error) {
	all, _ := listJSON[protocol.RoleBinding](ctx, s.db, "role_bindings")
	for _, rb := range all {
		if rb.OrgID == orgID && rb.Subject == subject {
			return rb, nil
		}
	}
	return nil, ErrNotFound
}

// --- Policies ---

func (s *SQLiteStore) AddPolicy(ctx context.Context, p *protocol.Policy) error {
	return putJSON(ctx, s.db, "policies", p.ID, p)
}
func (s *SQLiteStore) GetPolicy(ctx context.Context, id string) (*protocol.Policy, error) {
	return getJSON[protocol.Policy](ctx, s.db, "policies", id)
}
func (s *SQLiteStore) UpdatePolicy(ctx context.Context, p *protocol.Policy) error {
	return putJSON(ctx, s.db, "policies", p.ID, p)
}
func (s *SQLiteStore) RemovePolicy(ctx context.Context, id string) error {
	return deleteRow(ctx, s.db, "policies", id)
}
func (s *SQLiteStore) ListPoliciesByOrg(ctx context.Context, orgID string) []*protocol.Policy {
	all, _ := listJSON[protocol.Policy](ctx, s.db, "policies")
	var result []*protocol.Policy
	for _, p := range all {
		if p.OrgID == orgID {
			result = append(result, p)
		}
	}
	return result
}

func (s *SQLiteStore) AddDLQEntry(ctx context.Context, e *protocol.DLQEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO dlq (id, data) VALUES (?, ?)`, e.ID, string(data))
	return err
}

func (s *SQLiteStore) ListDLQ(ctx context.Context) []*protocol.DLQEntry {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM dlq ORDER BY rowid DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*protocol.DLQEntry
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var e protocol.DLQEntry
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			continue
		}
		result = append(result, &e)
	}
	return result
}

func (s *SQLiteStore) AddPrompt(ctx context.Context, p *protocol.PromptTemplate) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO prompts (id, data) VALUES (?, ?)`, p.ID, string(data))
	return err
}

func (s *SQLiteStore) ListPrompts(ctx context.Context) []*protocol.PromptTemplate {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM prompts ORDER BY rowid`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*protocol.PromptTemplate
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var p protocol.PromptTemplate
		if json.Unmarshal([]byte(data), &p) == nil {
			result = append(result, &p)
		}
	}
	return result
}

func (s *SQLiteStore) AddMemoryTurn(ctx context.Context, sessionID string, turn *protocol.MemoryTurn) error {
	data, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO memory_turns (session_id, data) VALUES (?, ?)`, sessionID, string(data))
	return err
}

func (s *SQLiteStore) GetMemoryTurns(ctx context.Context, sessionID string) []*protocol.MemoryTurn {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM memory_turns WHERE session_id = ? ORDER BY id`, sessionID)
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
