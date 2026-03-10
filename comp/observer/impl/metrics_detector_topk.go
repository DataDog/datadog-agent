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

// TopKConfig holds configuration for the TopK relative severity ranking detector.
type TopKConfig struct {
	// MinPoints is the minimum number of data points a series must have.
	MinPoints int
	// BaselineFraction is the fraction of the series to use as baseline (from the start).
	BaselineFraction float64
	// TopK is the maximum number of series to report as anomalies.
	TopK int
	// TopFraction is the fraction of series to report (alternative to TopK, whichever is smaller).
	TopFraction float64
	// MinAbsoluteChange is the minimum absolute change (post_median - pre_median) to be considered.
	MinAbsoluteChange float64
	// MinRelativeChange is the minimum relative change (|post_median - pre_median| / MAD) to be considered.
	MinRelativeChange float64
	// ExcludePrefixes is a list of metric name prefixes to exclude from ranking.
	// Infrastructure/platform metrics matching these prefixes are skipped so that
	// application-level metrics can bubble up to the top-K.
	ExcludePrefixes []string
	// TopPerService limits how many metrics per service can appear in the final selection.
	// 0 means no per-service limit (global ranking only).
	TopPerService int
}

// defaultTopKExcludePrefixes is intentionally empty. Prefix-based filtering was
// tested but changed the global ranking composition in ways that could regress
// either L2 or mRec depending on scenario. The service diversity bonus mechanism
// (Step 4b in Detect) ensures application services are represented without
// disturbing the global ranking that drives good L2 timestamp precision.
var defaultTopKExcludePrefixes []string

// DefaultTopKConfig returns sensible defaults.
func DefaultTopKConfig() TopKConfig {
	return TopKConfig{
		MinPoints:         30,
		BaselineFraction:  0.25,
		TopK:              20,
		TopFraction:       0.02, // top 2% of all series
		MinAbsoluteChange: 0.0,
		MinRelativeChange: 3.0, // must be at least 3 MADs away
		ExcludePrefixes:   defaultTopKExcludePrefixes,
		TopPerService:     1, // 1 per service for diversity (service-level matching)
	}
}

// TopKDetector implements MultiSeriesDetector using relative severity ranking.
// Instead of making independent binary anomaly decisions per series, it computes
// a change severity score for ALL series and only reports the top-K most anomalous.
// This directly attacks the precision problem identified in Phase 1.
type TopKDetector struct {
	config TopKConfig
	// fired tracks which series have already been reported to avoid duplicates across calls.
	fired map[string]bool
}

// NewTopKDetector creates a new TopK detector with default config.
func NewTopKDetector() *TopKDetector {
	return NewTopKDetectorWithConfig(DefaultTopKConfig())
}

// NewTopKDetectorWithConfig creates a new TopK detector with the given config.
func NewTopKDetectorWithConfig(config TopKConfig) *TopKDetector {
	return &TopKDetector{
		config: config,
		fired:  make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *TopKDetector) Name() string {
	return "topk"
}

// topKSeriesScore holds a series and its computed change severity score.
type topKSeriesScore struct {
	key        observer.SeriesKey
	metricID   string // "service:metricName" format for Source field
	label      string // human-readable label
	score      float64
	hasService bool // true if the series has a service tag (application-level metric)
	service    string
	// Details for anomaly description
	preMedian  float64
	postMedian float64
	preMAD     float64
	changePt   int64 // timestamp of the detected change point (split point)
}

// Detect implements MultiSeriesDetector.
// It scores all series by severity of change and only reports the top-K.
func (d *TopKDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	// Step 1: Discover all series
	allKeys := storage.ListSeries(observer.SeriesFilter{})

	// Step 2: Score each series by change severity
	var scored []topKSeriesScore
	excluded := 0
	for _, key := range allKeys {
		// Skip internal/observer/telemetry metrics
		if key.Namespace == "observer" || key.Namespace == "internal" || key.Namespace == "telemetry" {
			continue
		}

		// Skip infrastructure metric families that would drown out application signal
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

		result := d.scoreSeries(series)
		if result == nil {
			continue
		}

		result.key = key
		result.metricID = detectorMetricID(key)
		result.label = detectorSeriesLabel(key)
		result.service = detectorService(key)
		result.hasService = result.service != ""
		scored = append(scored, *result)
	}

	if len(scored) == 0 {
		return observer.DetectionResult{}
	}

	// Step 3: Rank by score descending (global ranking)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Step 4: Select top-K globally (preserves original L2-optimal behavior)
	k := d.config.TopK
	fractionK := int(math.Ceil(float64(len(scored)) * d.config.TopFraction))
	if fractionK < k {
		k = fractionK
	}
	if k > len(scored) {
		k = len(scored)
	}

	var selected []topKSeriesScore
	selectedSet := make(map[string]bool)
	for i := 0; i < k; i++ {
		if scored[i].score >= d.config.MinRelativeChange {
			selected = append(selected, scored[i])
			selectedSet[scored[i].metricID] = true
		}
	}
	globalSelected := len(selected)

	// Step 4b: Service diversity bonus. Add 1 metric per unique service that
	// wasn't already represented in the global top-K. This ensures the detector
	// output includes application services (important for metric recall via
	// service-level matching) without significantly increasing the total count.
	// Bonus metrics use the same timestamp detection as all metrics, so they
	// may cluster with the main group if the service was genuinely affected.
	perService := d.config.TopPerService
	if perService <= 0 {
		perService = 1
	}
	serviceCounts := make(map[string]int)
	// Count services already in global selection
	for _, s := range selected {
		if s.hasService {
			serviceCounts[s.service]++
		}
	}
	// Compute the median change point of the global selection, so bonus metrics
	// can adopt it. This ensures bonus metrics cluster with the main group in
	// the time_cluster correlator, preserving L2 timestamp precision.
	var globalChangePts []int64
	for _, s := range selected {
		globalChangePts = append(globalChangePts, s.changePt)
	}
	sort.Slice(globalChangePts, func(i, j int) bool {
		return globalChangePts[i] < globalChangePts[j]
	})
	var medianChangePt int64
	if n := len(globalChangePts); n > 0 {
		medianChangePt = globalChangePts[n/2]
	}

	// Cap total bonus to a small number to avoid inflating the anomaly count
	// excessively, which would degrade time_cluster precision (L2 F1).
	maxBonus := 3
	appBonus := 0
	for _, s := range scored {
		if s.score < d.config.MinRelativeChange {
			break
		}
		if !s.hasService || selectedSet[s.metricID] {
			continue
		}
		if serviceCounts[s.service] < perService {
			// Align bonus metric's timestamp with the global cluster median
			// so it doesn't create a separate time_cluster period.
			s.changePt = medianChangePt
			selected = append(selected, s)
			selectedSet[s.metricID] = true
			serviceCounts[s.service]++
			appBonus++
			if appBonus >= maxBonus {
				break
			}
		}
	}

	log.Printf("  TopK: scored %d series, selected %d (global=%d + svcBonus=%d, %d unique svcs), total keys=%d, excluded=%d",
		len(scored), len(selected), globalSelected, appBonus, len(serviceCounts), len(allKeys), excluded)

	// Step 5: Emit anomalies for selected series
	var anomalies []observer.Anomaly
	for _, s := range selected {
		fireKey := s.metricID
		if d.fired[fireKey] {
			continue
		}
		d.fired[fireKey] = true

		score := s.score
		sourceSeriesID := observer.SeriesID(
			s.key.Namespace + "|" + s.key.Name + "|" + strings.Join(s.key.Tags, ","))

		desc := fmt.Sprintf("TopK severity ranking: %s score=%.2f (pre_median=%.4f, post_median=%.4f, MAD=%.4f, change=%.4f)",
			s.label, s.score, s.preMedian, s.postMedian, s.preMAD, s.postMedian-s.preMedian)

		anomaly := observer.Anomaly{
			Type:           observer.AnomalyTypeMetric,
			Source:         observer.MetricName(s.metricID),
			SourceSeriesID: sourceSeriesID,
			DetectorName:   d.Name(),
			Title:          "TopK severity anomaly",
			Description:    desc,
			Tags:           s.key.Tags,
			Timestamp:      s.changePt,
			Score:          &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineMedian: s.preMedian,
				BaselineMAD:    s.preMAD,
				CurrentValue:   s.postMedian,
				DeviationSigma: s.score,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	log.Printf("  TopK: emitting %d anomalies (after dedup)", len(anomalies))

	return observer.DetectionResult{
		Anomalies: anomalies,
	}
}

// isExcluded checks whether a metric name matches any of the configured exclude prefixes.
func (d *TopKDetector) isExcluded(name string) bool {
	for _, prefix := range d.config.ExcludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// scoreSeries computes the change severity score for a single series.
// It finds the best split point that maximizes |post_median - pre_median| / MAD.
// Returns nil if the series has no significant change.
func (d *TopKDetector) scoreSeries(series *observer.Series) *topKSeriesScore {
	pts := series.Points
	n := len(pts)

	// Use a fixed baseline fraction approach first, then optionally scan for best split.
	// Fixed baseline: use the first BaselineFraction of points as baseline.
	baselineEnd := int(float64(n) * d.config.BaselineFraction)
	if baselineEnd < 5 {
		baselineEnd = 5
	}
	if baselineEnd >= n-5 {
		return nil // not enough points for both halves
	}

	// Extract values
	values := make([]float64, n)
	for i, p := range pts {
		values[i] = p.Value
	}

	// Compute baseline statistics (median and MAD)
	preMedian := detectorMedian(values[:baselineEnd])
	preMAD := detectorMAD(values[:baselineEnd], preMedian, false) // raw MAD as severity denominator

	// Scan post-baseline for the segment with the largest deviation.
	// Use a sliding approach: find the post-baseline median.
	postMedian := detectorMedian(values[baselineEnd:])

	absChange := math.Abs(postMedian - preMedian)

	// Check minimum absolute change
	if d.config.MinAbsoluteChange > 0 && absChange < d.config.MinAbsoluteChange {
		return nil
	}

	// Score = |post_median - pre_median| / MAD
	// If MAD is zero (constant baseline), use a small epsilon based on the absolute value.
	denominator := preMAD
	if denominator < 1e-10 {
		// For constant baselines, use 1% of the absolute median or a small constant.
		denominator = math.Max(math.Abs(preMedian)*0.01, 1e-6)
	}

	score := absChange / denominator

	// Find the approximate change point: scan for where the shift starts.
	// Use a simple approach: find the first point after baseline where
	// |value - preMedian| > preMAD * 2 consistently for 3+ consecutive points.
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
				changePt = pts[i-2].Timestamp // point where deviation started
				break
			}
		} else {
			consecutiveDeviant = 0
		}
	}

	return &topKSeriesScore{
		score:      score,
		preMedian:  preMedian,
		postMedian: postMedian,
		preMAD:     preMAD,
		changePt:   changePt,
	}
}
