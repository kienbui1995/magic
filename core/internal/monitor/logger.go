package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
)

// LogEntry represents a structured log record.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	EventType string         `json:"event_type"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// LogSink defines the interface for logging plugins.
// Implementations receive structured log entries and write them to a destination.
type LogSink interface {
	Name() string
	Write(entry LogEntry)
}

// --- Built-in sink: JSONSink ---

// JSONSink writes JSON-encoded log entries to an io.Writer.
type JSONSink struct {
	w  io.Writer
	mu sync.Mutex
}

// NewJSONSink creates a JSONSink writing to w.
func NewJSONSink(w io.Writer) *JSONSink {
	return &JSONSink{w: w}
}

func (s *JSONSink) Name() string { return "json" }

func (s *JSONSink) Write(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	s.mu.Lock()
	fmt.Fprintf(s.w, "%s\n", data)
	s.mu.Unlock()
}

// toLogEntry converts an event to a LogEntry.
func toLogEntry(e events.Event) LogEntry {
	level := "info"
	switch e.Severity {
	case "warn":
		level = "warn"
	case "error", "critical":
		level = "error"
	}
	return LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		EventType: e.Type,
		Source:    e.Source,
		Payload:   e.Payload,
	}
}
