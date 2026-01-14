package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/k8s"
)

// Server represents the MCP evaluation server
type Server struct {
	mcpServer  *mcp.Server
	httpServer *http.Server
	k8sClient  *k8s.Client
	port       int
}

// New creates a new MCP server
func New(
	k8sClient *k8s.Client,
	port int,
) *Server {
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
		k8sClient: k8sClient,
		port:      port,
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

// Start starts the MCP server
func (s *Server) Start(ctx context.Context) error {
	// Create HTTP/SSE handler for MCP protocol
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s.mcpServer },
		nil,
	)

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle(
		"/mcp",
		handler,
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
	log.Printf(
		"Kubernetes context: %s\n",
		s.k8sClient.Context(),
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
