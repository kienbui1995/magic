package store

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// maxAuditEntries caps the in-memory audit log to prevent unbounded growth (DoS via memory exhaustion).
const maxAuditEntries = 10_000

// MemoryStore is an in-memory implementation of the Store interface.
// All methods use deep copies to prevent external mutations.
// The ctx parameter is accepted for interface conformance; memory operations
// are CPU-bound and do not meaningfully support cancellation.
type MemoryStore struct {
	mu                sync.RWMutex
	workers           map[string]*protocol.Worker
	tasks             map[string]*protocol.Task
	workflows         map[string]*protocol.Workflow
	teams             map[string]*protocol.Team
	knowledge         map[string]*protocol.KnowledgeEntry
	tokens            map[string]*protocol.WorkerToken
	tokenIndex        map[string]string // hash -> token ID
	auditLog          []*protocol.AuditEntry
	hasTokens         bool
	webhooks          map[string]*protocol.Webhook
	webhookDeliveries map[string]*protocol.WebhookDelivery
	roleBindings      map[string]*protocol.RoleBinding
	policies          map[string]*protocol.Policy
	dlq               []*protocol.DLQEntry
	prompts           []*protocol.PromptTemplate
	memoryTurns       map[string][]*protocol.MemoryTurn // sessionID -> turns
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workers:           make(map[string]*protocol.Worker),
		tasks:             make(map[string]*protocol.Task),
		workflows:         make(map[string]*protocol.Workflow),
		teams:             make(map[string]*protocol.Team),
		knowledge:         make(map[string]*protocol.KnowledgeEntry),
		tokens:            make(map[string]*protocol.WorkerToken),
		tokenIndex:        make(map[string]string),
		webhooks:          make(map[string]*protocol.Webhook),
		webhookDeliveries: make(map[string]*protocol.WebhookDelivery),
		roleBindings:      make(map[string]*protocol.RoleBinding),
		policies:          make(map[string]*protocol.Policy),
		memoryTurns:       make(map[string][]*protocol.MemoryTurn),
	}
}

func (s *MemoryStore) AddWorker(_ context.Context, w *protocol.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[w.ID] = protocol.DeepCopyWorker(w)
	return nil
}

func (s *MemoryStore) GetWorker(_ context.Context, id string) (*protocol.Worker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyWorker(w), nil
}

func (s *MemoryStore) UpdateWorker(_ context.Context, w *protocol.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workers[w.ID]; !ok {
		return ErrNotFound
	}
	s.workers[w.ID] = protocol.DeepCopyWorker(w)
	return nil
}

func (s *MemoryStore) RemoveWorker(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workers[id]; !ok {
		return ErrNotFound
	}
	delete(s.workers, id)
	return nil
}

func (s *MemoryStore) ListWorkers(_ context.Context) []*protocol.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Worker, 0, len(s.workers))
	for _, w := range s.workers {
		result = append(result, protocol.DeepCopyWorker(w))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) FindWorkersByCapability(_ context.Context, capability string) []*protocol.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Worker
	for _, w := range s.workers {
		if w.Status != protocol.StatusActive {
			continue
		}
		for _, cap := range w.Capabilities {
			if cap.Name == capability {
				result = append(result, protocol.DeepCopyWorker(w))
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddTask(_ context.Context, t *protocol.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = protocol.DeepCopyTask(t)
	return nil
}

func (s *MemoryStore) GetTask(_ context.Context, id string) (*protocol.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyTask(t), nil
}

func (s *MemoryStore) UpdateTask(_ context.Context, t *protocol.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return ErrNotFound
	}
	s.tasks[t.ID] = protocol.DeepCopyTask(t)
	return nil
}

// CancelTask atomically transitions the task to cancelled under the write lock,
// preventing the TOCTOU race between a concurrent dispatcher completion and a
// user-initiated cancel.
func (s *MemoryStore) CancelTask(_ context.Context, id string) (*protocol.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
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
	s.tasks[id] = t
	return protocol.DeepCopyTask(t), nil
}

func (s *MemoryStore) ListTasks(_ context.Context) []*protocol.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, protocol.DeepCopyTask(t))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddWorkflow(_ context.Context, w *protocol.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows[w.ID] = protocol.DeepCopyWorkflow(w)
	return nil
}

func (s *MemoryStore) GetWorkflow(_ context.Context, id string) (*protocol.Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workflows[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyWorkflow(w), nil
}

func (s *MemoryStore) UpdateWorkflow(_ context.Context, w *protocol.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workflows[w.ID]; !ok {
		return ErrNotFound
	}
	s.workflows[w.ID] = protocol.DeepCopyWorkflow(w)
	return nil
}

func (s *MemoryStore) ListWorkflows(_ context.Context) []*protocol.Workflow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Workflow, 0, len(s.workflows))
	for _, w := range s.workflows {
		result = append(result, protocol.DeepCopyWorkflow(w))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddTeam(_ context.Context, t *protocol.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teams[t.ID] = protocol.DeepCopyTeam(t)
	return nil
}

func (s *MemoryStore) GetTeam(_ context.Context, id string) (*protocol.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.teams[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyTeam(t), nil
}

func (s *MemoryStore) UpdateTeam(_ context.Context, t *protocol.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[t.ID]; !ok {
		return ErrNotFound
	}
	s.teams[t.ID] = protocol.DeepCopyTeam(t)
	return nil
}

func (s *MemoryStore) RemoveTeam(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[id]; !ok {
		return ErrNotFound
	}
	delete(s.teams, id)
	return nil
}

func (s *MemoryStore) ListTeams(_ context.Context) []*protocol.Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Team, 0, len(s.teams))
	for _, t := range s.teams {
		result = append(result, protocol.DeepCopyTeam(t))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddKnowledge(_ context.Context, k *protocol.KnowledgeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledge[k.ID] = protocol.DeepCopyKnowledge(k)
	return nil
}

func (s *MemoryStore) GetKnowledge(_ context.Context, id string) (*protocol.KnowledgeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.knowledge[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyKnowledge(k), nil
}

func (s *MemoryStore) UpdateKnowledge(_ context.Context, k *protocol.KnowledgeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.knowledge[k.ID]; !ok {
		return ErrNotFound
	}
	s.knowledge[k.ID] = protocol.DeepCopyKnowledge(k)
	return nil
}

func (s *MemoryStore) DeleteKnowledge(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.knowledge[id]; !ok {
		return ErrNotFound
	}
	delete(s.knowledge, id)
	return nil
}

func (s *MemoryStore) ListKnowledge(_ context.Context) []*protocol.KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.KnowledgeEntry, 0, len(s.knowledge))
	for _, k := range s.knowledge {
		result = append(result, protocol.DeepCopyKnowledge(k))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) SearchKnowledge(_ context.Context, query string) []*protocol.KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.KnowledgeEntry
	queryLower := strings.ToLower(query)
	for _, k := range s.knowledge {
		if strings.Contains(strings.ToLower(k.Title), queryLower) ||
			strings.Contains(strings.ToLower(k.Content), queryLower) ||
			containsTag(k.Tags, queryLower) {
			result = append(result, protocol.DeepCopyKnowledge(k))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.ToLower(tag) == query {
			return true
		}
	}
	return false
}

// deepCopyWorkerToken returns a deep copy of a WorkerToken.
func deepCopyWorkerToken(t *protocol.WorkerToken) *protocol.WorkerToken {
	cp := *t
	if t.ExpiresAt != nil {
		exp := *t.ExpiresAt
		cp.ExpiresAt = &exp
	}
	if t.RevokedAt != nil {
		rev := *t.RevokedAt
		cp.RevokedAt = &rev
	}
	return &cp
}

// deepCopyAuditEntry returns a deep copy of an AuditEntry.
func deepCopyAuditEntry(e *protocol.AuditEntry) *protocol.AuditEntry {
	cp := *e
	if e.Detail != nil {
		cp.Detail = make(map[string]any, len(e.Detail))
		for k, v := range e.Detail {
			cp.Detail[k] = v
		}
	}
	return &cp
}

// Worker tokens

func (s *MemoryStore) AddWorkerToken(_ context.Context, t *protocol.WorkerToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t.ID] = deepCopyWorkerToken(t)
	s.tokenIndex[t.TokenHash] = t.ID
	s.hasTokens = true
	return nil
}

func (s *MemoryStore) GetWorkerToken(_ context.Context, id string) (*protocol.WorkerToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[id]
	if !ok {
		return nil, ErrNotFound
	}
	return deepCopyWorkerToken(t), nil
}

func (s *MemoryStore) GetWorkerTokenByHash(_ context.Context, hash string) (*protocol.WorkerToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.tokenIndex[hash]
	if !ok {
		return nil, ErrNotFound
	}
	t, ok := s.tokens[id]
	if !ok {
		return nil, ErrNotFound
	}
	return deepCopyWorkerToken(t), nil
}

func (s *MemoryStore) UpdateWorkerToken(_ context.Context, t *protocol.WorkerToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.tokens[t.ID]
	if !ok {
		return ErrNotFound
	}
	// CAS semantics: if token was unbound when we read it but is now bound to a different worker, reject.
	if existing.WorkerID != "" && t.WorkerID != existing.WorkerID {
		return ErrTokenAlreadyBound
	}
	s.tokens[t.ID] = deepCopyWorkerToken(t)
	return nil
}

func (s *MemoryStore) ListWorkerTokensByOrg(_ context.Context, orgID string) []*protocol.WorkerToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.WorkerToken
	for _, t := range s.tokens {
		if t.OrgID == orgID {
			result = append(result, deepCopyWorkerToken(t))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) ListWorkerTokensByWorker(_ context.Context, workerID string) []*protocol.WorkerToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.WorkerToken
	for _, t := range s.tokens {
		if t.WorkerID == workerID {
			result = append(result, deepCopyWorkerToken(t))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) HasAnyWorkerTokens(_ context.Context) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasTokens
}

// Audit log

func (s *MemoryStore) AppendAudit(_ context.Context, e *protocol.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditLog = append(s.auditLog, deepCopyAuditEntry(e))
	if len(s.auditLog) > maxAuditEntries {
		// Drop oldest entries, keep newest maxAuditEntries.
		s.auditLog = s.auditLog[len(s.auditLog)-maxAuditEntries:]
	}
	return nil
}

func (s *MemoryStore) QueryAudit(_ context.Context, filter AuditFilter) []*protocol.AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var filtered []*protocol.AuditEntry
	for _, e := range s.auditLog {
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
		filtered = append(filtered, deepCopyAuditEntry(e))
	}

	offset := filter.Offset
	if offset >= len(filtered) {
		return nil
	}
	filtered = filtered[offset:]
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

// Org-scoped queries

func (s *MemoryStore) ListWorkersByOrg(_ context.Context, orgID string) []*protocol.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Worker
	for _, w := range s.workers {
		// Empty orgID matches all workers (backward compat for dev mode).
		if orgID == "" || w.OrgID == orgID {
			result = append(result, protocol.DeepCopyWorker(w))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) ListTasksByOrg(_ context.Context, orgID string) []*protocol.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Task
	for _, t := range s.tasks {
		if orgID == "" || t.Context.OrgID == orgID {
			result = append(result, protocol.DeepCopyTask(t))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) FindWorkersByCapabilityAndOrg(_ context.Context, capability, orgID string) []*protocol.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Worker
	for _, w := range s.workers {
		if w.Status != protocol.StatusActive {
			continue
		}
		if orgID != "" && w.OrgID != orgID {
			continue
		}
		for _, cap := range w.Capabilities {
			if cap.Name == capability {
				result = append(result, protocol.DeepCopyWorker(w))
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// --- Webhooks ---

func (s *MemoryStore) AddWebhook(_ context.Context, w *protocol.Webhook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooks[w.ID] = protocol.DeepCopyWebhook(w)
	return nil
}

func (s *MemoryStore) GetWebhook(_ context.Context, id string) (*protocol.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.webhooks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyWebhook(w), nil
}

func (s *MemoryStore) UpdateWebhook(_ context.Context, w *protocol.Webhook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[w.ID]; !ok {
		return ErrNotFound
	}
	s.webhooks[w.ID] = protocol.DeepCopyWebhook(w)
	return nil
}

func (s *MemoryStore) DeleteWebhook(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[id]; !ok {
		return ErrNotFound
	}
	delete(s.webhooks, id)
	return nil
}

func (s *MemoryStore) ListWebhooksByOrg(_ context.Context, orgID string) []*protocol.Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Webhook
	for _, w := range s.webhooks {
		if w.OrgID == orgID {
			result = append(result, protocol.DeepCopyWebhook(w))
		}
	}
	return result
}

func (s *MemoryStore) FindWebhooksByEvent(_ context.Context, eventType string) []*protocol.Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Webhook
	for _, w := range s.webhooks {
		if !w.Active {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				result = append(result, protocol.DeepCopyWebhook(w))
				break
			}
		}
	}
	return result
}

// --- Webhook Deliveries ---

func (s *MemoryStore) AddWebhookDelivery(_ context.Context, d *protocol.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *d
	s.webhookDeliveries[d.ID] = &cp
	return nil
}

func (s *MemoryStore) UpdateWebhookDelivery(_ context.Context, d *protocol.WebhookDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhookDeliveries[d.ID]; !ok {
		return ErrNotFound
	}
	cp := *d
	s.webhookDeliveries[d.ID] = &cp
	return nil
}

func (s *MemoryStore) ListPendingWebhookDeliveries(_ context.Context) []*protocol.WebhookDelivery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []*protocol.WebhookDelivery
	for _, d := range s.webhookDeliveries {
		if d.Status == protocol.DeliveryPending || d.Status == protocol.DeliveryFailed {
			if d.NextRetry == nil || d.NextRetry.Before(now) {
				cp := *d
				result = append(result, &cp)
			}
		}
	}
	return result
}

// --- Role Bindings ---

func (s *MemoryStore) AddRoleBinding(_ context.Context, rb *protocol.RoleBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roleBindings[rb.ID] = protocol.DeepCopyRoleBinding(rb)
	return nil
}

func (s *MemoryStore) GetRoleBinding(_ context.Context, id string) (*protocol.RoleBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rb, ok := s.roleBindings[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyRoleBinding(rb), nil
}

func (s *MemoryStore) RemoveRoleBinding(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roleBindings[id]; !ok {
		return ErrNotFound
	}
	delete(s.roleBindings, id)
	return nil
}

func (s *MemoryStore) ListRoleBindingsByOrg(_ context.Context, orgID string) []*protocol.RoleBinding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.RoleBinding
	for _, rb := range s.roleBindings {
		if rb.OrgID == orgID {
			result = append(result, protocol.DeepCopyRoleBinding(rb))
		}
	}
	return result
}

func (s *MemoryStore) FindRoleBinding(_ context.Context, orgID, subject string) (*protocol.RoleBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rb := range s.roleBindings {
		if rb.OrgID == orgID && rb.Subject == subject {
			return protocol.DeepCopyRoleBinding(rb), nil
		}
	}
	return nil, ErrNotFound
}

// --- Policies ---

func (s *MemoryStore) AddPolicy(_ context.Context, p *protocol.Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[p.ID] = protocol.DeepCopyPolicy(p)
	return nil
}

func (s *MemoryStore) GetPolicy(_ context.Context, id string) (*protocol.Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.policies[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyPolicy(p), nil
}

func (s *MemoryStore) UpdatePolicy(_ context.Context, p *protocol.Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.policies[p.ID]; !ok {
		return ErrNotFound
	}
	s.policies[p.ID] = protocol.DeepCopyPolicy(p)
	return nil
}

func (s *MemoryStore) RemovePolicy(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.policies[id]; !ok {
		return ErrNotFound
	}
	delete(s.policies, id)
	return nil
}

func (s *MemoryStore) ListPoliciesByOrg(_ context.Context, orgID string) []*protocol.Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*protocol.Policy
	for _, p := range s.policies {
		if p.OrgID == orgID {
			result = append(result, protocol.DeepCopyPolicy(p))
		}
	}
	return result
}

const maxDLQEntries = 10_000

func (s *MemoryStore) AddDLQEntry(_ context.Context, e *protocol.DLQEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dlq = append(s.dlq, e)
	if len(s.dlq) > maxDLQEntries {
		s.dlq = s.dlq[len(s.dlq)-maxDLQEntries:]
	}
	return nil
}

func (s *MemoryStore) ListDLQ(_ context.Context) []*protocol.DLQEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.DLQEntry, len(s.dlq))
	copy(result, s.dlq)
	return result
}

func (s *MemoryStore) AddPrompt(_ context.Context, p *protocol.PromptTemplate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, p)
	return nil
}

func (s *MemoryStore) ListPrompts(_ context.Context) []*protocol.PromptTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.PromptTemplate, len(s.prompts))
	copy(result, s.prompts)
	return result
}

func (s *MemoryStore) AddMemoryTurn(_ context.Context, sessionID string, turn *protocol.MemoryTurn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryTurns[sessionID] = append(s.memoryTurns[sessionID], turn)
	return nil
}

func (s *MemoryStore) GetMemoryTurns(_ context.Context, sessionID string) []*protocol.MemoryTurn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	turns := s.memoryTurns[sessionID]
	result := make([]*protocol.MemoryTurn, len(turns))
	copy(result, turns)
	return result
}
