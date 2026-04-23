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
	"runtime/pprof"
	"strings"
	"syscall"

	"go.uber.org/fx"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	recorderfx "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type CLIParams struct {
	ScenariosDir      string
	HTTPAddr          string
	ComponentSettings observerimpl.ComponentSettings

	// Headless mode: run a scenario and exit (no HTTP server)
	Headless   string // scenario name to run (empty = interactive mode)
	Output     string // path for observer JSON output
	Verbose    bool   // include full detail in JSON output (headless mode only)
	MemProfile string // path to write heap profile after headless run (empty = disabled)

	// SendAnomalyEvent mode: run scenario and send one Datadog event per correlation
	SendAnomalyEvent string // scenario name to run (empty = disabled)

	SkipDroppedMetrics bool // skip metrics marked as dropped during parquet load

	// LogsOnly skips ingesting parquet metrics and trace stats; only log rows are loaded.
	LogsOnly bool
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./comp/observer/scenarios", "Directory containing scenario subdirectories")
	httpAddr := flag.String("http", ":8080", "HTTP server address for the API")
	enableStr := flag.String("enable", "", "Comma-separated components to enable (overrides defaults)")
	disableStr := flag.String("disable", "", "Comma-separated components to disable (overrides defaults)")
	onlyStr := flag.String("only", "", "Enable ONLY these components (plus extractors); disable everything else. Mutually exclusive with --enable/--disable.")
	configFile := flag.String("config", "", "Path to JSON params file for component enabled state and hyperparameters (for Bayesian optimization). Overrides --enable/--disable/--only.")
	headless := flag.String("headless", "", "Run scenario in headless mode (no HTTP server) and exit")
	output := flag.String("output", "", "Path for eval JSON output (headless mode only)")
	verbose := flag.Bool("verbose", false, "Include full detail in JSON output (headless mode only)")
	memProfile := flag.String("memprofile", "", "Write heap profile to this file after headless run (headless mode only)")
	sendAnomalyEvent := flag.String("send-anomaly-event", "", "Run scenario and send one Datadog event per correlation, then exit")
	skipDropped := flag.Bool("skip-dropped", true, "Skip metrics marked as dropped by the live observer's channel during parquet load")
	logsOnly := flag.Bool("logs-only", false, "Load only log rows from scenarios; skip parquet metrics and trace stats (interactive and headless)")
	flag.Parse()

	// --config takes full precedence over --enable/--disable/--only.
	var componentSettings observerimpl.ComponentSettings
	if *configFile != "" {
		loaded, err := observerimpl.LoadTestbenchParams(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config file: %v\n", err)
			os.Exit(1)
		}
		componentSettings = loaded
	} else {
		overrides := make(map[string]bool)
		if *onlyStr != "" {
			// --only: enable listed components + extractors, disable everything else.
			onlySet := make(map[string]bool)
			for _, name := range strings.Split(*onlyStr, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					onlySet[name] = true
				}
			}
			for _, entry := range observerimpl.TestbenchCatalogEntries() {
				if entry.Kind == "extractor" {
					continue // extractors always enabled
				}
				overrides[entry.Name] = onlySet[entry.Name]
			}
		} else {
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
		}
		componentSettings = observerimpl.ComponentSettings{Enabled: overrides}
	}

	if *headless == "" {
		fmt.Printf("Observer Test Bench\n")
		fmt.Printf("  Scenarios dir: %s\n", *scenariosDir)
		fmt.Printf("  HTTP address:  %s\n", *httpAddr)
		if *logsOnly {
			fmt.Printf("  Logs-only:     true (parquet metrics and trace stats are not loaded)\n")
		}
		fmt.Println()
	}

	err := fxutil.OneShot(run,
		recorderfx.Module(),
		core.Bundle(),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(""),
			LogParams:    log.ForOneShot("", "off", true),
		}),
		fx.Supply(CLIParams{
			ScenariosDir:       *scenariosDir,
			HTTPAddr:           *httpAddr,
			ComponentSettings:  componentSettings,
			Headless:           *headless,
			Output:             *output,
			Verbose:            *verbose,
			MemProfile:         *memProfile,
			SendAnomalyEvent:   *sendAnomalyEvent,
			SkipDroppedMetrics: *skipDropped,
			LogsOnly:           *logsOnly,
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
		ScenariosDir:       params.ScenariosDir,
		HTTPAddr:           params.HTTPAddr,
		Recorder:           recorder,
		Cfg:                cfg,
		Logger:             logger,
		ComponentSettings:  params.ComponentSettings,
		SkipDroppedMetrics: params.SkipDroppedMetrics,
		LogsOnly:           params.LogsOnly,
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
		if err := tb.RunHeadless(params.Headless, params.Output, params.Verbose); err != nil {
			return err
		}
		if params.MemProfile != "" {
			f, err := os.Create(params.MemProfile)
			if err != nil {
				return fmt.Errorf("could not create mem profile: %w", err)
			}
			defer f.Close()
			if err := pprof.WriteHeapProfile(f); err != nil {
				return fmt.Errorf("could not write mem profile: %w", err)
			}
			fmt.Printf("Heap profile written to %s\n", params.MemProfile)
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
	fmt.Println("  GET  /api/reports                         - Get Datadog-style report events")
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
