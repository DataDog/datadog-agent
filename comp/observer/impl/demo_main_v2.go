// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	// Correlator selection (mutually exclusive)
	// UseTimeClusterCorrelator uses time-based clustering (anomalies within N seconds cluster together)
	UseTimeClusterCorrelator bool
	// EnableGraphSketchCorrelator enables the GraphSketch-based anomaly correlator
	// This detects unusual co-occurrence patterns between anomalies
	EnableGraphSketchCorrelator bool
	// EnableLeadLagCorrelator enables temporal lead-lag pattern detection
	EnableLeadLagCorrelator bool
	// EnableSurpriseCorrelator enables lift-based surprise pattern detection
	EnableSurpriseCorrelator bool

	// Deduplication
	// EnableDedup enables anomaly deduplication before correlation
	EnableDedup bool

	// OutputFile is the path to write JSON results (anomalies + correlations)
	// If empty, no file is written
	OutputFile string

	// ProcessAllData if true, processes all parquet data without time limit
	// If false, runs for a fixed duration based on TimeScale
	ProcessAllData bool

	// ========== Tuning Parameters ==========
	// CUSUM parameters (ts_analysis_cusum.go)
	CUSUMBaselineFraction float64 // Default: 0.25, fraction of data for baseline
	CUSUMSlackFactor      float64 // Default: 0.5, multiplier for stddev → slack
	CUSUMThresholdFactor  float64 // Default: 4.0, multiplier for stddev → threshold

	// LightESD parameters (emitter_lightesd.go)
	LightESDMinWindowSize           int     // Default: 50
	LightESDAlpha                   float64 // Default: 0.05, significance level
	LightESDTrendWindowFraction     float64 // Default: 0.15
	LightESDPeriodicitySignificance float64 // Default: 0.01
	LightESDMaxPeriods              int     // Default: 2

	// GraphSketch correlator parameters (anomaly_processor_graphsketch.go)
	GraphSketchCoOccurrenceWindow int64   // Default: 10, seconds for co-occurrence
	GraphSketchDecayFactor        float64 // Default: 0.85
	GraphSketchMinCorrelation     float64 // Default: 2.0
	GraphSketchEdgeLimit          int     // Default: 200

	// TimeCluster correlator parameters (anomaly_processor_time_cluster.go)
	TimeClusterSlackSeconds int64 // Default: 1

	// LeadLag correlator parameters (anomaly_processor_leadlag.go)
	LeadLagMaxLag     int64   // Default: 30, max lag seconds to track
	LeadLagMinObs     int     // Default: 3, min observations for edge
	LeadLagConfidence float64 // Default: 0.6, confidence threshold

	// Surprise correlator parameters (anomaly_processor_surprise.go)
	SurpriseWindowSeconds int64   // Default: 10, window size
	SurpriseMinLift       float64 // Default: 2.0, min lift threshold
	SurpriseMinSupport    int     // Default: 2, min co-occurrences

	// Dedup parameters (anomaly_dedup.go)
	DedupBucketSeconds int64 // Default: 5, time bucket for dedup
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

	if config.ProcessAllData {
		fmt.Println("Starting observer demo V2 (processing ALL data, no time limit)")
	} else {
		fmt.Printf("Starting observer demo V2 (timeScale=%.2f, duration=%.1fs)\n", config.TimeScale, phaseTotalDuration*config.TimeScale)
	}

	// Correlator selection (mutually exclusive for primary correlator)
	var correlator observerdef.AnomalyProcessor
	var correlationState observerdef.CorrelationState
	var gsCorrelator *GraphSketchCorrelator // Keep a specific pointer for debug and freezing
	var tcCorrelator *TimeClusterCorrelator // Keep a specific pointer for visualization
	var llCorrelator *LeadLagCorrelator     // Keep a specific pointer for lead-lag
	var surpriseCorr *SurpriseCorrelator    // Keep a specific pointer for surprise

	// Deduplication layer (wraps the correlator)
	var deduplicator *AnomalyDeduplicator
	if config.EnableDedup {
		dedupConfig := DefaultAnomalyDedupConfig()
		if config.DedupBucketSeconds > 0 {
			dedupConfig.BucketSizeSeconds = config.DedupBucketSeconds
		}
		deduplicator = NewAnomalyDeduplicator(dedupConfig)
	}

	if config.EnableLeadLagCorrelator {
		llConfig := DefaultLeadLagConfig()
		// Apply tuning overrides
		if config.LeadLagMaxLag > 0 {
			llConfig.MaxLagSeconds = config.LeadLagMaxLag
		}
		if config.LeadLagMinObs > 0 {
			llConfig.MinObservations = config.LeadLagMinObs
		}
		if config.LeadLagConfidence > 0 {
			llConfig.ConfidenceThreshold = config.LeadLagConfidence
		}
		llc := NewLeadLagCorrelator(llConfig)
		correlator = llc
		correlationState = llc
		llCorrelator = llc
	} else if config.EnableSurpriseCorrelator {
		surpriseConfig := DefaultSurpriseConfig()
		// Apply tuning overrides
		if config.SurpriseWindowSeconds > 0 {
			surpriseConfig.WindowSizeSeconds = config.SurpriseWindowSeconds
		}
		if config.SurpriseMinLift > 0 {
			surpriseConfig.MinLift = config.SurpriseMinLift
		}
		if config.SurpriseMinSupport > 0 {
			surpriseConfig.MinSupport = config.SurpriseMinSupport
		}
		sc := NewSurpriseCorrelator(surpriseConfig)
		correlator = sc
		correlationState = sc
		surpriseCorr = sc
	} else if config.EnableGraphSketchCorrelator {
		gsConfig := DefaultGraphSketchCorrelatorConfig()
		// Apply tuning overrides
		if config.GraphSketchCoOccurrenceWindow > 0 {
			gsConfig.CoOccurrenceWindow = config.GraphSketchCoOccurrenceWindow
		}
		if config.GraphSketchDecayFactor > 0 {
			gsConfig.DecayFactor = config.GraphSketchDecayFactor
		}
		if config.GraphSketchMinCorrelation > 0 {
			gsConfig.MinCorrelationStrength = config.GraphSketchMinCorrelation
		}
		if config.GraphSketchEdgeLimit > 0 {
			gsConfig.Width = config.GraphSketchEdgeLimit // edge limit uses Width
		}
		gsc := NewGraphSketchCorrelator(gsConfig)
		correlator = gsc
		correlationState = gsc
		gsCorrelator = gsc // Store for later debug print and freeze
	} else if config.UseTimeClusterCorrelator {
		tcConfig := DefaultTimeClusterConfig()
		// Apply tuning overrides
		if config.TimeClusterSlackSeconds > 0 {
			tcConfig.ProximitySeconds = config.TimeClusterSlackSeconds
		}
		tc := NewTimeClusterCorrelator(tcConfig)
		correlator = tc
		correlationState = tc
		tcCorrelator = tc // Store for visualization
	}

	// Suppress unused variable warnings
	_ = llCorrelator
	_ = surpriseCorr

	stdoutReporter := &StdoutReporter{}
	if correlationState != nil {
		stdoutReporter.SetCorrelationState(correlationState)
	}

	storage := newTimeSeriesStorage()

	reporters := []observerdef.Reporter{stdoutReporter}

	// Optionally add HTML reporter
	var htmlReporter *HTMLReporter
	if config.HTTPAddr != "" {
		htmlReporter = NewHTMLReporter()
		if correlationState != nil {
			htmlReporter.SetCorrelationState(correlationState)
		}
		if gsCorrelator != nil {
			htmlReporter.SetGraphSketchCorrelator(gsCorrelator)
		}
		if tcCorrelator != nil {
			htmlReporter.SetTimeClusterCorrelator(tcCorrelator)
		}
		htmlReporter.SetStorage(storage)
		reporters = append(reporters, htmlReporter)

		if err := htmlReporter.Start(config.HTTPAddr); err != nil {
			fmt.Printf("Failed to start HTML reporter: %v\n", err)
		} else {
			fmt.Printf("HTML dashboard available at http://localhost%s\n", config.HTTPAddr)
		}
	}

	// Build signal emitters list based on config
	var emitters []observerdef.SignalEmitter
	var emitterNames []string

	// Multi-series analyses (pull-based, includes wrapped TimeSeriesAnalysis)
	var multiAnalyses []observerdef.MultiSeriesAnalysis
	var tsAnalysisNames []string
	defaultAggregations := []observerdef.Aggregate{observerdef.AggregateAverage, observerdef.AggregateCount}

	if config.EnableCUSUM {
		cusumDetector := NewCUSUMDetector()
		// Apply tuning overrides
		if config.CUSUMBaselineFraction > 0 {
			cusumDetector.BaselineFraction = config.CUSUMBaselineFraction
		}
		if config.CUSUMSlackFactor > 0 {
			cusumDetector.SlackFactor = config.CUSUMSlackFactor
		}
		if config.CUSUMThresholdFactor > 0 {
			cusumDetector.ThresholdFactor = config.CUSUMThresholdFactor
		}
		multiAnalyses = append(multiAnalyses, newTimeSeriesAnalysisAdapter(cusumDetector, defaultAggregations))
		tsAnalysisNames = append(tsAnalysisNames, "CUSUM")
	}
	if config.EnableLightESD {
		lightesdConfig := DefaultLightESDConfig()
		// Apply tuning overrides
		if config.LightESDMinWindowSize > 0 {
			lightesdConfig.MinWindowSize = config.LightESDMinWindowSize
		}
		if config.LightESDAlpha > 0 {
			lightesdConfig.Alpha = config.LightESDAlpha
		}
		if config.LightESDTrendWindowFraction > 0 {
			lightesdConfig.TrendWindowFraction = config.LightESDTrendWindowFraction
		}
		if config.LightESDPeriodicitySignificance > 0 {
			lightesdConfig.PeriodicitySignificance = config.LightESDPeriodicitySignificance
		}
		if config.LightESDMaxPeriods > 0 {
			lightesdConfig.MaxPeriods = config.LightESDMaxPeriods
		}
		emitters = append(emitters, NewLightESDEmitter(lightesdConfig))
		emitterNames = append(emitterNames, "LightESD")
	}
	if config.EnableGraphSketch {
		emitters = append(emitters, NewGraphSketchEmitter(DefaultGraphSketchConfig()))
		emitterNames = append(emitterNames, "GraphSketch")
	}

	// Build signal processors list based on config
	var processors []observerdef.SignalProcessor

	fmt.Println("---")
	fmt.Println("Enabled algorithms:")
	if len(emitterNames) > 0 {
		fmt.Printf("  Detector: %v\n", emitterNames)
	}
	if len(tsAnalysisNames) > 0 {
		fmt.Printf("  Detector (legacy): %v\n", tsAnalysisNames)
	}
	if config.EnableDedup {
		fmt.Println("  Dedup: Enabled (Stable Bloom Filter)")
	}
	if config.EnableLeadLagCorrelator {
		fmt.Println("  Correlator: LeadLagCorrelator (temporal lead-lag patterns)")
	} else if config.EnableSurpriseCorrelator {
		fmt.Println("  Correlator: SurpriseCorrelator (lift-based surprise patterns)")
	} else if config.EnableGraphSketchCorrelator {
		fmt.Println("  Correlator: GraphSketchCorrelator (co-occurrence patterns)")
	} else if config.UseTimeClusterCorrelator {
		fmt.Println("  Correlator: TimeClusterCorrelator (time proximity)")
	} else {
		fmt.Println("  Correlator: None (individual anomalies)")
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
		// Multi-series analyses (CUSUM wrapped in adapter)
		multiAnalyses: multiAnalyses,
		// Point-based signal emitters
		signalEmitters: emitters,
		// Anomaly processor for correlation
		anomalyProcessors: anomalyProcessors,
		// NEW path Layer 2: Signal processors (optional)
		signalProcessors: processors,
		// Deduplication layer (optional)
		deduplicator:     deduplicator,
		reporters:        reporters,
		storage:          storage,
		obsCh:            make(chan observation, 10000000), // Very large buffer (10M) to prevent drops during demo with large datasets
		rawAnomalyWindow: 0,                                // 0 = unlimited - keep all anomalies during demo
		maxRawAnomalies:  500,                              // keep up to 500 raw anomalies
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

		// Run the generator - either with timeout or until all data is processed
		if config.ProcessAllData {
			// No timeout - process all parquet data
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			fmt.Println("[parquet-replay] Processing ALL data (no time limit)...")
		} else {
			// Run with timeout for the scenario duration (70s scaled)
			scenarioDuration := time.Duration(float64(phaseTotalDuration) * float64(time.Second) * config.TimeScale)
			ctx, cancel = context.WithTimeout(context.Background(), scenarioDuration)
			defer cancel()
		}

		generator.Run(ctx)
	}

	// Let final events flush through the pipeline (fixed 2 seconds, not scaled)
	fmt.Println("[demo] Waiting for pipeline to flush...")
	time.Sleep(2 * time.Second)

	// Flush any remaining anomalies from correlators
	if correlator != nil {
		fmt.Println("[demo] Flushing correlator...")
		correlator.Flush()
	}

	// Another small wait for any final processing
	time.Sleep(500 * time.Millisecond)

	// Print final cluster state
	stdoutReporter.PrintFinalState()

	// Freeze the correlator and print debug state if enabled
	if gsCorrelator != nil {
		gsCorrelator.Freeze() // Explicitly freeze when replay finishes
		gsCorrelator.PrintDebugState()
	}

	fmt.Println("---")
	fmt.Println("Demo complete.")

	// Export results to file if requested
	fmt.Printf("[DEBUG] OutputFile=%q\n", config.OutputFile)
	if config.OutputFile != "" {
		fmt.Println("[DEBUG] Calling exportResults...")
		exportResults(config, obs, correlationState, gsCorrelator)
	}

	// Keep HTTP server running if started (so user can explore results)
	if htmlReporter != nil {
		fmt.Println("")
		fmt.Printf("Dashboard still available at http://localhost%s\n", config.HTTPAddr)
		fmt.Println("Press Ctrl+C to exit...")

		// Block forever - wait for interrupt signal
		select {}
	}
}

// DemoResults contains the exported demo results for comparison between detectors.
type DemoResults struct {
	// Metadata about the run
	Detector   string `json:"detector"`
	Correlator string `json:"correlator"`
	Timestamp  string `json:"timestamp"`

	// Summary counts (for quick comparison)
	TotalAnomalies           int `json:"total_anomalies"`
	UniqueSourcesInAnomalies int `json:"unique_sources_in_anomalies"`
	TotalCorrelations        int `json:"total_correlations"`
	TotalEdges               int `json:"total_edges,omitempty"`
	DedupSkipped             int `json:"dedup_skipped,omitempty"` // Anomalies filtered by deduplication

	// Sample of anomalies (first 20 for reference)
	SampleAnomalies []AnomalySample `json:"sample_anomalies,omitempty"`

	// Correlations found (from GraphSketchCorrelator or TimeClusterCorrelator)
	Correlations []CorrelationResult `json:"correlations"`

	// GraphSketch edges (if using GraphSketchCorrelator) - all edges
	Edges []EdgeResult `json:"edges,omitempty"`
}

// AnomalySample is a simplified anomaly for the sample output.
type AnomalySample struct {
	Source      string   `json:"source"`
	Analyzer    string   `json:"analyzer"`
	Description string   `json:"description"`
	Timestamp   int64    `json:"timestamp"`
	Tags        []string `json:"tags,omitempty"`
}

// CorrelationResult represents a correlation pattern.
type CorrelationResult struct {
	Pattern     string   `json:"pattern"`
	Title       string   `json:"title"`
	SourceCount int      `json:"source_count"`
	Sources     []string `json:"sources"` // top 20 sources only
	FirstSeen   int64    `json:"first_seen"`
	LastUpdated int64    `json:"last_updated"`
}

// EdgeResult represents a GraphSketch edge.
type EdgeResult struct {
	Source1      string  `json:"source1"`
	Source2      string  `json:"source2"`
	Observations int     `json:"observations"`
	Frequency    float64 `json:"frequency"`
}

func exportResults(config DemoV2Config, obs *observerImpl, correlationState observerdef.CorrelationState, gsCorrelator *GraphSketchCorrelator) {
	// Determine detector name
	detectorName := "unknown"
	if config.EnableCUSUM {
		detectorName = "CUSUM"
	} else if config.EnableLightESD {
		detectorName = "LightESD"
	} else if config.EnableGraphSketch {
		detectorName = "GraphSketch"
	}

	// Determine correlator name
	correlatorName := "none"
	if config.EnableLeadLagCorrelator {
		correlatorName = "LeadLagCorrelator"
	} else if config.EnableSurpriseCorrelator {
		correlatorName = "SurpriseCorrelator"
	} else if config.EnableGraphSketchCorrelator {
		correlatorName = "GraphSketchCorrelator"
	} else if config.UseTimeClusterCorrelator {
		correlatorName = "TimeClusterCorrelator"
	}
	if config.EnableDedup {
		correlatorName = "Dedup+" + correlatorName
	}

	results := DemoResults{
		Detector:   detectorName,
		Correlator: correlatorName,
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	// Get total anomaly counts (uncapped)
	results.TotalAnomalies = obs.TotalAnomalyCount()
	results.UniqueSourcesInAnomalies = obs.UniqueAnomalySourceCount()
	results.DedupSkipped = obs.DedupSkippedCount()

	// Export sample of anomalies (first 20)
	rawAnomalies := obs.RawAnomalies()
	sampleSize := 20
	if len(rawAnomalies) < sampleSize {
		sampleSize = len(rawAnomalies)
	}
	for i := 0; i < sampleSize; i++ {
		a := rawAnomalies[i]
		// Use Timestamp if set, otherwise fall back to TimeRange.End
		ts := a.Timestamp
		if ts == 0 {
			ts = a.TimeRange.End
		}
		results.SampleAnomalies = append(results.SampleAnomalies, AnomalySample{
			Source:      string(a.Source),
			Analyzer:    a.AnalyzerName,
			Description: a.Description,
			Timestamp:   ts,
			Tags:        a.Tags,
		})
	}

	// Export correlations (with all sources)
	if correlationState != nil {
		activeCorrs := correlationState.ActiveCorrelations()
		for _, c := range activeCorrs {
			results.Correlations = append(results.Correlations, CorrelationResult{
				Pattern:     c.Pattern,
				Title:       c.Title,
				SourceCount: len(c.MemberSeriesIDs),
				Sources:     seriesIDsToStringsForResults(c.MemberSeriesIDs), // All sources, not truncated
				FirstSeen:   c.FirstSeen,
				LastUpdated: c.LastUpdated,
			})
		}
	}
	results.TotalCorrelations = len(results.Correlations)

	// Export all GraphSketch edges if available
	if gsCorrelator != nil {
		allEdges := gsCorrelator.GetLearnedEdges()
		results.TotalEdges = len(allEdges)

		for _, e := range allEdges {
			results.Edges = append(results.Edges, EdgeResult{
				Source1:      e.Source1,
				Source2:      e.Source2,
				Observations: e.Observations,
				Frequency:    e.Frequency,
			})
		}
		fmt.Printf("  - %d edges exported\n", len(results.Edges))
	}

	// Write to file
	f, err := os.Create(config.OutputFile)
	if err != nil {
		fmt.Printf("Failed to create output file: %v\n", err)
		return
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fmt.Printf("Failed to write results: %v\n", err)
		return
	}

	fmt.Printf("Results written to: %s\n", config.OutputFile)
	fmt.Printf("  - %d anomalies (%d unique sources)\n", results.TotalAnomalies, results.UniqueSourcesInAnomalies)
	fmt.Printf("  - %d correlations\n", results.TotalCorrelations)
	if results.TotalEdges > 0 {
		fmt.Printf("  - %d edges\n", results.TotalEdges)
	}
}

func seriesIDsToStringsForResults(ids []observerdef.SeriesID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
