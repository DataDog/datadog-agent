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
	ScenariosDir      string
	HTTPAddr          string
	EnableTimeCluster bool
	EnableLeadLag     bool
	EnableSurprise    bool
	EnableGraphSketch bool
	EnableDedup       bool
	EnableCUSUM       bool
	EnableZScore      bool
	CUSUMIncludeCount bool
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./scenarios", "Directory containing scenario subdirectories")
	httpAddr := flag.String("http", ":8080", "HTTP server address for the API")
	enableTimeCluster := flag.Bool("time-cluster", true, "Enable TimeCluster correlator (time-based clustering)")
	enableLeadLag := flag.Bool("lead-lag", true, "Enable LeadLag correlator (temporal causality)")
	enableSurprise := flag.Bool("surprise", true, "Enable Surprise correlator (lift-based patterns)")
	enableGraphSketch := flag.Bool("graph-sketch", true, "Enable GraphSketch correlator (co-occurrence learning)")
	enableDedup := flag.Bool("dedup", false, "Enable anomaly deduplication before correlation")
	enableCUSUM := flag.Bool("cusum", true, "Enable CUSUM change-point detector")
	enableZScore := flag.Bool("zscore", true, "Enable Robust Z-Score detector")
	cusumIncludeCount := flag.Bool("cusum-include-count", false, "CUSUM: include :count metrics (default: skip them)")
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
			ScenariosDir:      *scenariosDir,
			HTTPAddr:          *httpAddr,
			EnableTimeCluster: *enableTimeCluster,
			EnableLeadLag:     *enableLeadLag,
			EnableSurprise:    *enableSurprise,
			EnableGraphSketch: *enableGraphSketch,
			EnableDedup:       *enableDedup,
			EnableCUSUM:       *enableCUSUM,
			EnableZScore:      *enableZScore,
			CUSUMIncludeCount: *cusumIncludeCount,
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
		ScenariosDir:      params.ScenariosDir,
		HTTPAddr:          params.HTTPAddr,
		Recorder:          recorder,
		EnableTimeCluster: params.EnableTimeCluster,
		EnableLeadLag:     params.EnableLeadLag,
		EnableSurprise:    params.EnableSurprise,
		EnableGraphSketch: params.EnableGraphSketch,
		EnableDedup:       params.EnableDedup,
		EnableCUSUM:       params.EnableCUSUM,
		EnableZScore:      params.EnableZScore,
		CUSUMIncludeCount: params.CUSUMIncludeCount,
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

	// Print enabled detectors
	fmt.Print("Detectors: ")
	var detectors []string
	if params.EnableCUSUM {
		cusum := "CUSUM"
		if params.CUSUMIncludeCount {
			cusum += " (with :count)"
		} else {
			cusum += " (skip :count)"
		}
		detectors = append(detectors, cusum)
	}
	if params.EnableZScore {
		detectors = append(detectors, "ZScore")
	}
	if len(detectors) == 0 {
		fmt.Println("none")
	} else {
		for i, d := range detectors {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(d)
		}
		fmt.Println()
	}

	// Print enabled correlators
	fmt.Print("Correlators: ")
	var correlators []string
	if params.EnableTimeCluster {
		correlators = append(correlators, "TimeCluster")
	}
	if params.EnableLeadLag {
		correlators = append(correlators, "LeadLag")
	}
	if params.EnableSurprise {
		correlators = append(correlators, "Surprise")
	}
	if params.EnableGraphSketch {
		correlators = append(correlators, "GraphSketch")
	}
	if len(correlators) == 0 {
		fmt.Println("none")
	} else {
		for i, c := range correlators {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(c)
		}
		fmt.Println()
	}

	// Print processing options
	fmt.Print("Processing: ")
	var processing []string
	if params.EnableDedup {
		processing = append(processing, "dedup")
	}
	if len(processing) == 0 {
		fmt.Println("default")
	} else {
		for i, p := range processing {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(p)
		}
		fmt.Println()
	}

	fmt.Println("\nEndpoints:")
	fmt.Println("  GET  /api/status              - Server status and loaded scenario")
	fmt.Println("  GET  /api/scenarios           - List available scenarios")
	fmt.Println("  POST /api/scenarios/{name}/load - Load a scenario")
	fmt.Println("  GET  /api/components          - List registered components")
	fmt.Println("  GET  /api/series              - List all series")
	fmt.Println("  GET  /api/series/{ns}/{name}  - Get series data")
	fmt.Println("  GET  /api/anomalies           - Get all anomalies")
	fmt.Println("  GET  /api/correlations        - Get correlation outputs")
	fmt.Println("  GET  /api/leadlag             - Lead-lag edges (if enabled)")
	fmt.Println("  GET  /api/surprise            - Surprise edges (if enabled)")
	fmt.Println("  GET  /api/graphsketch         - GraphSketch edges (if enabled)")
	fmt.Println("  GET  /api/stats               - Correlator statistics")
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
