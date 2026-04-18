package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kienbui1995/magic/core/internal/audit"
	"github.com/kienbui1995/magic/core/internal/auth"
	"github.com/kienbui1995/magic/core/internal/config"
	"github.com/kienbui1995/magic/core/internal/costctrl"
	"github.com/kienbui1995/magic/core/internal/dispatcher"
	"github.com/kienbui1995/magic/core/internal/evaluator"
	"github.com/kienbui1995/magic/core/internal/events"
	"github.com/kienbui1995/magic/core/internal/gateway"
	"github.com/kienbui1995/magic/core/internal/knowledge"
	"github.com/kienbui1995/magic/core/internal/llm"
	"github.com/kienbui1995/magic/core/internal/memory"
	"github.com/kienbui1995/magic/core/internal/monitor"
	"github.com/kienbui1995/magic/core/internal/orchestrator"
	"github.com/kienbui1995/magic/core/internal/orgmgr"
	"github.com/kienbui1995/magic/core/internal/prompt"
	"github.com/kienbui1995/magic/core/internal/registry"
	"github.com/kienbui1995/magic/core/internal/router"
	"github.com/kienbui1995/magic/core/internal/secrets"
	"github.com/kienbui1995/magic/core/internal/store"
	"github.com/kienbui1995/magic/core/internal/policy"
	"github.com/kienbui1995/magic/core/internal/rbac"
	"github.com/kienbui1995/magic/core/internal/tracing"
	"github.com/kienbui1995/magic/core/internal/webhook"
)

func main() {
	// Support --help / -h at top level.
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" || os.Args[1] == "help" {
		printUsage(os.Stdout)
		if len(os.Args) < 2 {
			os.Exit(0)
		}
		return
	}

	switch os.Args[1] {
	case "serve":
		runServer()
	case "workers":
		runCLI("GET", "/api/v1/workers", nil)
	case "tasks":
		runCLI("GET", "/api/v1/tasks", nil)
	case "submit":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: magic submit <task-type> [json-input]")
			os.Exit(1)
		}
		input := "{}"
		if len(os.Args) >= 4 {
			input = os.Args[3]
		} else {
			// Try reading from stdin if not a terminal
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				b, err := io.ReadAll(os.Stdin)
				if err == nil && len(b) > 0 {
					input = string(b)
				}
			}
		}
		body := fmt.Sprintf(`{"type":%q,"input":%s}`, os.Args[2], input)
		runCLI("POST", "/api/v1/tasks", []byte(body))
	case "status":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: magic status <task-id>")
			os.Exit(1)
		}
		runCLI("GET", "/api/v1/tasks/"+os.Args[2], nil)
	case "completion":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: magic completion <bash|zsh|fish>")
			os.Exit(1)
		}
		if err := printCompletion(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("magic v0.4.0")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		fmt.Fprintln(os.Stderr, "Run 'magic --help' for usage.")
		os.Exit(1)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "MagiC — Where AI becomes a Company")
	fmt.Fprintln(w, "Usage: magic <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  serve                Start the MagiC server")
	fmt.Fprintln(w, "  workers              List registered workers")
	fmt.Fprintln(w, "  tasks                List tasks")
	fmt.Fprintln(w, "  submit <type>        Submit a task (reads JSON input from stdin)")
	fmt.Fprintln(w, "  status <task-id>     Get task status")
	fmt.Fprintln(w, "  completion <shell>   Emit shell completion script (bash|zsh|fish)")
	fmt.Fprintln(w, "  version              Print version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags (serve):")
	fmt.Fprintln(w, "  --config <path>      Path to YAML config (default: ./magic.yaml if present)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Config precedence (highest first): CLI flag > env var > config file > built-in default")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  MAGIC_URL            Server URL for client commands (default: http://localhost:8080)")
	fmt.Fprintln(w, "  MAGIC_API_KEY        API key for authentication")
	fmt.Fprintln(w, "  MAGIC_POSTGRES_URL   PostgreSQL connection string (enables Postgres backend)")
	fmt.Fprintln(w, "  MAGIC_STORE          SQLite path (enables SQLite backend)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  magic serve --config ./magic.yaml")
	fmt.Fprintln(w, "  magic submit summarize '{\"text\":\"hello\"}'")
	fmt.Fprintln(w, "  magic completion bash > /etc/bash_completion.d/magic")
}

// printCompletion writes a shell completion script for the requested shell.
// Scripts are hardcoded (no runtime reflection) for portability — completing
// subcommand names is enough for the overwhelming majority of CLI use.
func printCompletion(w io.Writer, shell string) error {
	switch shell {
	case "bash":
		_, err := fmt.Fprint(w, bashCompletion)
		return err
	case "zsh":
		_, err := fmt.Fprint(w, zshCompletion)
		return err
	case "fish":
		_, err := fmt.Fprint(w, fishCompletion)
		return err
	default:
		return fmt.Errorf("unsupported shell %q (expected: bash, zsh, fish)", shell)
	}
}

const bashCompletion = `# bash completion for magic
# Install: magic completion bash > /etc/bash_completion.d/magic
# Or (user-local): magic completion bash > ~/.local/share/bash-completion/completions/magic
_magic_complete() {
    local cur prev subcmds
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    subcmds="serve workers tasks submit status completion version help"

    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "${subcmds} --help" -- "${cur}") )
        return 0
    fi

    case "${COMP_WORDS[1]}" in
        serve)
            if [ "${prev}" = "--config" ]; then
                COMPREPLY=( $(compgen -f -- "${cur}") )
            else
                COMPREPLY=( $(compgen -W "--config" -- "${cur}") )
            fi
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            ;;
    esac
    return 0
}
complete -F _magic_complete magic
`

const zshCompletion = `#compdef magic
# zsh completion for magic
# Install: magic completion zsh > "${fpath[1]}/_magic"
# Then restart your shell (or run: autoload -U compinit && compinit)
_magic() {
    local -a subcmds
    subcmds=(
        'serve:Start the MagiC server'
        'workers:List registered workers'
        'tasks:List tasks'
        'submit:Submit a task'
        'status:Get task status'
        'completion:Emit shell completion script'
        'version:Print version'
        'help:Show help'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' subcmds
        return
    fi

    case "${words[2]}" in
        serve)
            _arguments '--config[Path to YAML config]:config file:_files -g "*.yaml"'
            ;;
        completion)
            _values 'shell' bash zsh fish
            ;;
    esac
}
compdef _magic magic
`

const fishCompletion = `# fish completion for magic
# Install: magic completion fish > ~/.config/fish/completions/magic.fish
complete -c magic -f

complete -c magic -n '__fish_use_subcommand' -a serve      -d 'Start the MagiC server'
complete -c magic -n '__fish_use_subcommand' -a workers    -d 'List registered workers'
complete -c magic -n '__fish_use_subcommand' -a tasks      -d 'List tasks'
complete -c magic -n '__fish_use_subcommand' -a submit     -d 'Submit a task'
complete -c magic -n '__fish_use_subcommand' -a status     -d 'Get task status'
complete -c magic -n '__fish_use_subcommand' -a completion -d 'Emit shell completion script'
complete -c magic -n '__fish_use_subcommand' -a version    -d 'Print version'
complete -c magic -n '__fish_use_subcommand' -a help       -d 'Show help'

complete -c magic -n '__fish_seen_subcommand_from serve'      -l config -r -d 'Path to YAML config'
complete -c magic -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`

func serverURL() string {
	if u := os.Getenv("MAGIC_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8080"
}

func runCLI(method, path string, body []byte) {
	url := serverURL() + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("MAGIC_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", serverURL(), err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	out, _ := io.ReadAll(resp.Body)
	// Pretty-print JSON
	var buf bytes.Buffer
	if json.Indent(&buf, out, "", "  ") == nil {
		fmt.Println(buf.String())
	} else {
		fmt.Println(string(out))
	}
	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}

func runServer() {
	// Load config: YAML file (optional) + env var overrides.
	// Precedence: CLI flag > env var > config file > built-in default.
	configPath := ""
	for i, arg := range os.Args {
		if (arg == "--config" || arg == "-c") && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		} else if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
		}
	}
	// Default: auto-discover ./magic.yaml when no --config flag is set.
	if configPath == "" {
		if _, err := os.Stat("magic.yaml"); err == nil {
			configPath = "magic.yaml"
			log.Printf("[config] using default ./magic.yaml (override with --config)")
		}
	} else {
		log.Printf("[config] loading from %s", configPath)
	}
	// Secret provider is constructed before config so credentials can be
	// resolved through it (MAGIC_API_KEY, MAGIC_POSTGRES_URL, LLM keys).
	// Non-secret knobs (port, proxy trust, pool sizes, pgvector dim) stay
	// on direct os.Getenv. See docs/security/secrets.md.
	secretProvider, err := secrets.NewFromEnv()
	if err != nil {
		log.Fatalf("Failed to init secret provider: %v", err)
	}
	log.Printf("[secrets] provider: %s", secretProvider.Name())

	cfg, err := config.LoadWithSecrets(context.Background(), configPath, secretProvider)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// OpenTelemetry tracing — controlled by OTEL_EXPORTER_OTLP_ENDPOINT
	// (no-op when unset, so zero overhead for dev).
	tracingShutdown, err := tracing.Setup(context.Background())
	if err != nil {
		log.Fatalf("[tracing] init failed: %v", err)
	}
	defer func() {
		sCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer sCancel()
		if err := tracingShutdown(sCtx); err != nil {
			log.Printf("[tracing] shutdown: %v", err)
		}
	}()
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		log.Printf("[tracing] OTLP exporter: %s", ep)
	} else {
		log.Printf("[tracing] disabled (set OTEL_EXPORTER_OTLP_ENDPOINT to enable)")
	}

	port := cfg.Port

	if cfg.APIKey != "" && len(cfg.APIKey) < 32 {
		log.Fatalf("[security] MAGIC_API_KEY must be at least 32 characters (got %d). Generate one with: openssl rand -hex 32", len(cfg.APIKey))
	}

	// Store — auto-detect backend from config
	var s store.Store
	switch cfg.Store.Driver {
	case "postgres":
		pgURL := cfg.Store.PostgresURL
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
	case "sqlite":
		sqliteStore, err := store.NewSQLiteStore(cfg.Store.SQLitePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
			os.Exit(1)
		}
		s = sqliteStore
		fmt.Printf("  Storage: SQLite (%s)\n", cfg.Store.SQLitePath)
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
	bus.OnDrop = func() { monitor.MetricEventsDroppedTotal.Inc() }
	reg := registry.New(s, bus)
	rt := router.New(reg, s, bus)
	mon := monitor.New(bus, os.Stdout)
	mon.Start()
	stopHealthCheck := reg.StartHealthCheck(30_000_000_000)

	// Tier 2
	cc := costctrl.New(s, bus)
	stopCostReset := cc.StartDailyReset()
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

	// AI modules
	llmGW := llm.NewGateway()
	llmGW.OnCost = func(model, provider string, cost float64, usage llm.Usage) {
		cc.RecordCost("llm:"+provider+":"+model, "llm-request", cost)
	}
	if key := cfg.LLM.OpenAI.APIKey; key != "" {
		llmGW.RegisterProvider(llm.NewOpenAIProvider(key, cfg.LLM.OpenAI.BaseURL))
		fmt.Println("  LLM: OpenAI enabled")
	}
	if key := cfg.LLM.Anthropic.APIKey; key != "" {
		llmGW.RegisterProvider(llm.NewAnthropicProvider(key))
		fmt.Println("  LLM: Anthropic enabled")
	}
	if url := cfg.LLM.Ollama.URL; url != "" {
		llmGW.RegisterProvider(llm.NewOllamaProvider(url))
		fmt.Println("  LLM: Ollama enabled")
	}
	prompts := prompt.NewRegistry()
	agentMemory := memory.NewStore(nil)

	// RBAC + Policy engine
	rbacEnforcer := rbac.New(s)
	policyEngine := policy.New(s, bus)

	wh.Start()

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	var dispatchWG sync.WaitGroup

	orch.SetShutdownContext(shutdownCtx)

	// OIDC / JWT authentication (optional). When MAGIC_OIDC_ISSUER is set,
	// the gateway additionally accepts JWT bearer tokens validated against
	// the issuer's JWKS. Existing API-key auth keeps working in parallel.
	var oidcVerifier *auth.OIDCVerifier
	if issuer := os.Getenv("MAGIC_OIDC_ISSUER"); issuer != "" {
		clientID := os.Getenv("MAGIC_OIDC_CLIENT_ID")
		audience := os.Getenv("MAGIC_OIDC_AUDIENCE")
		discCtx, discCancel := context.WithTimeout(context.Background(), 10*time.Second)
		v, err := auth.NewOIDCVerifier(discCtx, issuer, clientID, audience)
		discCancel()
		if err != nil {
			log.Fatalf("[security] OIDC discovery failed: %v", err)
		}
		oidcVerifier = v
		log.Printf("[security] OIDC/JWT auth: enabled (issuer=%s)", issuer)
	} else {
		log.Printf("[security] OIDC/JWT auth: disabled (set MAGIC_OIDC_ISSUER to enable)")
	}

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
		RBAC:         rbacEnforcer,
		Policy:       policyEngine,
		ShutdownCtx:  shutdownCtx,
		DispatchWG:   &dispatchWG,
		LLM:          llmGW,
		Prompts:      prompts,
		Memory:       agentMemory,
		OIDC:         oidcVerifier,
		APIKey:       cfg.APIKey,
	})

	if s.HasAnyWorkerTokens(context.Background()) {
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
		if cfg.APIKey != "" {
			fmt.Println("  Authentication: enabled (MAGIC_API_KEY via " + secretProvider.Name() + ")")
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
		fmt.Println("  POST /api/v1/orgs/{orgID}/roles    — Manage RBAC roles")
		fmt.Println("  POST /api/v1/orgs/{orgID}/policies  — Manage policies")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-done
	fmt.Println("\nShutting down gracefully...")

	shutdownCancel() // cancel in-flight dispatches
	dispatchWG.Wait() // wait for gateway dispatches
	orch.Wait()       // wait for workflow step dispatches

	stopHealthCheck()
	stopCostReset()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Shutdown error: %v\n", err)
	}
	fmt.Println("Server stopped.")
}
