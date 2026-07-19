package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func TestMemoryStore_Workers(t *testing.T) {
	s := store.NewMemoryStore()

	w := &protocol.Worker{
		ID:     "worker_001",
		Name:   "TestBot",
		Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{
			{Name: "greeting"},
		},
	}

	if err := s.AddWorker(context.Background(), w); err != nil {
		t.Fatalf("AddWorker: %v", err)
	}

	got, err := s.GetWorker(context.Background(), "worker_001")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if got.Name != "TestBot" {
		t.Errorf("Name: got %q, want TestBot", got.Name)
	}

	workers := s.ListWorkers(context.Background())
	if len(workers) != 1 {
		t.Errorf("ListWorkers: got %d, want 1", len(workers))
	}

	found := s.FindWorkersByCapability(context.Background(), "greeting")
	if len(found) != 1 {
		t.Errorf("FindByCapability: got %d, want 1", len(found))
	}

	found = s.FindWorkersByCapability(context.Background(), "nonexistent")
	if len(found) != 0 {
		t.Errorf("FindByCapability nonexistent: got %d, want 0", len(found))
	}

	if err := s.RemoveWorker(context.Background(), "worker_001"); err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if _, err := s.GetWorker(context.Background(), "worker_001"); err == nil {
		t.Error("GetWorker after remove should fail")
	}
}

func TestMemoryStore_Tasks(t *testing.T) {
	s := store.NewMemoryStore()

	task := &protocol.Task{
		ID:     "task_001",
		Type:   "greeting",
		Status: protocol.TaskPending,
	}

	if err := s.AddTask(context.Background(), task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	got, err := s.GetTask(context.Background(), "task_001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Type != "greeting" {
		t.Errorf("Type: got %q, want greeting", got.Type)
	}

	task.Status = protocol.TaskCompleted
	if err := s.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, _ = s.GetTask(context.Background(), "task_001")
	if got.Status != protocol.TaskCompleted {
		t.Errorf("Status: got %q, want completed", got.Status)
	}
}

func TestMemoryStore_Workflows(t *testing.T) {
	s := store.NewMemoryStore()
	wf := &protocol.Workflow{ID: "wf_001", Name: "Test Workflow", Status: protocol.WorkflowPending,
		Steps: []protocol.WorkflowStep{{ID: "step1", TaskType: "greeting", Status: protocol.StepPending}}}
	if err := s.AddWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("AddWorkflow: %v", err)
	}
	got, err := s.GetWorkflow(context.Background(), "wf_001")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Name != "Test Workflow" {
		t.Errorf("Name: got %q", got.Name)
	}
	wf.Status = protocol.WorkflowRunning
	if err := s.UpdateWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	got, _ = s.GetWorkflow(context.Background(), "wf_001")
	if got.Status != protocol.WorkflowRunning {
		t.Errorf("Status: got %q", got.Status)
	}
	if len(s.ListWorkflows(context.Background())) != 1 {
		t.Errorf("ListWorkflows: got %d", len(s.ListWorkflows(context.Background())))
	}
}

func TestMemoryStore_Teams(t *testing.T) {
	s := store.NewMemoryStore()
	team := &protocol.Team{ID: "team_001", Name: "Marketing", OrgID: "org_magic", DailyBudget: 10.0}
	if err := s.AddTeam(context.Background(), team); err != nil {
		t.Fatalf("AddTeam: %v", err)
	}
	got, err := s.GetTeam(context.Background(), "team_001")
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.Name != "Marketing" {
		t.Errorf("Name: got %q", got.Name)
	}
	team.Workers = []string{"worker_001"}
	if err := s.UpdateTeam(context.Background(), team); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if len(s.ListTeams(context.Background())) != 1 {
		t.Errorf("ListTeams: got %d", len(s.ListTeams(context.Background())))
	}
	if err := s.RemoveTeam(context.Background(), "team_001"); err != nil {
		t.Fatalf("RemoveTeam: %v", err)
	}
	if _, err := s.GetTeam(context.Background(), "team_001"); err == nil {
		t.Error("should fail after remove")
	}
}

func TestMemoryStore_Knowledge(t *testing.T) {
	s := store.NewMemoryStore()

	entry := &protocol.KnowledgeEntry{
		ID:      "kb_001",
		Title:   "API Guidelines",
		Content: "Use REST conventions for all endpoints",
		Tags:    []string{"api", "rest"},
		Scope:   "org",
		ScopeID: "org_magic",
	}

	if err := s.AddKnowledge(context.Background(), entry); err != nil {
		t.Fatalf("AddKnowledge: %v", err)
	}

	got, err := s.GetKnowledge(context.Background(), "kb_001")
	if err != nil {
		t.Fatalf("GetKnowledge: %v", err)
	}
	if got.Title != "API Guidelines" {
		t.Errorf("Title: got %q", got.Title)
	}

	entry.Content = "Updated content"
	if err := s.UpdateKnowledge(context.Background(), entry); err != nil {
		t.Fatalf("UpdateKnowledge: %v", err)
	}

	if len(s.ListKnowledge(context.Background())) != 1 {
		t.Errorf("ListKnowledge: got %d", len(s.ListKnowledge(context.Background())))
	}

	// Search by title substring
	results := s.SearchKnowledge(context.Background(), "API")
	if len(results) != 1 {
		t.Errorf("SearchKnowledge 'API': got %d, want 1", len(results))
	}

	// Search by tag
	results = s.SearchKnowledge(context.Background(), "rest")
	if len(results) != 1 {
		t.Errorf("SearchKnowledge 'rest': got %d, want 1", len(results))
	}

	// Search no match
	results = s.SearchKnowledge(context.Background(), "nonexistent")
	if len(results) != 0 {
		t.Errorf("SearchKnowledge 'nonexistent': got %d, want 0", len(results))
	}

	if err := s.DeleteKnowledge(context.Background(), "kb_001"); err != nil {
		t.Fatalf("DeleteKnowledge: %v", err)
	}
	if _, err := s.GetKnowledge(context.Background(), "kb_001"); err == nil {
		t.Error("should fail after delete")
	}
}

// --- Worker Token Tests ---

func makeTestToken(id, orgID, hash string) *protocol.WorkerToken {
	return &protocol.WorkerToken{
		ID:        id,
		OrgID:     orgID,
		WorkerID:  "",
		TokenHash: hash,
		Name:      "test-token-" + id,
		CreatedAt: time.Now(),
	}
}

func TestAddWorkerToken(t *testing.T) {
	s := store.NewMemoryStore()

	tok := makeTestToken("token_001", "org_acme", "hash_abc")
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	got, err := s.GetWorkerToken(context.Background(), "token_001")
	if err != nil {
		t.Fatalf("GetWorkerToken: %v", err)
	}
	if got.OrgID != "org_acme" {
		t.Errorf("OrgID: got %q, want org_acme", got.OrgID)
	}
	if got.TokenHash != "hash_abc" {
		t.Errorf("TokenHash: got %q, want hash_abc", got.TokenHash)
	}
}

func TestGetWorkerTokenByHash(t *testing.T) {
	s := store.NewMemoryStore()

	tok := makeTestToken("token_002", "org_beta", "hash_xyz")
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	got, err := s.GetWorkerTokenByHash(context.Background(), "hash_xyz")
	if err != nil {
		t.Fatalf("GetWorkerTokenByHash: %v", err)
	}
	if got.ID != "token_002" {
		t.Errorf("ID: got %q, want token_002", got.ID)
	}

	// Non-existent hash returns error
	_, err = s.GetWorkerTokenByHash(context.Background(), "hash_nonexistent")
	if err == nil {
		t.Error("expected error for non-existent hash, got nil")
	}
}

func TestUpdateWorkerToken_CASRejection(t *testing.T) {
	s := store.NewMemoryStore()

	tok := makeTestToken("token_003", "org_acme", "hash_cas")
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	// Simulate two concurrent goroutines both reading the unbound token,
	// then both trying to bind it to different workers.
	var wg sync.WaitGroup
	results := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Read the token (simulating the concurrent read)
			read, err := s.GetWorkerToken(context.Background(), "token_003")
			if err != nil {
				results[idx] = err
				return
			}
			// Each goroutine tries to bind to a different worker
			read.WorkerID = protocol.GenerateID("worker")
			results[idx] = s.UpdateWorkerToken(context.Background(), read)
		}(i)
	}
	wg.Wait()

	// Exactly one should succeed, one should fail with ErrTokenAlreadyBound
	successCount := 0
	failCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		} else {
			failCount++
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d (errors: %v, %v)", successCount, results[0], results[1])
	}
	if failCount != 1 {
		t.Errorf("expected exactly 1 failure, got %d", failCount)
	}
}

func TestHasAnyWorkerTokens(t *testing.T) {
	s := store.NewMemoryStore()

	// Initially false
	if s.HasAnyWorkerTokens(context.Background()) {
		t.Error("HasAnyWorkerTokens should be false on empty store")
	}

	// After adding the first token, becomes true
	tok := makeTestToken("token_has", "org_acme", "hash_has")
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	if !s.HasAnyWorkerTokens(context.Background()) {
		t.Error("HasAnyWorkerTokens should be true after adding a token")
	}
}

func TestListWorkerTokensByOrg(t *testing.T) {
	s := store.NewMemoryStore()

	if err := s.AddWorkerToken(context.Background(), makeTestToken("tok_a1", "org_acme", "h_a1")); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}
	if err := s.AddWorkerToken(context.Background(), makeTestToken("tok_a2", "org_acme", "h_a2")); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}
	if err := s.AddWorkerToken(context.Background(), makeTestToken("tok_b1", "org_beta", "h_b1")); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	acmeTokens := s.ListWorkerTokensByOrg(context.Background(), "org_acme")
	if len(acmeTokens) != 2 {
		t.Errorf("ListWorkerTokensByOrg org_acme: got %d, want 2", len(acmeTokens))
	}

	betaTokens := s.ListWorkerTokensByOrg(context.Background(), "org_beta")
	if len(betaTokens) != 1 {
		t.Errorf("ListWorkerTokensByOrg org_beta: got %d, want 1", len(betaTokens))
	}
}

func TestListWorkerTokensByWorker(t *testing.T) {
	s := store.NewMemoryStore()

	tok := makeTestToken("tok_w1", "org_acme", "h_w1")
	tok.WorkerID = "worker_abc"
	if err := s.AddWorkerToken(context.Background(), tok); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}
	// Unbound token for same org
	if err := s.AddWorkerToken(context.Background(), makeTestToken("tok_w2", "org_acme", "h_w2")); err != nil {
		t.Fatalf("AddWorkerToken: %v", err)
	}

	tokens := s.ListWorkerTokensByWorker(context.Background(), "worker_abc")
	if len(tokens) != 1 {
		t.Errorf("ListWorkerTokensByWorker: got %d, want 1", len(tokens))
	}
	if tokens[0].ID != "tok_w1" {
		t.Errorf("ListWorkerTokensByWorker: got ID %q, want tok_w1", tokens[0].ID)
	}

	// No tokens for unknown worker
	tokens = s.ListWorkerTokensByWorker(context.Background(), "worker_unknown")
	if len(tokens) != 0 {
		t.Errorf("ListWorkerTokensByWorker unknown: got %d, want 0", len(tokens))
	}
}

// --- Audit Log Tests ---

func makeTestAuditEntry(id, orgID, workerID, action, outcome string) *protocol.AuditEntry {
	return &protocol.AuditEntry{
		ID:        id,
		Timestamp: time.Now(),
		OrgID:     orgID,
		WorkerID:  workerID,
		Action:    action,
		Resource:  "worker:" + workerID,
		Outcome:   outcome,
	}
}

func TestAppendAudit(t *testing.T) {
	s := store.NewMemoryStore()

	entry := makeTestAuditEntry("audit_001", "org_acme", "worker_001", "worker.register", "success")
	if err := s.AppendAudit(context.Background(), entry); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	// Query with no filter (empty OrgID matches all)
	results := s.QueryAudit(context.Background(), store.AuditFilter{Limit: 10})
	if len(results) != 1 {
		t.Errorf("QueryAudit after append: got %d, want 1", len(results))
	}
	if results[0].ID != "audit_001" {
		t.Errorf("QueryAudit: got ID %q, want audit_001", results[0].ID)
	}
}

func TestQueryAudit_FilterByOrg(t *testing.T) {
	s := store.NewMemoryStore()

	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a1", "org_acme", "w1", "worker.register", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a2", "org_beta", "w2", "worker.register", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a3", "org_acme", "w3", "task.route", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	// Filter by org_acme
	results := s.QueryAudit(context.Background(), store.AuditFilter{OrgID: "org_acme", Limit: 100})
	if len(results) != 2 {
		t.Errorf("QueryAudit org_acme: got %d, want 2", len(results))
	}
	for _, r := range results {
		if r.OrgID != "org_acme" {
			t.Errorf("QueryAudit org_acme: got entry with OrgID %q", r.OrgID)
		}
	}

	// Filter by org_beta
	results = s.QueryAudit(context.Background(), store.AuditFilter{OrgID: "org_beta", Limit: 100})
	if len(results) != 1 {
		t.Errorf("QueryAudit org_beta: got %d, want 1", len(results))
	}
}

func TestQueryAudit_FilterByWorker(t *testing.T) {
	s := store.NewMemoryStore()

	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a1", "org_acme", "worker_alice", "worker.register", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a2", "org_acme", "worker_bob", "worker.heartbeat", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	if err := s.AppendAudit(context.Background(), makeTestAuditEntry("a3", "org_acme", "worker_alice", "task.complete", "success")); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	results := s.QueryAudit(context.Background(), store.AuditFilter{OrgID: "org_acme", WorkerID: "worker_alice", Limit: 100})
	if len(results) != 2 {
		t.Errorf("QueryAudit by worker_alice: got %d, want 2", len(results))
	}
	for _, r := range results {
		if r.WorkerID != "worker_alice" {
			t.Errorf("QueryAudit: unexpected WorkerID %q", r.WorkerID)
		}
	}
}

func TestQueryAudit_TimeRange(t *testing.T) {
	s := store.NewMemoryStore()

	past := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-30 * time.Minute)
	future := time.Now().Add(1 * time.Hour)

	// Add entry 2 hours ago
	old := makeTestAuditEntry("audit_old", "org_acme", "w1", "worker.register", "success")
	old.Timestamp = past
	if err := s.AppendAudit(context.Background(), old); err != nil {
		t.Fatalf("AppendAudit old: %v", err)
	}

	// Add entry 30 minutes ago
	mid := makeTestAuditEntry("audit_mid", "org_acme", "w2", "worker.heartbeat", "success")
	mid.Timestamp = recent
	if err := s.AppendAudit(context.Background(), mid); err != nil {
		t.Fatalf("AppendAudit mid: %v", err)
	}

	// Query: only entries after 1 hour ago
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	results := s.QueryAudit(context.Background(), store.AuditFilter{StartTime: &oneHourAgo, Limit: 100})
	if len(results) != 1 {
		t.Errorf("QueryAudit StartTime: got %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].ID != "audit_mid" {
		t.Errorf("QueryAudit StartTime: got ID %q, want audit_mid", results[0].ID)
	}

	// Query: only entries before future (should return all)
	results = s.QueryAudit(context.Background(), store.AuditFilter{EndTime: &future, Limit: 100})
	if len(results) != 2 {
		t.Errorf("QueryAudit EndTime future: got %d, want 2", len(results))
	}

	// Query: only entries before 1 hour ago
	results = s.QueryAudit(context.Background(), store.AuditFilter{EndTime: &oneHourAgo, Limit: 100})
	if len(results) != 1 {
		t.Errorf("QueryAudit EndTime past: got %d, want 1", len(results))
	}
	if len(results) > 0 && results[0].ID != "audit_old" {
		t.Errorf("QueryAudit EndTime past: got ID %q, want audit_old", results[0].ID)
	}
}

// --- Org-scoped Worker Query Tests ---

func TestListWorkersByOrg(t *testing.T) {
	s := store.NewMemoryStore()

	// Add workers to two different orgs
	wA1 := &protocol.Worker{ID: "w_a1", Name: "BotA1", OrgID: "org_acme", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "writing"}}}
	wA2 := &protocol.Worker{ID: "w_a2", Name: "BotA2", OrgID: "org_acme", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "coding"}}}
	wB1 := &protocol.Worker{ID: "w_b1", Name: "BotB1", OrgID: "org_beta", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "writing"}}}

	for _, w := range []*protocol.Worker{wA1, wA2, wB1} {
		if err := s.AddWorker(context.Background(), w); err != nil {
			t.Fatalf("AddWorker %s: %v", w.ID, err)
		}
	}

	// org_acme should have 2 workers
	acmeWorkers := s.ListWorkersByOrg(context.Background(), "org_acme")
	if len(acmeWorkers) != 2 {
		t.Errorf("ListWorkersByOrg org_acme: got %d, want 2", len(acmeWorkers))
	}

	// org_beta should have 1 worker
	betaWorkers := s.ListWorkersByOrg(context.Background(), "org_beta")
	if len(betaWorkers) != 1 {
		t.Errorf("ListWorkersByOrg org_beta: got %d, want 1", len(betaWorkers))
	}
	if betaWorkers[0].ID != "w_b1" {
		t.Errorf("ListWorkersByOrg org_beta: got ID %q, want w_b1", betaWorkers[0].ID)
	}

	// Org A workers should NOT appear in org B results
	for _, w := range betaWorkers {
		if w.OrgID != "org_beta" {
			t.Errorf("ListWorkersByOrg org_beta: returned worker with OrgID %q", w.OrgID)
		}
	}

	// Empty orgID returns all (backward compat dev mode)
	allWorkers := s.ListWorkersByOrg(context.Background(), "")
	if len(allWorkers) != 3 {
		t.Errorf("ListWorkersByOrg empty: got %d, want 3", len(allWorkers))
	}
}

func TestFindWorkersByCapabilityAndOrg(t *testing.T) {
	s := store.NewMemoryStore()

	// Add writers in different orgs, plus a coder in org_acme
	wA1 := &protocol.Worker{ID: "w_a1", Name: "WriterA", OrgID: "org_acme", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "writing"}}}
	wA2 := &protocol.Worker{ID: "w_a2", Name: "CoderA", OrgID: "org_acme", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "coding"}}}
	wB1 := &protocol.Worker{ID: "w_b1", Name: "WriterB", OrgID: "org_beta", Status: protocol.StatusActive,
		Capabilities: []protocol.Capability{{Name: "writing"}}}
	// Offline writer in org_acme (should be filtered out)
	wA3 := &protocol.Worker{ID: "w_a3", Name: "OfflineWriter", OrgID: "org_acme", Status: protocol.StatusOffline,
		Capabilities: []protocol.Capability{{Name: "writing"}}}

	for _, w := range []*protocol.Worker{wA1, wA2, wB1, wA3} {
		if err := s.AddWorker(context.Background(), w); err != nil {
			t.Fatalf("AddWorker %s: %v", w.ID, err)
		}
	}

	// Find writing workers in org_acme: should return only wA1 (wA3 is offline)
	result := s.FindWorkersByCapabilityAndOrg(context.Background(), "writing", "org_acme")
	if len(result) != 1 {
		t.Errorf("FindWorkersByCapabilityAndOrg writing org_acme: got %d, want 1", len(result))
	}
	if len(result) > 0 && result[0].ID != "w_a1" {
		t.Errorf("FindWorkersByCapabilityAndOrg writing org_acme: got ID %q, want w_a1", result[0].ID)
	}

	// Find writing workers in org_beta: should return only wB1
	result = s.FindWorkersByCapabilityAndOrg(context.Background(), "writing", "org_beta")
	if len(result) != 1 {
		t.Errorf("FindWorkersByCapabilityAndOrg writing org_beta: got %d, want 1", len(result))
	}
	if len(result) > 0 && result[0].ID != "w_b1" {
		t.Errorf("FindWorkersByCapabilityAndOrg writing org_beta: got ID %q, want w_b1", result[0].ID)
	}

	// Find coding workers in org_beta: should return 0
	result = s.FindWorkersByCapabilityAndOrg(context.Background(), "coding", "org_beta")
	if len(result) != 0 {
		t.Errorf("FindWorkersByCapabilityAndOrg coding org_beta: got %d, want 0", len(result))
	}

	// Empty orgID: find all active writers across orgs
	result = s.FindWorkersByCapabilityAndOrg(context.Background(), "writing", "")
	if len(result) != 2 {
		t.Errorf("FindWorkersByCapabilityAndOrg writing empty org: got %d, want 2", len(result))
	}
}
