// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/module"
)

var (
	socketPath = flag.String("socket", "/opt/datadog-agent/run/service-discovery.sock", "Unix socket path")
	configPath = flag.String("config", "", "Path to configuration file")
)

// standaloneDiscovery wraps the module discovery to provide HTTP endpoints
type standaloneDiscovery struct {
	module *module.StandaloneDiscoveryModule
}

// newStandaloneDiscovery creates a new standalone discovery service without system-probe dependencies
func newStandaloneDiscovery() (*standaloneDiscovery, error) {
	// Create the discovery module with minimal dependencies
	discoveryModule, err := module.NewStandaloneDiscoveryModule()
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery module: %w", err)
	}

	return &standaloneDiscovery{
		module: discoveryModule,
	}, nil
}

// setupRoutes configures HTTP routes compatible with system-probe interface
func (sd *standaloneDiscovery) setupRoutes() *mux.Router {
	router := mux.NewRouter()

	// Service discovery endpoints that match system-probe paths
	discoveryRouter := router.PathPrefix("/discovery").Subrouter()

	// Register the module endpoints
	moduleRouter := &moduleRouterWrapper{router: discoveryRouter}
	if err := sd.module.Register(moduleRouter); err != nil {
		log.Printf("Failed to register module routes: %v", err)
	}

	return router
}

// moduleRouterWrapper adapts mux.Router to module.Router interface
type moduleRouterWrapper struct {
	router *mux.Router
}

func (w *moduleRouterWrapper) HandleFunc(pattern string, handler http.HandlerFunc) {
	w.router.HandleFunc(pattern, handler)
}

func main() {
	flag.Parse()

	// TODO: Add configuration support if needed
	if *configPath != "" {
		fmt.Printf("Configuration file specified but not yet implemented: %s\n", *configPath)
	}

	// Initialize basic logging
	log.SetPrefix("[service-discovery] ")

	log.Println("Starting standalone service discovery process")

	// Create standalone discovery service
	discovery, err := newStandaloneDiscovery()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create discovery service: %v\n", err)
		os.Exit(1)
	}

	// Setup HTTP routes
	router := discovery.setupRoutes()

	// Remove existing socket file
	if err := os.RemoveAll(*socketPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Failed to remove existing socket: %v\n", err)
		os.Exit(1)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on Unix socket %s: %v\n", *socketPath, err)
		os.Exit(1)
	}
	defer listener.Close()

	// Set socket permissions
	if err := os.Chmod(*socketPath, 0770); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set socket permissions: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Service discovery listening on Unix socket: %s", *socketPath)

	// Create HTTP server
	server := &http.Server{
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down service discovery process")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Clean up
	discovery.module.Close()
	log.Println("Service discovery process stopped")
}
