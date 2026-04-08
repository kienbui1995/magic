package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kienbui1995/magic/core/internal/audit"
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
	"github.com/kienbui1995/magic/core/internal/webhook"
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

	apiKey := os.Getenv("MAGIC_API_KEY")
	if apiKey != "" && len(apiKey) < 32 {
		log.Fatalf("[security] MAGIC_API_KEY must be at least 32 characters (got %d). Generate one with: openssl rand -hex 32", len(apiKey))
	}

	// Store — auto-detect backend from env vars
	var s store.Store
	switch {
	case os.Getenv("MAGIC_POSTGRES_URL") != "":
		pgURL := os.Getenv("MAGIC_POSTGRES_URL")
		// Support pool size config via URL query params.
		// Use ? if no query string exists, & otherwise.
		sep := func() string {
			if strings.Contains(pgURL, "?") {
				return "&"
			}
			return "?"
		}
		if min := os.Getenv("MAGIC_POSTGRES_POOL_MIN"); min != "" {
			pgURL += sep() + "pool_min_conns=" + min
		}
		if max := os.Getenv("MAGIC_POSTGRES_POOL_MAX"); max != "" {
			pgURL += sep() + "pool_max_conns=" + max
		}
		if err := store.RunMigrations(pgURL); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
			os.Exit(1)
		}
		pgStore, err := store.NewPostgreSQLStore(context.Background(), pgURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to PostgreSQL: %v\n", err)
			os.Exit(1)
		}
		s = pgStore
		fmt.Println("  Storage: PostgreSQL")
	case os.Getenv("MAGIC_STORE") != "":
		sqliteStore, err := store.NewSQLiteStore(os.Getenv("MAGIC_STORE"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
			os.Exit(1)
		}
		s = sqliteStore
		fmt.Printf("  Storage: SQLite (%s)\n", os.Getenv("MAGIC_STORE"))
	default:
		s = store.NewMemoryStore()
		fmt.Println("  Storage: in-memory (set MAGIC_STORE=path.db or MAGIC_POSTGRES_URL for persistence)")
	}

	// VectorStore — only available with PostgreSQL backend
	var vs store.VectorStore
	if pgStore, ok := s.(*store.PostgreSQLStore); ok {
		dim := 1536
		if d := os.Getenv("MAGIC_PGVECTOR_DIM"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
				dim = parsed
			}
		}
		vs = store.NewPGVectorStore(pgStore.Pool(), dim)
		fmt.Println("  Semantic search: enabled (pgvector)")
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
	kb := knowledge.New(s, bus, vs)

	// Audit logger — subscribes to events and records them
	auditLogger := audit.New(s, bus)
	auditLogger.SubscribeToEvents()

	// Webhook manager — subscribes to events and delivers webhooks
	wh := webhook.New(s, bus)
	wh.Start()

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
		Webhook:      wh,
	})

	if s.HasAnyWorkerTokens() {
		log.Printf("[security] worker token auth: enabled")
	} else {
		log.Printf("[security] worker token auth: disabled (dev mode — create a token to enable)")
	}

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
		fmt.Println("  POST /api/v1/knowledge/{id}/embedding — Store embedding")
		fmt.Println("  POST /api/v1/knowledge/search/semantic — Semantic search")
		fmt.Println("  GET  /api/v1/metrics           — View stats")
		fmt.Println("  GET  /metrics                  — Prometheus metrics (no auth)")
		fmt.Println("  GET  /health                   — Health check")
		fmt.Println("  POST /api/v1/orgs/{orgID}/webhooks — Register webhook")

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
