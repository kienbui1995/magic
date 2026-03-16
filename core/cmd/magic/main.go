package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/kienbm/magic-claw/core/internal/events"
	"github.com/kienbm/magic-claw/core/internal/gateway"
	"github.com/kienbm/magic-claw/core/internal/monitor"
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

	s := store.NewMemoryStore()
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stdout)
	mon.Start()
	reg.StartHealthCheck(30_000_000_000) // 30s

	gw := gateway.New(reg, rt, s, bus, mon)

	fmt.Printf("MagiC server starting on :%s\n", port)
	fmt.Println("  POST /api/v1/workers/register  — Register a worker")
	fmt.Println("  GET  /api/v1/workers           — List workers")
	fmt.Println("  POST /api/v1/tasks             — Submit a task")
	fmt.Println("  GET  /api/v1/metrics           — View stats")
	fmt.Println("  GET  /health                   — Health check")

	if err := http.ListenAndServe(":"+port, gw.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
