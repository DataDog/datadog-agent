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

// DemoConfig configures the demo run.
type DemoConfig struct {
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
}

// RunDemo runs the full demo scenario and blocks until complete.
// timeScale controls speed: 1.0 = realtime (70s), 0.1 = 10x faster (7s)
func RunDemo(timeScale float64) {
	RunDemoWithConfig(DemoConfig{TimeScale: timeScale})
}

// RunDemoWithConfig runs the demo with the given configuration.
func RunDemoWithConfig(config DemoConfig) {
	if config.TimeScale <= 0 {
		config.TimeScale = 0.1
	}

	fmt.Printf("Starting observer demo (timeScale=%.2f, duration=%.1fs)\n", config.TimeScale, phaseTotalDuration*config.TimeScale)

	// Create components directly for demo so we can wire up HTML reporter
	// Toggle between pattern-based and time-based correlation:
	useTimeBasedCorrelation := true

	var correlator interface {
		observerdef.AnomalyProcessor
		ActiveCorrelations() []observerdef.ActiveCorrelation
	}
	if useTimeBasedCorrelation {
		correlator = NewTimeClusterCorrelator(TimeClusterConfig{
			ProximitySeconds: 1,  // Only 1 second proximity - anomalies must be nearly simultaneous
			MinClusterSize:   2,  // need at least 2 anomalies to report
			WindowSeconds:    60, // keep anomalies for 60s
		})
	} else {
		correlator = NewCorrelator(CorrelatorConfig{})
	}
	stdoutReporter := &StdoutReporter{}
	stdoutReporter.SetCorrelationState(correlator)

	storage := newTimeSeriesStorage()

	reporters := []observerdef.Reporter{stdoutReporter}

	// Optionally add HTML reporter
	var htmlReporter *HTMLReporter
	if config.HTTPAddr != "" {
		htmlReporter = NewHTMLReporter()
		htmlReporter.SetCorrelationState(correlator)
		htmlReporter.SetStorage(storage)
		reporters = append(reporters, htmlReporter)

		if err := htmlReporter.Start(config.HTTPAddr); err != nil {
			fmt.Printf("Failed to start HTML reporter: %v\n", err)
		} else {
			fmt.Printf("HTML dashboard available at http://localhost%s\n", config.HTTPAddr)
		}
	}

	fmt.Println("---")

	obs := &observerImpl{
		logProcessors: []observerdef.LogProcessor{
			&ConnectionErrorExtractor{},
		},
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			NewCUSUMDetector(),
			NewGraphSketchEmitter(DefaultGraphSketchConfig()),
			NewLightESDEmitter(DefaultLightESDConfig()),
		},
		anomalyProcessors: []observerdef.AnomalyProcessor{
			correlator,
		},
		reporters:        reporters,
		storage:          storage,
		obsCh:            make(chan observation, 1000),
		rawAnomalyWindow: 120, // keep raw anomalies for 2 minutes
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

	fmt.Println("---")
	fmt.Println("Demo complete.")

	// Stop HTML reporter if it was started
	if htmlReporter != nil {
		_ = htmlReporter.Stop()
	}
}
