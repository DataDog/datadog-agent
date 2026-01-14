package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/config"
	"github.com/DataDog/datadog-agent/q_branch/mcp-evaluation/mcp/internal/k8s"
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
	log.Printf("  Kubeconfig: %s", cfg.KubeconfigPath)
	log.Printf("  Context: %s", cfg.Context)
	log.Printf("  Log Level: %s", cfg.LogLevel)

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient(cfg.KubeconfigPath, cfg.Context)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Verify cluster connectivity
	ctx := context.Background()
	if err := k8sClient.HealthCheck(ctx); err != nil {
		log.Fatalf("Kubernetes cluster health check failed: %v", err)
	}
	log.Printf("Successfully connected to Kubernetes cluster")

	// Create MCP server
	mcpServer := server.New(k8sClient, cfg.Port)

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
