// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides the entry point for the observer test bench.
// The test bench is a standalone tool for iterating on observer components
// by loading scenarios and visualizing analysis results.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/fx"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type CLIParams struct {
	ScenariosDir string
	HTTPAddr     string
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./scenarios", "Directory containing scenario subdirectories")
	httpAddr := flag.String("http", ":8080", "HTTP server address for the API")
	flag.Parse()

	fmt.Printf("Observer Test Bench\n")
	fmt.Printf("  Scenarios dir: %s\n", *scenariosDir)
	fmt.Printf("  HTTP address:  %s\n", *httpAddr)
	fmt.Println()

	err := fxutil.OneShot(run,
		recorderfx.Module(),
		core.Bundle(),
		secretsnoopfx.Module(),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(""),
			LogParams:    log.ForOneShot("", "off", true),
		}),
		fx.Supply(CLIParams{
			ScenariosDir: *scenariosDir,
			HTTPAddr:     *httpAddr,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run observer test bench: %v\n", err)
		os.Exit(1)
	}
}

func run(recorder recorderdef.Component, params CLIParams) error {
	// Create and start the test bench
	tb, err := observerimpl.NewTestBench(observerimpl.TestBenchConfig{
		ScenariosDir: params.ScenariosDir,
		HTTPAddr:     params.HTTPAddr,
		Recorder:     recorder,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create test bench: %v\n", err)
		return err
	}

	if err := tb.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start test bench: %v\n", err)
		return err
	}

	fmt.Printf("API server running at http://localhost%s\n", params.HTTPAddr)
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /api/status              - Server status and loaded scenario")
	fmt.Println("  GET  /api/scenarios           - List available scenarios")
	fmt.Println("  POST /api/scenarios/{name}/load - Load a scenario")
	fmt.Println("  GET  /api/components          - List registered components")
	fmt.Println("  GET  /api/series              - List all series")
	fmt.Println("  GET  /api/series/{ns}/{name}  - Get series data")
	fmt.Println("  GET  /api/anomalies           - Get all anomalies")
	fmt.Println("  GET  /api/correlations        - Get correlation outputs")
	fmt.Println()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	if err := tb.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}

	return nil
}
