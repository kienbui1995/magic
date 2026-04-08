package knowledge

import (
	"fmt"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

type Hub struct {
	store   store.Store
	bus     *events.Bus
	vectors VectorStore // nil if semantic search not configured
}

func New(s store.Store, bus *events.Bus, vs VectorStore) *Hub {
	return &Hub{store: s, bus: bus, vectors: vs}
}

func (h *Hub) Add(title, content string, tags []string, scope, scopeID, createdBy string) (*protocol.KnowledgeEntry, error) {
	entry := &protocol.KnowledgeEntry{
		ID:        protocol.GenerateID("kb"),
		Title:     title,
		Content:   content,
		Tags:      tags,
		Scope:     scope,
		ScopeID:   scopeID,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.store.AddKnowledge(entry); err != nil {
		return nil, err
	}

	h.bus.Publish(events.Event{
		Type:   "knowledge.added",
		Source: "knowledge",
		Payload: map[string]any{
			"entry_id": entry.ID,
			"title":    entry.Title,
			"scope":    entry.Scope,
		},
	})

	return entry, nil
}

func (h *Hub) Get(id string) (*protocol.KnowledgeEntry, error) {
	return h.store.GetKnowledge(id)
}

func (h *Hub) Update(id, title, content string, tags []string) error {
	entry, err := h.store.GetKnowledge(id)
	if err != nil {
		return err
	}
	entry.Title = title
	entry.Content = content
	entry.Tags = tags
	entry.UpdatedAt = time.Now()

	if err := h.store.UpdateKnowledge(entry); err != nil {
		return err
	}

	h.bus.Publish(events.Event{
		Type:   "knowledge.updated",
		Source: "knowledge",
		Payload: map[string]any{"entry_id": id, "title": title},
	})

	return nil
}

func (h *Hub) Delete(id string) error {
	if err := h.store.DeleteKnowledge(id); err != nil {
		return err
	}

	h.bus.Publish(events.Event{
		Type:   "knowledge.deleted",
		Source: "knowledge",
		Payload: map[string]any{"entry_id": id},
	})

	return nil
}

func (h *Hub) Search(query string) []*protocol.KnowledgeEntry {
	return h.store.SearchKnowledge(query)
}

func (h *Hub) List() []*protocol.KnowledgeEntry {
	return h.store.ListKnowledge()
}

// SemanticSearch returns knowledge entries ranked by cosine similarity to queryVector.
// Returns an error if no VectorStore is configured (nil).
func (h *Hub) SemanticSearch(queryVector []float32, topK int) ([]SearchResult, error) {
	if h.vectors == nil {
		return nil, fmt.Errorf("semantic search requires PostgreSQL backend with pgvector")
	}
	return h.vectors.Search(queryVector, topK)
}

// AddEmbedding stores a vector embedding for an existing knowledge entry.
func (h *Hub) AddEmbedding(id string, vector []float32, meta map[string]any) error {
	if h.vectors == nil {
		return fmt.Errorf("semantic search requires PostgreSQL backend with pgvector")
	}
	return h.vectors.Upsert(id, vector, meta)
}
