package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/config"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Port: %d", cfg.Port)
	log.Printf("  Log Level: %s", cfg.LogLevel)
	log.Printf("  Mode: %s", cfg.Mode)

	// Create MCP server
	mcpServer := server.New(cfg.Port, cfg.Mode)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v", sig)
		cancel()
	}()

	// Start server
	if err := mcpServer.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped gracefully")
}
