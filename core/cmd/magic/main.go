package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/gateway"
	"github.com/kienbui1995/magic/core/internal/knowledge"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/orchestrator"
	"github.com/kienbui1995/magic/core/internal/orgmgr"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/store"
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

	// Store
	var s store.Store
	storePath := os.Getenv("MAGIC_STORE")
	if storePath != "" {
		sqliteStore, err := store.NewSQLiteStore(storePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
			os.Exit(1)
		}
		s = sqliteStore
	} else {
		s = store.NewMemoryStore()
	}

	// Core
	bus := events.NewBus()
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stdout)
	mon.Start()
	stopHealthCheck := reg.StartHealthCheck(30_000_000_000)

	// Tier 2
	cc := costctrl.New(s, bus)
	ev := evaluator.New(bus)
	disp := dispatcher.New(s, bus, cc, ev)
	orch := orchestrator.New(s, rt, bus, disp)
	mgr := orgmgr.New(s, bus)
	kb := knowledge.New(s, bus)

	gw := gateway.New(gateway.Deps{
		Registry:     reg,
		Router:       rt,
		Store:        s,
		Bus:          bus,
		Monitor:      mon,
		CostCtrl:     cc,
		Evaluator:    ev,
		Orchestrator: orch,
		OrgMgr:       mgr,
		Knowledge:    kb,
		Dispatcher:   disp,
	})

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
		if os.Getenv("MAGIC_STORE") == "" {
			fmt.Println("  Storage: in-memory (set MAGIC_STORE=path.db for persistence)")
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

	stopHealthCheck()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
	}
	fmt.Println("Server stopped.")
}
