package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/events"
)

type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	EventType string         `json:"event_type"`
	Source    string         `json:"source"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func writeLogEntry(w io.Writer, mu *sync.Mutex, e events.Event) {
	level := "info"
	switch e.Severity {
	case "warn":
		level = "warn"
	case "error", "critical":
		level = "error"
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		EventType: e.Type,
		Source:    e.Source,
		Payload:   e.Payload,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	mu.Lock()
	fmt.Fprintf(w, "%s\n", data)
	mu.Unlock()
}
