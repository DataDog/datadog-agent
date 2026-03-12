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

	// Headless mode: run a scenario and exit (no HTTP server)
	Headless string  // scenario name to run (empty = interactive mode)
	Output   string  // path for observer JSON output
	Verbose  bool    // include full detail in JSON output (headless mode only)
	Score    bool    // include scoring in output (headless mode only)
	Sigma    float64 // Gaussian width for scoring

	// SendAnomalyEvent mode: run scenario and send one Datadog event per correlation
	SendAnomalyEvent string // scenario name to run (empty = disabled)
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./comp/observer/scenarios", "Directory containing scenario subdirectories")
	httpAddr := flag.String("http", ":8080", "HTTP server address for the API")
	enableStr := flag.String("enable", "", "Comma-separated components to enable (overrides defaults)")
	disableStr := flag.String("disable", "", "Comma-separated components to disable (overrides defaults)")
	headless := flag.String("headless", "", "Run scenario in headless mode (no HTTP server) and exit")
	output := flag.String("output", "", "Path for eval JSON output (headless mode only)")
	verbose := flag.Bool("verbose", false, "Include full detail in JSON output (headless mode only)")
	score := flag.Bool("score", false, "Score analysis against ground truth (headless mode only)")
	sigma := flag.Float64("sigma", 30.0, "Gaussian width in seconds for scoring")
	sendAnomalyEvent := flag.String("send-anomaly-event", "", "Run scenario and send one Datadog event per correlation, then exit")
	flag.Parse()

	overrides := make(map[string]bool)
	if *enableStr != "" {
		for _, name := range strings.Split(*enableStr, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				overrides[name] = true
			}
		}
	}
	if *disableStr != "" {
		for _, name := range strings.Split(*disableStr, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				overrides[name] = false
			}
		}
	}

	if *headless == "" {
		fmt.Printf("Observer Test Bench\n")
		fmt.Printf("  Scenarios dir: %s\n", *scenariosDir)
		fmt.Printf("  HTTP address:  %s\n", *httpAddr)
		fmt.Println()
	}

	err := fxutil.OneShot(run,
		recorderfx.Module(),
		core.Bundle(),
		secretsnoopfx.Module(),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(""),
			LogParams:    log.ForOneShot("", "off", true),
		}),
		fx.Supply(CLIParams{
			ScenariosDir:     *scenariosDir,
			HTTPAddr:         *httpAddr,
			EnableOverrides:  overrides,
			Headless:         *headless,
			Output:           *output,
			Verbose:          *verbose,
			Score:            *score,
			Sigma:            *sigma,
			SendAnomalyEvent: *sendAnomalyEvent,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run observer test bench: %v\n", err)
		os.Exit(1)
	}
}

func run(recorder recorderdef.Component, cfg config.Component, logger log.Component, params CLIParams) error {
	// Create the test bench
	tb, err := observerimpl.NewTestBench(observerimpl.TestBenchConfig{
		ScenariosDir:    params.ScenariosDir,
		HTTPAddr:        params.HTTPAddr,
		Recorder:        recorder,
		Cfg:             cfg,
		Logger:          logger,
		EnableOverrides: params.EnableOverrides,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create test bench: %v\n", err)
		return err
	}

	// SendAnomalyEvent mode: run scenario and send one Datadog event per correlation, then exit.
	if params.SendAnomalyEvent != "" {
		return tb.RunSendAnomalyEvents(params.SendAnomalyEvent)
	}

	// Headless mode: run scenario, write output, exit (no HTTP server)
	if params.Headless != "" {
		if err := tb.RunHeadless(params.Headless, params.Output, observerimpl.HeadlessOptions{
			Verbose: params.Verbose,
			Score:   params.Score,
			Sigma:   params.Sigma,
		}); err != nil {
			return err
		}
		return nil
	}

	if err := tb.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start test bench: %v\n", err)
		return err
	}

	fmt.Printf("API server running at http://localhost%s\n", params.HTTPAddr)

	// Print component status from registry
	components := tb.GetComponents()
	fmt.Print("Detectors: ")
	var detectors []string
	for _, c := range components {
		if c.Category == "detector" {
			status := c.DisplayName
			if !c.Enabled {
				status += " (disabled)"
			}
			detectors = append(detectors, status)
		}
	}
	fmt.Println(strings.Join(detectors, ", "))

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
	fmt.Println("  GET  /api/components/{name}/data           - Get component data")
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
