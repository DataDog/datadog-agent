// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// DemoV2Config configures the V2 demo run with algorithm selection.
type DemoV2Config struct {
	// TimeScale controls speed: 1.0 = realtime (70s), 0.1 = 10x faster (7s)
	TimeScale float64
	// HTTPAddr is the address for the HTML reporter server (e.g., ":8080").
	// If empty, HTML reporter is disabled and only stdout is used.
	HTTPAddr string
	// ParquetDir is the directory containing FGM parquet files for replay.
	// If set, the demo will replay from parquet instead of generating synthetic data.
	ParquetDir string
	// Loop controls whether to loop parquet replay after reaching the end.
	Loop bool

	// Algorithm selection (Layer 1 emitters)
	// EnableCUSUM enables the CUSUM change-point detector (produces range-based anomalies)
	EnableCUSUM bool
	// EnableLightESD enables the LightESD statistical outlier detector (produces point-based signals)
	EnableLightESD bool
	// EnableGraphSketch enables the GraphSketch edge anomaly detector (produces point-based signals)
	EnableGraphSketch bool
	// EnableFFADE enables the F-FADE frequency-based anomaly detector (Layer 2 processor)
	EnableFFADE bool

	// Correlator selection (mutually exclusive)
	// UseTimeClusterCorrelator uses time-based clustering (anomalies within N seconds cluster together)
	UseTimeClusterCorrelator bool
	// EnableGraphSketchCorrelator enables the GraphSketch-based anomaly correlator
	// This detects unusual co-occurrence patterns between anomalies
	EnableGraphSketchCorrelator bool
}

// RunDemoV2 runs the demo with the new signal-based architecture.
// Uses CUSUM for anomaly detection and TimeClusterCorrelator for correlation.
func RunDemoV2(timeScale float64) {
	RunDemoV2WithConfig(DemoV2Config{TimeScale: timeScale})
}

// RunDemoV2WithConfig runs the demo with the given configuration.
func RunDemoV2WithConfig(config DemoV2Config) {
	if config.TimeScale <= 0 {
		config.TimeScale = 0.1
	}

	fmt.Printf("Starting observer demo V2 (timeScale=%.2f, duration=%.1fs)\n", config.TimeScale, phaseTotalDuration*config.TimeScale)

	// Correlator selection (mutually exclusive)
	var correlator observerdef.AnomalyProcessor
	var gsCorrelator *GraphSketchCorrelator // Keep a specific pointer for debug and freezing
	if config.EnableGraphSketchCorrelator {
		gsc := NewGraphSketchCorrelator(DefaultGraphSketchCorrelatorConfig())
		correlator = gsc
		gsCorrelator = gsc // Store for later debug print and freeze
	} else if config.UseTimeClusterCorrelator {
		correlator = NewTimeClusterCorrelator(DefaultTimeClusterConfig())
	}

	stdoutReporter := &StdoutReporter{}
	if correlator != nil {
		stdoutReporter.SetCorrelationState(correlator.(observerdef.CorrelationState))
	}

	storage := newTimeSeriesStorage()

	reporters := []observerdef.Reporter{stdoutReporter}

	// Optionally add HTML reporter
	var htmlReporter *HTMLReporter
	if config.HTTPAddr != "" {
		htmlReporter = NewHTMLReporter()
		if correlator != nil {
			htmlReporter.SetCorrelationState(correlator.(observerdef.CorrelationState))
		}
		htmlReporter.SetStorage(storage)
		reporters = append(reporters, htmlReporter)

		if err := htmlReporter.Start(config.HTTPAddr); err != nil {
			fmt.Printf("Failed to start HTML reporter: %v\n", err)
		} else {
			fmt.Printf("HTML dashboard available at http://localhost%s\n", config.HTTPAddr)
		}
	}

	// Build signal emitters list based on config (for point-based anomaly detection)
	var emitters []observerdef.SignalEmitter
	var emitterNames []string

	// CUSUM goes on the OLD path (TimeSeriesAnalysis) for range-based visualization
	// Other emitters go on the NEW path (SignalEmitter) for point-based visualization
	var tsAnalysesForRanges []observerdef.TimeSeriesAnalysis
	var tsAnalysisNames []string

	if config.EnableCUSUM {
		// Use CUSUM as TimeSeriesAnalysis for range-based anomalies (shaded regions on charts)
		tsAnalysesForRanges = append(tsAnalysesForRanges, NewCUSUMDetector())
		tsAnalysisNames = append(tsAnalysisNames, "CUSUM (ranges)")
	}
	if config.EnableLightESD {
		emitters = append(emitters, NewLightESDEmitter(DefaultLightESDConfig()))
		emitterNames = append(emitterNames, "LightESD (points)")
	}
	if config.EnableGraphSketch {
		emitters = append(emitters, NewGraphSketchEmitter(DefaultGraphSketchConfig()))
		emitterNames = append(emitterNames, "GraphSketch (points)")
	}

	// Build signal processors list based on config
	var processors []observerdef.SignalProcessor
	var processorNames []string

	if config.EnableFFADE {
		processors = append(processors, NewFFADEProcessor(DefaultFFADEConfig()))
		processorNames = append(processorNames, "F-FADE")
	}

	fmt.Println("---")
	fmt.Println("Enabled algorithms:")
	if len(tsAnalysisNames) > 0 {
		fmt.Printf("  Range-based (OLD path): %v\n", tsAnalysisNames)
	}
	if len(emitterNames) > 0 {
		fmt.Printf("  Point-based (NEW path): %v\n", emitterNames)
	}
	if config.EnableGraphSketchCorrelator {
		fmt.Println("  Correlator: GraphSketchCorrelator (co-occurrence patterns)")
	} else if config.UseTimeClusterCorrelator {
		fmt.Println("  Correlator: TimeClusterCorrelator (time proximity)")
	} else {
		fmt.Println("  Correlator: None (individual anomalies)")
	}
	if len(processorNames) > 0 {
		fmt.Printf("  Layer 2 (Processors): %v\n", processorNames)
	}
	fmt.Println("---")

	// Build anomaly processors list
	var anomalyProcessors []observerdef.AnomalyProcessor
	if correlator != nil {
		anomalyProcessors = append(anomalyProcessors, correlator)
	}

	obs := &observerImpl{
		logProcessors: []observerdef.LogProcessor{
			&ConnectionErrorExtractor{},
		},
		// OLD path: Region-based anomaly detection (CUSUM produces time ranges)
		tsAnalyses: tsAnalysesForRanges,
		// NEW path Layer 1: Point-based signal emitters
		signalEmitters: emitters,
		// Anomaly processor for correlation
		anomalyProcessors: anomalyProcessors,
		// NEW path Layer 2: Signal processors (optional)
		signalProcessors: processors,
		reporters:        reporters,
		storage:          storage,
		obsCh:            make(chan observation, 1000),
		rawAnomalyWindow: 0,   // 0 = unlimited - keep all anomalies during demo
		maxRawAnomalies:  500, // keep up to 500 raw anomalies
	}
	go obs.run()

	// Wire raw anomaly state to reporters for test bench display
	stdoutReporter.SetRawAnomalyState(obs)
	if htmlReporter != nil {
		htmlReporter.SetRawAnomalyState(obs)
	}

	// Get a handle for the demo generator
	handle := obs.GetHandle("demo")

	// Choose between parquet replay and synthetic data generation
	var ctx context.Context
	var cancel context.CancelFunc

	if config.ParquetDir != "" {
		// Parquet replay mode
		fmt.Printf("Using parquet replay from: %s\n", config.ParquetDir)
		replayGen, err := NewParquetReplayGenerator(handle, ParquetReplayConfig{
			ParquetDir: config.ParquetDir,
			TimeScale:  config.TimeScale,
			Loop:       config.Loop,
		})
		if err != nil {
			fmt.Printf("Failed to create parquet replay generator: %v\n", err)
			return
		}

		// For parquet replay, use a long timeout or no timeout if looping
		if config.Loop {
			ctx, cancel = context.WithCancel(context.Background())
		} else {
			// Give enough time for the replay to complete
			ctx, cancel = context.WithTimeout(context.Background(), 1*time.Hour)
		}
		defer cancel()

		replayGen.Run(ctx)
	} else {
		// Synthetic data generation mode
		generator := NewDataGenerator(handle, GeneratorConfig{
			TimeScale:     config.TimeScale,
			BaselineNoise: 0.1,
		})

		// Run the generator with a timeout for the scenario duration (70s scaled)
		scenarioDuration := time.Duration(float64(phaseTotalDuration) * float64(time.Second) * config.TimeScale)
		ctx, cancel = context.WithTimeout(context.Background(), scenarioDuration)
		defer cancel()

		generator.Run(ctx)
	}

	// Small buffer to let final events flush through the pipeline
	time.Sleep(time.Duration(float64(500*time.Millisecond) * config.TimeScale))

	// Print final cluster state
	stdoutReporter.PrintFinalState()

	// Freeze the correlator and print debug state if enabled
	if gsCorrelator != nil {
		gsCorrelator.Freeze() // Explicitly freeze when replay finishes
		gsCorrelator.PrintDebugState()
	}

	fmt.Println("---")
	fmt.Println("Demo complete.")

	// Keep HTTP server running if started (so user can explore results)
	if htmlReporter != nil {
		fmt.Println("")
		fmt.Printf("Dashboard still available at http://localhost%s\n", config.HTTPAddr)
		fmt.Println("Press Ctrl+C to exit...")

		// Block forever - wait for interrupt signal
		select {}
	}
}
