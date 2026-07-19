package dispatcher_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/store"
)

func TestDispatchStream_ProxiesSSE(t *testing.T) {
	// Fake streaming worker
	fakeWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"chunk\":\"hello \",\"done\":false}\n\n")
		fmt.Fprintf(w, "data: {\"chunk\":\"world\",\"done\":false}\n\n")
		fmt.Fprintf(w, "data: {\"task_id\":\"t-1\",\"done\":true}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer fakeWorker.Close()

	s := store.NewMemoryStore()
	bus := events.NewBus()
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	d := dispatcher.New(s, bus, cc, ev)

	task := &protocol.Task{
		ID:     "t-1",
		Type:   "chat",
		Status: protocol.TaskPending,
		Input:  []byte(`{"message":"hi"}`),
	}
	if err := s.AddTask(context.Background(), task); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	worker := &protocol.Worker{
		ID:       "w-1",
		Endpoint: protocol.Endpoint{URL: fakeWorker.URL},
	}

	rr := httptest.NewRecorder()
	err := d.DispatchStream(context.Background(), task, worker, rr)
	if err != nil {
		t.Fatalf("DispatchStream: %v", err)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "hello") {
		t.Errorf("expected SSE body to contain 'hello', got: %s", body)
	}
	if !strings.Contains(body, `"done":true`) {
		t.Errorf("expected final done event, got: %s", body)
	}
}
