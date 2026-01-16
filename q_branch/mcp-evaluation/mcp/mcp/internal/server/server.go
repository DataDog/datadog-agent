package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools/files"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools/network"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools/process"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools/sysinfo"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/tools/system"
)

// Server represents the MCP evaluation server
type Server struct {
	mcpServer  *mcp.Server
	httpServer *http.Server
	port       int
	mode       string
}

// New creates a new MCP server
func New(port int, mode string) *Server {
	// Create MCP server with implementation metadata
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-evaluation",
			Version: "0.1.0",
		},
		nil,
	)

	s := &Server{
		mcpServer: mcpServer,
		port:      port,
		mode:      mode,
	}

	// Register tools based on mode
	log.Printf("Starting MCP server in %s mode", mode)

	switch mode {
	case "bash":
		// Only register bash tool
		bashTool := tools.NewBashTool(30 * time.Second)
		if err := bashTool.Register(mcpServer); err != nil {
			log.Printf("Failed to register bash tool: %v", err)
		} else {
			log.Printf("Registered tool: bash_execute")
		}

	case "safe-shell":
		// Only register safe-shell tool
		safeShellTool, err := tools.NewSafeShellTool(30 * time.Second)
		if err != nil {
			log.Fatalf("Failed to create safe-shell tool: %v", err)
		}
		if err := safeShellTool.Register(mcpServer); err != nil {
			log.Printf("Failed to register safe-shell tool: %v", err)
		} else {
			log.Printf("Registered tool: safe_shell_execute")
		}

	case "tools":
		// Register diagnostic tools for SRE/on-call scenarios
		var registrationErrors []string

		// System Resources (4 tools)
		if err := system.NewGetMemoryInfoTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_memory_info: %v", err))
		}
		if err := system.NewGetDiskUsageTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_disk_usage: %v", err))
		}
		if err := system.NewGetCPUInfoTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_cpu_info: %v", err))
		}
		if err := system.NewGetIOStatsTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_io_stats: %v", err))
		}

		// Process Management (3 tools)
		if err := process.NewListProcessesTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("list_processes: %v", err))
		}
		if err := process.NewGetProcessInfoTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_process_info: %v", err))
		}
		if err := process.NewFindProcessTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("find_process: %v", err))
		}

		// Network (4 tools)
		if err := network.NewGetNetworkInterfacesTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_network_interfaces: %v", err))
		}
		if err := network.NewGetListeningPortsTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_listening_ports: %v", err))
		}
		if err := network.NewGetNetworkConnectionsTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_network_connections: %v", err))
		}
		if err := network.NewCheckConnectivityTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("check_connectivity: %v", err))
		}

		// Files (3 tools)
		if err := files.NewReadFileTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("read_file: %v", err))
		}
		if err := files.NewTailFileTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("tail_file: %v", err))
		}
		if err := files.NewSearchFileTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("search_file: %v", err))
		}

		// System Info (2 tools)
		if err := sysinfo.NewGetSystemInfoTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_system_info: %v", err))
		}
		if err := sysinfo.NewGetEnvironmentTool().Register(mcpServer); err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("get_environment: %v", err))
		}

		if len(registrationErrors) > 0 {
			log.Printf("Failed to register some tools:")
			for _, err := range registrationErrors {
				log.Printf("  - %s", err)
			}
		}

		log.Printf("Registered 16 diagnostic tools for SRE/on-call scenarios")

	default:
		log.Fatalf("Invalid mode: %s (this should not happen - validation failed)", mode)
	}

	return s
}

// RegisterTool registers a new tool with the MCP server
// This is prepared for future tool implementations
func (s *Server) RegisterTool(
	name, description string,
	schema interface{},
	handler mcp.ToolHandler,
) {
	s.mcpServer.AddTool(
		&mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: schema,
		},
		handler,
	)
}

// loggingMiddleware wraps an HTTP handler with request logging
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		log.Printf("[MCP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Log headers for debugging
		if sessionID := r.Header.Get("x-mcp-session-id"); sessionID != "" {
			log.Printf("[MCP] Session ID: %s", sessionID)
		}

		// Call the actual handler
		next.ServeHTTP(w, r)

		log.Printf("[MCP] %s %s completed in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// Start starts the MCP server
func (s *Server) Start(ctx context.Context) error {
	// Create HTTP/SSE handler for MCP protocol with JSON response option
	opts := &mcp.StreamableHTTPOptions{
		JSONResponse: true, // Return JSON instead of SSE
	}
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcpServer },
		opts,
	)

	// Wrap handler with logging middleware
	loggedHandler := loggingMiddleware(handler)

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle(
		"/mcp",
		loggedHandler,
	)

	// Add health check endpoint
	mux.HandleFunc(
		"/health",
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(
				w,
				"OK\n",
			)
		},
	)

	s.httpServer = &http.Server{
		Addr: fmt.Sprintf(
			"127.0.0.1:%d",
			s.port,
		),
		Handler: mux,
	}

	log.Printf(
		"Starting MCP evaluation server on http://127.0.0.1:%d/mcp\n",
		s.port,
	)
	log.Printf(
		"Health check endpoint: http://127.0.0.1:%d/health\n",
		s.port,
	)

	// Start server in a goroutine
	errChan := make(
		chan error,
		1,
	)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case err := <-errChan:
		return fmt.Errorf(
			"server error: %w",
			err,
		)
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down MCP server...")

	if s.httpServer == nil {
		return nil
	}

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}
