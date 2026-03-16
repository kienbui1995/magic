package store

import (
	"sort"
	"strings"
	"sync"

	"github.com/kienbui1995/magic/core/internal/protocol"
)

// MemoryStore is an in-memory implementation of the Store interface.
// All methods use deep copies to prevent external mutations.
type MemoryStore struct {
	mu        sync.RWMutex
	workers   map[string]*protocol.Worker
	tasks     map[string]*protocol.Task
	workflows map[string]*protocol.Workflow
	teams     map[string]*protocol.Team
	knowledge map[string]*protocol.KnowledgeEntry
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workers:   make(map[string]*protocol.Worker),
		tasks:     make(map[string]*protocol.Task),
		workflows: make(map[string]*protocol.Workflow),
		teams:     make(map[string]*protocol.Team),
		knowledge: make(map[string]*protocol.KnowledgeEntry),
	}
}

func (s *MemoryStore) AddWorker(w *protocol.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[w.ID] = protocol.DeepCopyWorker(w)
	return nil
}

func (s *MemoryStore) GetWorker(id string) (*protocol.Worker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyWorker(w), nil
}

func (s *MemoryStore) UpdateWorker(w *protocol.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workers[w.ID]; !ok {
		return ErrNotFound
	}
	s.workers[w.ID] = protocol.DeepCopyWorker(w)
	return nil
}

func (s *MemoryStore) RemoveWorker(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workers[id]; !ok {
		return ErrNotFound
	}
	delete(s.workers, id)
	return nil
}

func (s *MemoryStore) ListWorkers() []*protocol.Worker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Worker, 0, len(s.workers))
	for _, w := range s.workers {
		result = append(result, protocol.DeepCopyWorker(w))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) FindWorkersByCapability(capability string) []*protocol.Worker {
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

func (s *MemoryStore) AddTask(t *protocol.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = protocol.DeepCopyTask(t)
	return nil
}

func (s *MemoryStore) GetTask(id string) (*protocol.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyTask(t), nil
}

func (s *MemoryStore) UpdateTask(t *protocol.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return ErrNotFound
	}
	s.tasks[t.ID] = protocol.DeepCopyTask(t)
	return nil
}

func (s *MemoryStore) ListTasks() []*protocol.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, protocol.DeepCopyTask(t))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddWorkflow(w *protocol.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows[w.ID] = protocol.DeepCopyWorkflow(w)
	return nil
}

func (s *MemoryStore) GetWorkflow(id string) (*protocol.Workflow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workflows[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyWorkflow(w), nil
}

func (s *MemoryStore) UpdateWorkflow(w *protocol.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workflows[w.ID]; !ok {
		return ErrNotFound
	}
	s.workflows[w.ID] = protocol.DeepCopyWorkflow(w)
	return nil
}

func (s *MemoryStore) ListWorkflows() []*protocol.Workflow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Workflow, 0, len(s.workflows))
	for _, w := range s.workflows {
		result = append(result, protocol.DeepCopyWorkflow(w))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddTeam(t *protocol.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teams[t.ID] = protocol.DeepCopyTeam(t)
	return nil
}

func (s *MemoryStore) GetTeam(id string) (*protocol.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.teams[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyTeam(t), nil
}

func (s *MemoryStore) UpdateTeam(t *protocol.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[t.ID]; !ok {
		return ErrNotFound
	}
	s.teams[t.ID] = protocol.DeepCopyTeam(t)
	return nil
}

func (s *MemoryStore) RemoveTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[id]; !ok {
		return ErrNotFound
	}
	delete(s.teams, id)
	return nil
}

func (s *MemoryStore) ListTeams() []*protocol.Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.Team, 0, len(s.teams))
	for _, t := range s.teams {
		result = append(result, protocol.DeepCopyTeam(t))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) AddKnowledge(k *protocol.KnowledgeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledge[k.ID] = protocol.DeepCopyKnowledge(k)
	return nil
}

func (s *MemoryStore) GetKnowledge(id string) (*protocol.KnowledgeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.knowledge[id]
	if !ok {
		return nil, ErrNotFound
	}
	return protocol.DeepCopyKnowledge(k), nil
}

func (s *MemoryStore) UpdateKnowledge(k *protocol.KnowledgeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.knowledge[k.ID]; !ok {
		return ErrNotFound
	}
	s.knowledge[k.ID] = protocol.DeepCopyKnowledge(k)
	return nil
}

func (s *MemoryStore) DeleteKnowledge(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.knowledge[id]; !ok {
		return ErrNotFound
	}
	delete(s.knowledge, id)
	return nil
}

func (s *MemoryStore) ListKnowledge() []*protocol.KnowledgeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*protocol.KnowledgeEntry, 0, len(s.knowledge))
	for _, k := range s.knowledge {
		result = append(result, protocol.DeepCopyKnowledge(k))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *MemoryStore) SearchKnowledge(query string) []*protocol.KnowledgeEntry {
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
