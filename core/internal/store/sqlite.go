package store

import (
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
func putJSON(db *sql.DB, table, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		"INSERT OR REPLACE INTO "+table+" (id, data) VALUES (?, ?)",
		id, string(data),
	)
	return err
}

func getJSON[T any](db *sql.DB, table, id string) (*T, error) {
	var data string
	err := db.QueryRow("SELECT data FROM "+table+" WHERE id = ?", id).Scan(&data)
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

func deleteRow(db *sql.DB, table, id string) error {
	result, err := db.Exec("DELETE FROM "+table+" WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func listJSON[T any](db *sql.DB, table string) ([]*T, error) {
	rows, err := db.Query("SELECT data FROM " + table + " ORDER BY id")
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
func (s *SQLiteStore) AddWorker(w *protocol.Worker) error { return putJSON(s.db, "workers", w.ID, w) }
func (s *SQLiteStore) GetWorker(id string) (*protocol.Worker, error) {
	return getJSON[protocol.Worker](s.db, "workers", id)
}
func (s *SQLiteStore) UpdateWorker(w *protocol.Worker) error {
	// Check exists first
	if _, err := s.GetWorker(w.ID); err != nil {
		return err
	}
	return putJSON(s.db, "workers", w.ID, w)
}
func (s *SQLiteStore) RemoveWorker(id string) error { return deleteRow(s.db, "workers", id) }
func (s *SQLiteStore) ListWorkers() []*protocol.Worker {
	r, _ := listJSON[protocol.Worker](s.db, "workers")
	return r
}
func (s *SQLiteStore) FindWorkersByCapability(capability string) []*protocol.Worker {
	workers := s.ListWorkers()
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
func (s *SQLiteStore) AddTask(t *protocol.Task) error { return putJSON(s.db, "tasks", t.ID, t) }
func (s *SQLiteStore) GetTask(id string) (*protocol.Task, error) {
	return getJSON[protocol.Task](s.db, "tasks", id)
}
func (s *SQLiteStore) UpdateTask(t *protocol.Task) error {
	if _, err := s.GetTask(t.ID); err != nil {
		return err
	}
	return putJSON(s.db, "tasks", t.ID, t)
}
func (s *SQLiteStore) ListTasks() []*protocol.Task {
	r, _ := listJSON[protocol.Task](s.db, "tasks")
	return r
}

// Workflows
func (s *SQLiteStore) AddWorkflow(w *protocol.Workflow) error {
	return putJSON(s.db, "workflows", w.ID, w)
}
func (s *SQLiteStore) GetWorkflow(id string) (*protocol.Workflow, error) {
	return getJSON[protocol.Workflow](s.db, "workflows", id)
}
func (s *SQLiteStore) UpdateWorkflow(w *protocol.Workflow) error {
	if _, err := s.GetWorkflow(w.ID); err != nil {
		return err
	}
	return putJSON(s.db, "workflows", w.ID, w)
}
func (s *SQLiteStore) ListWorkflows() []*protocol.Workflow {
	r, _ := listJSON[protocol.Workflow](s.db, "workflows")
	return r
}

// Teams
func (s *SQLiteStore) AddTeam(t *protocol.Team) error { return putJSON(s.db, "teams", t.ID, t) }
func (s *SQLiteStore) GetTeam(id string) (*protocol.Team, error) {
	return getJSON[protocol.Team](s.db, "teams", id)
}
func (s *SQLiteStore) UpdateTeam(t *protocol.Team) error {
	if _, err := s.GetTeam(t.ID); err != nil {
		return err
	}
	return putJSON(s.db, "teams", t.ID, t)
}
func (s *SQLiteStore) RemoveTeam(id string) error { return deleteRow(s.db, "teams", id) }
func (s *SQLiteStore) ListTeams() []*protocol.Team {
	r, _ := listJSON[protocol.Team](s.db, "teams")
	return r
}

// Knowledge
func (s *SQLiteStore) AddKnowledge(k *protocol.KnowledgeEntry) error {
	return putJSON(s.db, "knowledge", k.ID, k)
}
func (s *SQLiteStore) GetKnowledge(id string) (*protocol.KnowledgeEntry, error) {
	return getJSON[protocol.KnowledgeEntry](s.db, "knowledge", id)
}
func (s *SQLiteStore) UpdateKnowledge(k *protocol.KnowledgeEntry) error {
	if _, err := s.GetKnowledge(k.ID); err != nil {
		return err
	}
	return putJSON(s.db, "knowledge", k.ID, k)
}
func (s *SQLiteStore) DeleteKnowledge(id string) error {
	return deleteRow(s.db, "knowledge", id)
}
func (s *SQLiteStore) ListKnowledge() []*protocol.KnowledgeEntry {
	r, _ := listJSON[protocol.KnowledgeEntry](s.db, "knowledge")
	return r
}
func (s *SQLiteStore) SearchKnowledge(query string) []*protocol.KnowledgeEntry {
	// Use SQL LIKE for search
	rows, err := s.db.Query(
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

// Worker tokens — not yet implemented for SQLite; use MemoryStore for token operations.
func (s *SQLiteStore) AddWorkerToken(t *protocol.WorkerToken) error {
	return putJSON(s.db, "worker_tokens", t.ID, t)
}
func (s *SQLiteStore) GetWorkerToken(id string) (*protocol.WorkerToken, error) {
	return getJSON[protocol.WorkerToken](s.db, "worker_tokens", id)
}
// GetWorkerTokenByHash looks up a token by its hash.
// NOTE: Returns token regardless of validity state (expired or revoked).
// Callers MUST call token.IsValid() before using the token.
// This allows callers to distinguish "token not found" from "token expired/revoked".
func (s *SQLiteStore) GetWorkerTokenByHash(hash string) (*protocol.WorkerToken, error) {
	rows, err := s.db.Query("SELECT data FROM worker_tokens ORDER BY id")
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
func (s *SQLiteStore) UpdateWorkerToken(t *protocol.WorkerToken) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Read current state inside the transaction for atomic CAS.
	var data string
	err = tx.QueryRow("SELECT data FROM worker_tokens WHERE id = ?", t.ID).Scan(&data)
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
	// CAS: if the token is already bound to a different worker, reject.
	if existing.WorkerID != "" && t.WorkerID != existing.WorkerID {
		return fmt.Errorf("token already in use")
	}

	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT OR REPLACE INTO worker_tokens (id, data) VALUES (?, ?)", t.ID, string(b))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *SQLiteStore) ListWorkerTokensByOrg(orgID string) []*protocol.WorkerToken {
	all, _ := listJSON[protocol.WorkerToken](s.db, "worker_tokens")
	var result []*protocol.WorkerToken
	for _, t := range all {
		if t.OrgID == orgID {
			result = append(result, t)
		}
	}
	return result
}
func (s *SQLiteStore) ListWorkerTokensByWorker(workerID string) []*protocol.WorkerToken {
	all, _ := listJSON[protocol.WorkerToken](s.db, "worker_tokens")
	var result []*protocol.WorkerToken
	for _, t := range all {
		if t.WorkerID == workerID {
			result = append(result, t)
		}
	}
	return result
}
func (s *SQLiteStore) HasAnyWorkerTokens() bool {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM worker_tokens LIMIT 1").Scan(&count) //nolint:errcheck
	return count > 0
}

// Audit log — not yet implemented for SQLite.
func (s *SQLiteStore) AppendAudit(e *protocol.AuditEntry) error {
	return putJSON(s.db, "audit_log", e.ID, e)
}
func (s *SQLiteStore) QueryAudit(filter AuditFilter) []*protocol.AuditEntry {
	all, _ := listJSON[protocol.AuditEntry](s.db, "audit_log")
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
func (s *SQLiteStore) ListWorkersByOrg(orgID string) []*protocol.Worker {
	all := s.ListWorkers()
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
func (s *SQLiteStore) ListTasksByOrg(orgID string) []*protocol.Task {
	all := s.ListTasks()
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
func (s *SQLiteStore) FindWorkersByCapabilityAndOrg(capability, orgID string) []*protocol.Worker {
	all := s.FindWorkersByCapability(capability)
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
func (s *SQLiteStore) AddWebhook(w *protocol.Webhook) error { return putJSON(s.db, "webhooks", w.ID, w) }
func (s *SQLiteStore) GetWebhook(id string) (*protocol.Webhook, error) {
	return getJSON[protocol.Webhook](s.db, "webhooks", id)
}
func (s *SQLiteStore) UpdateWebhook(w *protocol.Webhook) error { return putJSON(s.db, "webhooks", w.ID, w) }
func (s *SQLiteStore) DeleteWebhook(id string) error           { return deleteRow(s.db, "webhooks", id) }
func (s *SQLiteStore) ListWebhooksByOrg(orgID string) []*protocol.Webhook {
	all, _ := listJSON[protocol.Webhook](s.db, "webhooks")
	var result []*protocol.Webhook
	for _, w := range all {
		if w.OrgID == orgID {
			result = append(result, w)
		}
	}
	return result
}
func (s *SQLiteStore) FindWebhooksByEvent(eventType string) []*protocol.Webhook {
	all, _ := listJSON[protocol.Webhook](s.db, "webhooks")
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
func (s *SQLiteStore) AddWebhookDelivery(d *protocol.WebhookDelivery) error {
	return putJSON(s.db, "webhook_deliveries", d.ID, d)
}
func (s *SQLiteStore) UpdateWebhookDelivery(d *protocol.WebhookDelivery) error {
	return putJSON(s.db, "webhook_deliveries", d.ID, d)
}
func (s *SQLiteStore) ListPendingWebhookDeliveries() []*protocol.WebhookDelivery {
	all, _ := listJSON[protocol.WebhookDelivery](s.db, "webhook_deliveries")
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

func (s *SQLiteStore) AddRoleBinding(rb *protocol.RoleBinding) error {
	return putJSON(s.db, "role_bindings", rb.ID, rb)
}
func (s *SQLiteStore) GetRoleBinding(id string) (*protocol.RoleBinding, error) {
	return getJSON[protocol.RoleBinding](s.db, "role_bindings", id)
}
func (s *SQLiteStore) RemoveRoleBinding(id string) error { return deleteRow(s.db, "role_bindings", id) }
func (s *SQLiteStore) ListRoleBindingsByOrg(orgID string) []*protocol.RoleBinding {
	all, _ := listJSON[protocol.RoleBinding](s.db, "role_bindings")
	var result []*protocol.RoleBinding
	for _, rb := range all {
		if rb.OrgID == orgID {
			result = append(result, rb)
		}
	}
	return result
}
func (s *SQLiteStore) FindRoleBinding(orgID, subject string) (*protocol.RoleBinding, error) {
	all, _ := listJSON[protocol.RoleBinding](s.db, "role_bindings")
	for _, rb := range all {
		if rb.OrgID == orgID && rb.Subject == subject {
			return rb, nil
		}
	}
	return nil, ErrNotFound
}

// --- Policies ---

func (s *SQLiteStore) AddPolicy(p *protocol.Policy) error { return putJSON(s.db, "policies", p.ID, p) }
func (s *SQLiteStore) GetPolicy(id string) (*protocol.Policy, error) {
	return getJSON[protocol.Policy](s.db, "policies", id)
}
func (s *SQLiteStore) UpdatePolicy(p *protocol.Policy) error {
	return putJSON(s.db, "policies", p.ID, p)
}
func (s *SQLiteStore) RemovePolicy(id string) error { return deleteRow(s.db, "policies", id) }
func (s *SQLiteStore) ListPoliciesByOrg(orgID string) []*protocol.Policy {
	all, _ := listJSON[protocol.Policy](s.db, "policies")
	var result []*protocol.Policy
	for _, p := range all {
		if p.OrgID == orgID {
			result = append(result, p)
		}
	}
	return result
}
