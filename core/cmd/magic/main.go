package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kienbm/magic-claw/core/internal/costctrl"
	"github.com/kienbm/magic-claw/core/internal/dispatcher"
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
	reg.StartHealthCheck(30_000_000_000)

	// Tier 2
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	disp := dispatcher.New(s, bus, cc)
	orch := orchestrator.New(s, rt, bus, disp)
	mgr := orgmgr.New(s, bus)
	kb := knowledge.New(s, bus)

	gw := gateway.New(reg, rt, s, bus, mon, cc, ev, orch, mgr, kb, disp)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("MagiC server starting on :%s\n", port)
		if os.Getenv("MAGIC_API_KEY") != "" {
			fmt.Println("  Authentication: enabled (MAGIC_API_KEY)")
		} else {
			fmt.Println("  Authentication: disabled (set MAGIC_API_KEY to enable)")
		}
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

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-done
	fmt.Println("\nShutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
	}
	fmt.Println("Server stopped.")
}
