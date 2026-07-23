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
	"time"

	"go.uber.org/fx"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	recordernoop "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx-noop"
	reportertestbenchfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx-testbench"
	testbenchimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl-testbench"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/internal/qbranch/anomalydetection-testbench/bench"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

	// ParquetFormat selects the parquet file layout. Empty string = auto-detect.
	ParquetFormat bench.ParquetFormat
}

func main() {
	scenariosDir := flag.String("scenarios-dir", "./comp/anomalydetection/observer/scenarios", "Directory containing scenario subdirectories")
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
	parquetFormat := flag.String("parquet-format", "", "Parquet layout: v1 (observer-metrics-*/observer-logs-*), v2 (contexts.parquet + metrics-*/logs-*), or empty to auto-detect")
	attributionScenario := flag.String("pattern-attribution", "", "Analyze semantic-vs-Logs tokenizer cluster mappings for one scenario and exit")
	attributionOutput := flag.String("pattern-attribution-output", "", "Path for pattern-attribution JSON output")
	attributionThreshold := flag.Float64("pattern-attribution-threshold", 0.5, "Positional token-match threshold for pattern attribution")
	baselineDuration := flag.String("baseline-duration", "", "Baseline analysis window duration (e.g. \"7m\", \"0\" to disable). Default: enabled with 10m window.")
	muteNoisyMetrics := flag.Bool("mute-noisy-metrics", true, "Mute metrics that fire anomalies during the baseline window")
	flag.Parse()

	if *attributionScenario != "" {
		if *attributionOutput == "" {
			fmt.Fprintln(os.Stderr, "--pattern-attribution-output is required with --pattern-attribution")
			os.Exit(2)
		}
		if err := bench.RunPatternAttribution(*scenariosDir, *attributionScenario, *attributionOutput, *attributionThreshold); err != nil {
			fmt.Fprintf(os.Stderr, "Pattern attribution failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// --config takes full precedence over --enable/--disable/--only.
	var componentSettings observerimpl.ComponentSettings
	if *configFile != "" {
		loaded, err := bench.LoadTestbenchParams(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config file: %v\n", err)
			os.Exit(1)
		}
		componentSettings = loaded
	} else {
		overrides := make(map[string]bool)
		if *onlyStr != "" {
			onlySet := make(map[string]bool)
			for _, name := range strings.Split(*onlyStr, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					onlySet[name] = true
				}
			}
			for _, entry := range observerimpl.TestbenchCatalogEntries() {
				if entry.Kind == "extractor" {
					continue
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

	if *baselineDuration == "0" || *baselineDuration == "disabled" {
		componentSettings.Baseline = observerimpl.BaselineConfig{Enabled: false}
	} else if *baselineDuration != "" {
		dur, err := time.ParseDuration(*baselineDuration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --baseline-duration %q: %v\n", *baselineDuration, err)
			os.Exit(1)
		}
		componentSettings.Baseline = observerimpl.BaselineConfig{
			Enabled:          true,
			DurationSec:      int64(dur.Seconds()),
			MuteNoisyMetrics: *muteNoisyMetrics,
		}
	} else {
		componentSettings.Baseline = observerimpl.BaselineConfig{
			Enabled:          true,
			DurationSec:      300,
			MuteNoisyMetrics: *muteNoisyMetrics,
		}
	}

	if *headless == "" {
		fmt.Printf("Observer Test Bench\n")
		fmt.Printf("  Scenarios dir: %s\n", *scenariosDir)
		fmt.Printf("  HTTP address:  %s\n", *httpAddr)
		if *logsOnly {
			fmt.Printf("  Logs-only:     true (parquet metrics and trace stats are not loaded)\n")
		}
		b := componentSettings.Baseline
		if b.Enabled {
			mode := "mute"
			if !b.MuteNoisyMetrics {
				mode = "observe"
			}
			fmt.Printf("  Baseline:      %dm window, mode=%s\n", b.DurationSec/60, mode)
		} else {
			fmt.Printf("  Baseline:      disabled\n")
		}
		fmt.Println()
	}

	err := fxutil.OneShot(run,
		recordernoop.Module(),
		observerfx.Module(),
		reportertestbenchfx.Module(),
		// Observer optional deps not needed by the testbench.
		fx.Supply(option.None[workloadmetadef.Component]()),
		fx.Supply(option.None[workloadfilterdef.Component]()),
		fx.Supply(option.None[taggerdef.Component]()),
		core.Bundle(),
		// The testbench drives the engine directly via DebugView, so it needs the
		// full observerImpl, not the disabled stub that NewComponent returns when
		// anomaly detection is off. Force the feature on; replay is driven by
		// DebugView.Reset with the testbench's own ComponentSettings, so this does
		// not change scenario results. Keep the agent-internal log tap off so
		// pkg/util/log messages (e.g. from parquet loading) are never ingested as
		// scenario data — the testbench feeds the engine exclusively via DebugView.
		fx.Decorate(func(c config.Component) config.Component {
			c.Set("anomaly_detection.enabled", true, pkgconfigmodel.SourceAgentRuntime)
			c.Set("anomaly_detection.logs.internal.enabled", false, pkgconfigmodel.SourceAgentRuntime)
			return c
		}),
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
			ParquetFormat:      bench.ParquetFormat(*parquetFormat),
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run observer test bench: %v\n", err)
		os.Exit(1)
	}
}

func run(
	obs observerdef.Component,
	sseAccess testbenchimpl.SSEAccess,
	cfg config.Component,
	logger log.Component,
	params CLIParams,
) error {
	debug, ok := obs.(observerimpl.DebugView)
	if !ok {
		return fmt.Errorf("observer does not implement DebugView")
	}

	tb, err := bench.New(obs, debug, sseAccess, bench.Config{
		ScenariosDir:       params.ScenariosDir,
		HTTPAddr:           params.HTTPAddr,
		Cfg:                cfg,
		Logger:             logger,
		ComponentSettings:  params.ComponentSettings,
		SkipDroppedMetrics: params.SkipDroppedMetrics,
		LogsOnly:           params.LogsOnly,
		ParquetFormat:      params.ParquetFormat,
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	if err := tb.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}

	return nil
}
