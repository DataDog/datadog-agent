// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"

	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
)

func main() {
	timeScale := flag.Float64("timescale", 0.25, "time multiplier (0.25 = 4x faster)")
	httpAddr := flag.String("http", "", "HTTP address for web dashboard (e.g., :8080)")
	parquetDir := flag.String("parquet", "", "directory containing FGM parquet files for replay")
	loop := flag.Bool("loop", false, "loop parquet replay after reaching the end")

	// Algorithm selection flags (Layer 1 emitters)
	cusum := flag.Bool("cusum", false, "enable CUSUM change-point detector")
	lightESD := flag.Bool("lightesd", false, "enable LightESD statistical outlier detector")
	graphSketch := flag.Bool("graphsketch", false, "enable GraphSketch edge anomaly detector")

	// Correlator selection
	timeClusterCorrelator := flag.Bool("time-cluster", true, "use TimeClusterCorrelator to group anomalies by timestamp proximity")
	graphSketchCorrelator := flag.Bool("graphsketch-correlator", false, "use GraphSketchCorrelator to group anomalies by co-occurrence patterns")
	leadLagCorrelator := flag.Bool("lead-lag", false, "use LeadLagCorrelator to detect temporal lead-lag patterns")
	surpriseCorrelator := flag.Bool("surprise", false, "use SurpriseCorrelator to detect unexpected co-occurrences")

	// Deduplication
	enableDedup := flag.Bool("dedup", false, "enable anomaly deduplication before correlation")
	enableRCA := flag.Bool("rca", false, "enable RCA ranking/explanations for supported correlators")

	// Output
	outputFile := flag.String("output", "", "path to write JSON results (anomalies + correlations)")

	// Processing mode
	processAll := flag.Bool("all", false, "process all parquet data without time limit")

	// ========== Tuning Parameters ==========
	// CUSUM tuning
	cusumBaselineFraction := flag.Float64("cusum-baseline-fraction", 0, "CUSUM: fraction of data for baseline (default: 0.25)")
	cusumSlackFactor := flag.Float64("cusum-slack-factor", 0, "CUSUM: multiplier for stddev → slack (default: 0.5)")
	cusumThresholdFactor := flag.Float64("cusum-threshold-factor", 0, "CUSUM: multiplier for stddev → threshold (default: 4.0)")

	// LightESD tuning
	lightesdMinWindow := flag.Int("lightesd-min-window-size", 0, "LightESD: min points for analysis (default: 50)")
	lightesdAlpha := flag.Float64("lightesd-alpha", 0, "LightESD: significance level (default: 0.05)")
	lightesdTrendFrac := flag.Float64("lightesd-trend-window-fraction", 0, "LightESD: fraction for trend smoothing (default: 0.15)")
	lightesdPeriodSig := flag.Float64("lightesd-periodicity-significance", 0, "LightESD: p-value for seasonality (default: 0.01)")
	lightesdMaxPeriods := flag.Int("lightesd-max-periods", 0, "LightESD: max seasonal components (default: 2)")

	// GraphSketch correlator tuning
	gsCoOccurrence := flag.Int64("graphsketch-cooccurrence-window", 0, "GraphSketch: seconds for co-occurrence (default: 10)")
	gsDecay := flag.Float64("graphsketch-decay-factor", 0, "GraphSketch: time decay factor (default: 0.85)")
	gsMinCorrelation := flag.Float64("graphsketch-min-correlation", 0, "GraphSketch: min edge strength (default: 2.0)")
	gsEdgeLimit := flag.Int("graphsketch-edge-limit", 0, "GraphSketch: max edges to track (default: 200)")

	// TimeCluster correlator tuning
	tcSlackSeconds := flag.Int64("timecluster-slack-seconds", 0, "TimeCluster: seconds of slack for grouping (default: 1)")

	// LeadLag correlator tuning
	llMaxLag := flag.Int64("leadlag-max-lag", 0, "LeadLag: max lag seconds to track (default: 30)")
	llMinObs := flag.Int("leadlag-min-obs", 0, "LeadLag: min observations for edge (default: 3)")
	llConfidence := flag.Float64("leadlag-confidence", 0, "LeadLag: confidence threshold (default: 0.6)")

	// Surprise correlator tuning
	surpriseWindow := flag.Int64("surprise-window", 0, "Surprise: window size seconds (default: 10)")
	surpriseMinLift := flag.Float64("surprise-min-lift", 0, "Surprise: min lift threshold (default: 2.0)")
	surpriseMinSupport := flag.Int("surprise-min-support", 0, "Surprise: min co-occurrences (default: 2)")

	// Dedup tuning
	dedupBucket := flag.Int64("dedup-bucket", 0, "Dedup: bucket size seconds (default: 5)")

	// RCA tuning
	rcaTopK := flag.Int("rca-top-k", 0, "RCA: max root candidates to emit (default: 3)")
	rcaMaxPaths := flag.Int("rca-max-paths", 0, "RCA: max evidence paths to emit (default: 3)")
	rcaOnsetEpsilon := flag.Int64("rca-onset-epsilon", 0, "RCA: onset epsilon seconds for direction ties (default: 1)")
	rcaMaxEdgeLag := flag.Int64("rca-max-edge-lag", 0, "RCA: max lag seconds used for temporal edges (default: 10)")

	flag.Parse()

	// If no emitters specified, default to CUSUM
	enableCUSUM := *cusum
	if !*cusum && !*lightESD && !*graphSketch {
		enableCUSUM = true
	}

	// Run V2 demo with selected algorithms
	observerimpl.RunDemoV2WithConfig(observerimpl.DemoV2Config{
		TimeScale:                   *timeScale,
		HTTPAddr:                    *httpAddr,
		ParquetDir:                  *parquetDir,
		Loop:                        *loop,
		EnableCUSUM:                 enableCUSUM,
		EnableLightESD:              *lightESD,
		EnableGraphSketch:           *graphSketch,
		UseTimeClusterCorrelator:    *timeClusterCorrelator,
		EnableGraphSketchCorrelator: *graphSketchCorrelator,
		EnableLeadLagCorrelator:     *leadLagCorrelator,
		EnableSurpriseCorrelator:    *surpriseCorrelator,
		EnableDedup:                 *enableDedup,
		EnableRCA:                   *enableRCA,
		OutputFile:                  *outputFile,
		ProcessAllData:              *processAll,

		// Tuning parameters (0 = use default)
		CUSUMBaselineFraction:           *cusumBaselineFraction,
		CUSUMSlackFactor:                *cusumSlackFactor,
		CUSUMThresholdFactor:            *cusumThresholdFactor,
		LightESDMinWindowSize:           *lightesdMinWindow,
		LightESDAlpha:                   *lightesdAlpha,
		LightESDTrendWindowFraction:     *lightesdTrendFrac,
		LightESDPeriodicitySignificance: *lightesdPeriodSig,
		LightESDMaxPeriods:              *lightesdMaxPeriods,
		GraphSketchCoOccurrenceWindow:   *gsCoOccurrence,
		GraphSketchDecayFactor:          *gsDecay,
		GraphSketchMinCorrelation:       *gsMinCorrelation,
		GraphSketchEdgeLimit:            *gsEdgeLimit,
		TimeClusterSlackSeconds:         *tcSlackSeconds,
		LeadLagMaxLag:                   *llMaxLag,
		LeadLagMinObs:                   *llMinObs,
		LeadLagConfidence:               *llConfidence,
		SurpriseWindowSeconds:           *surpriseWindow,
		SurpriseMinLift:                 *surpriseMinLift,
		SurpriseMinSupport:              *surpriseMinSupport,
		DedupBucketSeconds:              *dedupBucket,
		RCAMaxRootCandidates:            *rcaTopK,
		RCAMaxEvidencePaths:             *rcaMaxPaths,
		RCAOnsetEpsilonSeconds:          *rcaOnsetEpsilon,
		RCAMaxEdgeLagSeconds:            *rcaMaxEdgeLag,
	})
}
