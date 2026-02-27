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
	"strings"
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
	ScenariosDir    string
	HTTPAddr        string
	EnableOverrides map[string]bool
	EnableRRCF      bool
	CUSUMIncludeCount bool
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./scenarios", "Directory containing scenario subdirectories")
	httpAddr := flag.String("http", ":8080", "HTTP server address for the API")
	enableCUSUM := flag.Bool("cusum", true, "Enable CUSUM change-point detector")
	enableZScore := flag.Bool("zscore", true, "Enable Robust Z-Score detector")
	enableBOCPD := flag.Bool("bocpd", true, "Enable BOCPD change-point detector")
	enableTimeCluster := flag.Bool("time-cluster", true, "Enable TimeCluster correlator (time-based clustering)")
	enableLeadLag := flag.Bool("lead-lag", false, "Enable LeadLag correlator (temporal causality)")
	enableSurprise := flag.Bool("surprise", false, "Enable Surprise correlator (lift-based patterns)")
	enableGraphSketch := flag.Bool("graph-sketch", false, "Enable GraphSketch correlator (co-occurrence learning)")
	enableDedup := flag.Bool("dedup", false, "Enable anomaly deduplication before correlation")
	enableRRCF := flag.Bool("rrcf", false, "Enable RRCF multivariate anomaly detector")
	cusumIncludeCount := flag.Bool("cusum-include-count", false, "CUSUM: include :count metrics (default: skip them)")
	flag.Parse()

	overrides := map[string]bool{
		"cusum":        *enableCUSUM,
		"zscore":       *enableZScore,
		"bocpd":        *enableBOCPD,
		"time_cluster": *enableTimeCluster,
		"lead_lag":     *enableLeadLag,
		"surprise":     *enableSurprise,
		"graph_sketch": *enableGraphSketch,
		"dedup":        *enableDedup,
	}

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
			EnableOverrides:   overrides,
			EnableRRCF:        *enableRRCF,
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
		EnableOverrides:   params.EnableOverrides,
		EnableRRCF:        params.EnableRRCF,
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

	// Print component status from registry
	components := tb.GetComponents()
	fmt.Print("Analyzers: ")
	var analyzers []string
	for _, c := range components {
		if c.Category == "analyzer" {
			status := c.DisplayName
			if !c.Enabled {
				status += " (disabled)"
			}
			analyzers = append(analyzers, status)
		}
	}
	if params.EnableRRCF {
		analyzers = append(analyzers, "RRCF")
	}
	fmt.Println(strings.Join(analyzers, ", "))

	fmt.Print("Correlators: ")
	var correlators []string
	for _, c := range components {
		if c.Category == "correlator" {
			status := c.DisplayName
			if !c.Enabled {
				status += " (disabled)"
			}
			correlators = append(correlators, status)
		}
	}
	fmt.Println(strings.Join(correlators, ", "))

	fmt.Print("Processing: ")
	var processing []string
	for _, c := range components {
		if c.Category == "processing" && c.Enabled {
			processing = append(processing, c.DisplayName)
		}
	}
	if len(processing) == 0 {
		fmt.Println("default")
	} else {
		fmt.Println(strings.Join(processing, ", "))
	}

	fmt.Println("\nEndpoints:")
	fmt.Println("  GET  /api/status                          - Server status")
	fmt.Println("  GET  /api/scenarios                       - List scenarios")
	fmt.Println("  POST /api/scenarios/{name}/load            - Load a scenario")
	fmt.Println("  GET  /api/components                      - List components")
	fmt.Println("  POST /api/components/{name}/toggle         - Toggle component")
	fmt.Println("  GET  /api/series                          - List all series")
	fmt.Println("  GET  /api/series/{ns}/{name}              - Get series data")
	fmt.Println("  GET  /api/anomalies                       - Get anomalies")
	fmt.Println("  GET  /api/correlations                    - Get correlations")
	fmt.Println("  GET  /api/correlators/{name}              - Get correlator data")
	fmt.Println("  GET  /api/rrcf-scores                     - RRCF score distribution (if enabled)")
	fmt.Println("  GET  /api/stats                           - Correlator stats")
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
