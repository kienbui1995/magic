package store

import (
	"database/sql"
	"encoding/json"

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
