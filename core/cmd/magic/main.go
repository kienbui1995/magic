package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/evaluator"
	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/gateway"
	"github.com/kienbm/magic-claw/core/internal/knowledge"
	"github.com/kienbm/magic-claw/core/internal/monitor"
	"github.com/kienbm/magic-claw/core/internal/orchestrator"
	"github.com/kienbm/magic-claw/core/internal/orgmgr"
	"github.com/kienbm/magic-claw/core/internal/registry"
	"github.com/kienbm/magic-claw/core/internal/router"
	"github.com/kienbm/magic-claw/core/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("MagiC — Where AI becomes a Company")
		fmt.Println("Usage: magic <command>")
		fmt.Println("Commands: serve")
		os.Exit(0)
	}

	switch os.Args[1] {
	case "serve":
		runServer()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runServer() {
	port := os.Getenv("MAGIC_PORT")
	if port == "" {
		port = "8080"
	}

	// Core
	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stdout)
	mon.Start()
	reg.StartHealthCheck(30_000_000_000) // 30s

	// Tier 2
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	orch := orchestrator.New(s, rt, bus)
	mgr := orgmgr.New(s, bus)
	kb := knowledge.New(s, bus)

	gw := gateway.New(reg, rt, s, bus, mon, cc, ev, orch, mgr, kb)

	fmt.Printf("MagiC server starting on :%s\n", port)
	fmt.Println("  POST /api/v1/workers/register  — Register a worker")
	fmt.Println("  GET  /api/v1/workers           — List workers")
	fmt.Println("  POST /api/v1/tasks             — Submit a task")
	fmt.Println("  POST /api/v1/workflows         — Submit a workflow")
	fmt.Println("  GET  /api/v1/workflows         — List workflows")
	fmt.Println("  POST /api/v1/teams             — Create a team")
	fmt.Println("  GET  /api/v1/teams             — List teams")
	fmt.Println("  GET  /api/v1/costs             — Cost report")
	fmt.Println("  POST /api/v1/knowledge         — Add knowledge entry")
	fmt.Println("  GET  /api/v1/knowledge         — Search/list knowledge")
	fmt.Println("  GET  /api/v1/metrics           — View stats")
	fmt.Println("  GET  /health                   — Health check")

	if err := http.ListenAndServe(":"+port, gw.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
