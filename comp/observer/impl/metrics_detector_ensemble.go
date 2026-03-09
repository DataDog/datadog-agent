// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// defaultEnsembleExcludePrefixes is an expanded list of infrastructure/platform
// metric families excluded from ensemble ranking. This is a superset of
// defaultTopKExcludePrefixes — the ensemble needs more aggressive filtering
// because with only 10-20 output slots, even a few infrastructure metrics
// crowd out application signal.
//
// These are all general infrastructure metrics present in any Kubernetes
// deployment regardless of the application being monitored.
var defaultEnsembleExcludePrefixes = []string{
	// Agent internals
	"datadog.agent.",
	"datadog.trace_agent.",
	"datadog.dogstatsd.",
	"datadog.cluster_agent.",
	// Kubernetes platform
	"kube_apiserver.",
	"kube_controller_manager.",
	"kube_scheduler.",
	"kubernetes.",
	"kubelet.",
	// Container runtime
	"container.",
	"containerd.",
	"cri.",
	// System/OS infrastructure
	"system.disk.",
	"system.io.",
	"system.mem.",
	"system.net.",
	"system.swap.",
	// DNS/networking infrastructure
	"coredns.",
	"etcd.",
}

// EnsembleConfig holds configuration for the Ensemble detector.
type EnsembleConfig struct {
	// --- TopK ranking parameters ---

	// MinPoints is the minimum number of data points a series must have.
	MinPoints int
	// BaselineFraction is the fraction of the series to use as baseline (from the start).
	BaselineFraction float64
	// TopK is the maximum number of series to consider.
	TopK int
	// TopFraction is the fraction of series to consider (alternative to TopK, whichever is smaller).
	TopFraction float64
	// MinSeverityMADs is the minimum MAD-normalized change to be considered by TopK ranking.
	MinSeverityMADs float64
	// ExcludePrefixes is a list of metric name prefixes to exclude (infrastructure filtering).
	ExcludePrefixes []string

	// --- Mann-Whitney timestamp refinement parameters ---
	// MW is used for timestamp precision only — all TopK-selected metrics are emitted
	// regardless of MW result. MW provides the precise changepoint timestamp when it
	// can find one; otherwise the TopK heuristic timestamp is used as fallback.

	// MWWindowSize is the number of points in each half-window for the MW U test.
	MWWindowSize int
	// MWSignificanceThreshold is the p-value below which the MW test considers a changepoint significant.
	MWSignificanceThreshold float64
	// MWMinEffectSize is the minimum |rank-biserial correlation| required.
	MWMinEffectSize float64
	// MWMinDeviationSigma is the minimum |median_after - median_before| / MAD_before.
	MWMinDeviationSigma float64
	// MWMinRelativeChange is the minimum |mean_after - mean_before| / |mean_before|.
	MWMinRelativeChange float64
	// MWStepSize controls how many points to skip between candidate splits.
	MWStepSize int
}

// DefaultEnsembleConfig returns principled defaults for the ensemble detector.
func DefaultEnsembleConfig() EnsembleConfig {
	return EnsembleConfig{
		// TopK parameters — same as standalone TopK
		MinPoints:        30,
		BaselineFraction: 0.25,
		TopK:             20,
		TopFraction:      0.02,
		MinSeverityMADs:  3.0,
		ExcludePrefixes:  defaultEnsembleExcludePrefixes,

		// MW parameters — very relaxed because TopK already did metric selection.
		// MW is used purely for timestamp refinement. Low thresholds ensure most
		// TopK-selected metrics get MW-refined timestamps.
		MWWindowSize:            60,
		MWSignificanceThreshold: 1e-4,
		MWMinEffectSize:         0.50,
		MWMinDeviationSigma:     2.0,
		MWMinRelativeChange:     0.05,
		MWStepSize:              3,
	}
}

// EnsembleDetector combines TopK severity ranking (metric selection) with
// Mann-Whitney U test (timestamp precision) into a single MultiSeriesDetector.
//
// Algorithm:
//  1. Score all series by TopK severity (|post_median - pre_median| / MAD)
//  2. Select top-K most changed metrics (with infrastructure filtering)
//  3. For each selected metric, run Mann-Whitney sliding window to find precise changepoint
//  4. All top-K metrics are emitted — MW provides timestamp refinement, not validation
//     If MW finds a significant changepoint, use its precise timestamp.
//     Otherwise, fall back to TopK's heuristic changepoint timestamp.
//
// This gives TopK's metric selectivity + MW's timestamp precision where available.
type EnsembleDetector struct {
	config EnsembleConfig
	fired  map[string]bool
}

// NewEnsembleDetector creates a new Ensemble detector with default config.
func NewEnsembleDetector() *EnsembleDetector {
	return NewEnsembleDetectorWithConfig(DefaultEnsembleConfig())
}

// NewEnsembleDetectorWithConfig creates a new Ensemble detector with the given config.
func NewEnsembleDetectorWithConfig(config EnsembleConfig) *EnsembleDetector {
	return &EnsembleDetector{
		config: config,
		fired:  make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *EnsembleDetector) Name() string {
	return "ensemble"
}

// ensembleCandidate holds a TopK-selected series with its severity score and series data.
type ensembleCandidate struct {
	key          observer.SeriesKey
	metricID     string  // "service:metricName" format
	label        string  // human-readable
	severity     float64 // TopK score (MAD-normalized change)
	preMedian    float64
	postMedian   float64
	preMAD       float64
	topkChangePt int64            // heuristic changepoint from TopK scoring
	series       *observer.Series // full series data for MW analysis
}

// Detect implements MultiSeriesDetector.
func (d *EnsembleDetector) Detect(storage observer.StorageReader, dataTime int64) observer.MultiSeriesDetectionResult {
	// Phase 1: TopK severity ranking to select candidate metrics
	allKeys := storage.ListSeries(observer.SeriesFilter{})

	var candidates []ensembleCandidate
	excluded := 0
	for _, key := range allKeys {
		// Skip internal namespaces
		if key.Namespace == "observer" || key.Namespace == "internal" || key.Namespace == "telemetry" {
			continue
		}

		// Skip infrastructure metric families
		if d.isExcluded(key.Name) {
			excluded++
			continue
		}

		pc := storage.PointCount(key)
		if pc < d.config.MinPoints {
			continue
		}

		series := storage.GetSeriesRange(key, 0, dataTime, observer.AggregateAverage)
		if series == nil || len(series.Points) < d.config.MinPoints {
			continue
		}

		// Score using TopK severity metric
		scored := d.scoreSeverity(series)
		if scored == nil {
			continue
		}

		candidates = append(candidates, ensembleCandidate{
			key:          key,
			metricID:     topKMetricID(key),
			label:        topKSeriesLabel(key),
			severity:     scored.severity,
			preMedian:    scored.preMedian,
			postMedian:   scored.postMedian,
			preMAD:       scored.preMAD,
			topkChangePt: scored.changePt,
			series:       series,
		})
	}

	if len(candidates) == 0 {
		return observer.MultiSeriesDetectionResult{}
	}

	// Rank by severity descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].severity > candidates[j].severity
	})

	// Select top-K
	k := d.config.TopK
	fractionK := int(math.Ceil(float64(len(candidates)) * d.config.TopFraction))
	if fractionK < k {
		k = fractionK
	}
	if k > len(candidates) {
		k = len(candidates)
	}

	// Only include candidates above minimum severity threshold
	var selected []ensembleCandidate
	for i := 0; i < k; i++ {
		if candidates[i].severity >= d.config.MinSeverityMADs {
			selected = append(selected, candidates[i])
		}
	}

	log.Printf("  Ensemble: scored %d series, top-K selected %d (of %d candidates), total keys=%d, excluded=%d",
		len(candidates), len(selected), k, len(allKeys), excluded)

	// Phase 2: For each selected metric, use MW to find precise changepoint timestamp.
	// All selected metrics are emitted regardless of MW result — MW only provides
	// timestamp refinement. If MW can't find a significant changepoint, we fall back
	// to the TopK heuristic timestamp.
	var anomalies []observer.Anomaly
	mwRefined := 0
	mwFallback := 0

	for _, c := range selected {
		fireKey := c.metricID
		if d.fired[fireKey] {
			continue
		}
		d.fired[fireKey] = true

		// Try MW for timestamp refinement
		mwResult := d.runMannWhitney(c.series)

		score := c.severity
		sourceSeriesID := observer.SeriesID(
			c.key.Namespace + "|" + c.key.Name + "|" + strings.Join(c.key.Tags, ","))

		var timestamp int64
		var desc string
		if mwResult != nil {
			// MW found a precise changepoint
			mwRefined++
			timestamp = mwResult.timestamp
			desc = fmt.Sprintf("Ensemble (TopK+MW): %s severity=%.2f (pre=%.4f, post=%.4f, MAD=%.4f), MW p=%.2e effect=%.2f dev=%.1fσ relΔ=%.1f%%",
				c.label, c.severity, c.preMedian, c.postMedian, c.preMAD,
				mwResult.pValue, mwResult.effectSize, mwResult.deviation, mwResult.relChange*100)
		} else {
			// Fall back to TopK heuristic timestamp
			mwFallback++
			timestamp = c.topkChangePt
			desc = fmt.Sprintf("Ensemble (TopK fallback): %s severity=%.2f (pre=%.4f, post=%.4f, MAD=%.4f)",
				c.label, c.severity, c.preMedian, c.postMedian, c.preMAD)
		}

		anomaly := observer.Anomaly{
			Type:           observer.AnomalyTypeMetric,
			Source:         observer.MetricName(c.metricID),
			SourceSeriesID: sourceSeriesID,
			DetectorName:   d.Name(),
			Title:          "Ensemble severity anomaly",
			Description:    desc,
			Tags:           c.key.Tags,
			Timestamp:      timestamp,
			Score:          &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineMedian: c.preMedian,
				BaselineMAD:    c.preMAD,
				CurrentValue:   c.postMedian,
				DeviationSigma: c.severity,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	log.Printf("  Ensemble: MW refined %d timestamps, %d used TopK fallback, emitting %d anomalies",
		mwRefined, mwFallback, len(anomalies))

	return observer.MultiSeriesDetectionResult{
		Anomalies: anomalies,
	}
}

// isExcluded checks whether a metric name matches any configured exclude prefix.
func (d *EnsembleDetector) isExcluded(name string) bool {
	for _, prefix := range d.config.ExcludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ensembleSeverity holds TopK severity scoring results.
type ensembleSeverity struct {
	severity   float64
	preMedian  float64
	postMedian float64
	preMAD     float64
	changePt   int64 // heuristic changepoint timestamp
}

// scoreSeverity computes the TopK-style severity score for a series.
// Returns nil if the series has no significant change.
func (d *EnsembleDetector) scoreSeverity(series *observer.Series) *ensembleSeverity {
	pts := series.Points
	n := len(pts)

	baselineEnd := int(float64(n) * d.config.BaselineFraction)
	if baselineEnd < 5 {
		baselineEnd = 5
	}
	if baselineEnd >= n-5 {
		return nil
	}

	values := make([]float64, n)
	for i, p := range pts {
		values[i] = p.Value
	}

	preMedian := topKMedian(values[:baselineEnd])
	preMAD := topKMAD(values[:baselineEnd], preMedian)
	postMedian := topKMedian(values[baselineEnd:])

	absChange := math.Abs(postMedian - preMedian)

	denominator := preMAD
	if denominator < 1e-10 {
		denominator = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}

	severity := absChange / denominator

	// Find heuristic changepoint (same as TopK: first 3 consecutive deviants)
	changePt := pts[baselineEnd].Timestamp
	consecutiveDeviant := 0
	threshold := preMAD * 2
	if threshold < 1e-10 {
		threshold = math.Abs(preMedian) * 0.05
	}
	for i := baselineEnd; i < n; i++ {
		if math.Abs(values[i]-preMedian) > threshold {
			consecutiveDeviant++
			if consecutiveDeviant >= 3 {
				changePt = pts[i-2].Timestamp
				break
			}
		} else {
			consecutiveDeviant = 0
		}
	}

	return &ensembleSeverity{
		severity:   severity,
		preMedian:  preMedian,
		postMedian: postMedian,
		preMAD:     preMAD,
		changePt:   changePt,
	}
}

// ensembleMWResult holds the output of a Mann-Whitney validation pass.
type ensembleMWResult struct {
	timestamp  int64
	pValue     float64
	effectSize float64
	deviation  float64
	relChange  float64
}

// runMannWhitney runs the Mann-Whitney U test on a series to find the precise changepoint.
// Returns nil if the series doesn't pass MW's statistical filters.
func (d *EnsembleDetector) runMannWhitney(series *observer.Series) *ensembleMWResult {
	n := len(series.Points)
	windowSize := d.config.MWWindowSize
	stepSize := d.config.MWStepSize

	// Adaptive window
	maxWindow := (n - 1) / 2
	if windowSize > maxWindow {
		windowSize = maxWindow
	}
	if windowSize < 10 {
		return nil
	}

	bestPValue := 1.0
	bestSplit := -1
	bestU := 0.0
	bestEffect := 0.0

	// Slide split point
	for t := windowSize; t <= n-windowSize; t += stepSize {
		before := extractValues(series.Points[t-windowSize : t])
		after := extractValues(series.Points[t : t+windowSize])

		u, pValue := mannWhitneyU(before, after)

		if pValue < bestPValue {
			effectSize := rankBiserialCorrelation(u, len(before), len(after))
			bestPValue = pValue
			bestSplit = t
			bestU = u
			bestEffect = effectSize
		}
	}
	_ = bestU

	// Filter 1: statistical significance
	if bestSplit < 0 || bestPValue >= d.config.MWSignificanceThreshold {
		return nil
	}

	// Filter 2: effect size
	if math.Abs(bestEffect) < d.config.MWMinEffectSize {
		return nil
	}

	// Compute robust stats around the best split
	beforeStart := bestSplit - windowSize
	if beforeStart < 0 {
		beforeStart = 0
	}
	beforeVals := extractValues(series.Points[beforeStart:bestSplit])
	afterVals := extractValues(series.Points[bestSplit : bestSplit+windowSize])

	beforeMedian := mwMedian(beforeVals)
	beforeMAD := mwMAD(beforeVals, beforeMedian)
	afterMedian := mwMedian(afterVals)

	beforeMean := mwMeanValues(beforeVals)
	afterMean := mwMeanValues(afterVals)

	// Filter 3: deviation check
	deviation := 0.0
	if beforeMAD > 1e-10 {
		deviation = (afterMedian - beforeMedian) / beforeMAD
	} else if math.Abs(beforeMedian) > 1e-10 {
		deviation = (afterMedian - beforeMedian) / (math.Abs(beforeMedian) * 0.05)
	}

	if math.Abs(deviation) < d.config.MWMinDeviationSigma {
		return nil
	}

	// Filter 4: relative change
	absBaseline := math.Abs(beforeMean)
	if absBaseline < 1e-6 {
		absBaseline = 1e-6
	}
	relChange := math.Abs(afterMean-beforeMean) / absBaseline
	if relChange < d.config.MWMinRelativeChange {
		return nil
	}

	return &ensembleMWResult{
		timestamp:  series.Points[bestSplit].Timestamp,
		pValue:     bestPValue,
		effectSize: bestEffect,
		deviation:  deviation,
		relChange:  relChange,
	}
}
