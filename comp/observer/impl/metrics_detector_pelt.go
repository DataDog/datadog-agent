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

// PELTDetector uses the Pruned Exact Linear Time (PELT) algorithm to find
// optimal changepoints in a time series by minimizing a penalized cost function,
// then ranks all candidates globally and emits only the top-K most severe.
//
// Reference: Killick, Fearnhead & Eckley (2012), "Optimal Detection of
// Changepoints with a Linear Computational Cost", JASA 107(500).
//
// Algorithm:
//
//	F(t) = min over s in R(t) of { F(s) + C(y_{s+1:t}) + beta }
//	where C is the segment cost (negative log-likelihood under normal model)
//	and beta is the per-changepoint penalty (BIC: log(n)).
//	PELT prunes: discard s from R(t) if F(s) + C(y_{s+1:t}) > F(t).
//
// The cost function assumes a normal distribution with unknown mean:
//
//	C(y_{a:b}) = (b-a) * log(variance(y_{a:b}))
//
// This detector finds WHERE level shifts occur without assuming a fixed baseline.
// It implements MultiSeriesDetector to enable global ranking across all series.
type PELTDetector struct {
	// MinPoints is the minimum number of points required for analysis.
	MinPoints int

	// MinSegmentLen is the minimum number of points in a segment.
	// Prevents detecting changepoints in very short windows.
	MinSegmentLen int

	// PenaltyFactor scales the BIC penalty (log(n) * PenaltyFactor).
	// Higher values = fewer changepoints (more conservative).
	// Default: 5.0
	PenaltyFactor float64

	// BaselineFraction is the fraction of points to use for baseline estimation.
	// The baseline provides a stable reference for computing severity.
	// Default: 0.25 (first 25% of data)
	BaselineFraction float64

	// MinRelativeChange is the minimum relative change in mean
	// (|post_mean - baseline_mean| / max(|baseline_mean|, epsilon)) to report.
	// This filters out metrics with tiny absolute shifts that happen to have
	// high sigma due to near-constant baselines.
	// Default: 0.10 (10% change)
	MinRelativeChange float64

	// TopK is the maximum number of series to report as anomalies.
	// Default: 30
	TopK int

	// TopFraction is the fraction of scored series to report (alternative to TopK, whichever is smaller).
	// Default: 0.02 (top 2%)
	TopFraction float64

	// ExcludePrefixes is a list of metric name prefixes to exclude from ranking.
	// Infrastructure/platform metrics matching these prefixes are skipped.
	ExcludePrefixes []string

	// fired tracks which series have already been reported to avoid duplicates across calls.
	fired map[string]bool
}

// NewPELTDetector creates a PELTDetector with default settings.
func NewPELTDetector() *PELTDetector {
	return &PELTDetector{
		MinPoints:         10,
		MinSegmentLen:     5,
		PenaltyFactor:     5.0,
		BaselineFraction:  0.25,
		MinRelativeChange: 0.10,
		TopK:              30,
		TopFraction:       0.02,
		ExcludePrefixes:   defaultPELTExcludePrefixes,
		fired:             make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *PELTDetector) Name() string {
	return "pelt_detector"
}

// defaultPELTExcludePrefixes lists infrastructure/platform metric families that
// are excluded from PELT ranking. This is a superset of the TopK exclude list,
// adding control-plane and agent infrastructure metrics that dominate severity
// rankings due to high volatility unrelated to application behavior.
var defaultPELTExcludePrefixes = []string{
	// Same as TopK
	"datadog.agent.",
	"kube_apiserver.",
	"kubernetes.",
	"kubelet.",
	"container.",
	"containerd.",
	"cri.",
	"system.disk.",
	"system.io.",
	// Additional: control plane and cluster infrastructure
	"coredns.",
	"etcd.",
	"datadog.cluster_agent.",
	"datadog.dogstatsd.",
	"datadog.trace_agent.",
	"datadog.process.",
	"kube_controller_manager.",
	"kube_scheduler.",
	// Network metrics: high-volume counters that change with any traffic pattern
	"system.net.",
}

// peltCandidate holds the result of running PELT on a single series.
type peltCandidate struct {
	key            observer.SeriesKey
	metricID       string // "service:metricName:agg" format for Source field
	label          string
	severity       float64
	timestamp      int64
	baselineMean   float64
	baselineStd    float64
	baselineStart  int64
	baselineEnd    int64
	postMean       float64
	deviationSigma float64
	penalty        float64
	tags           []string
}

// Detect implements MultiSeriesDetector.
// It runs PELT on every series, collects all candidates with severity scores,
// ranks globally by severity, and emits only the top-K.
func (d *PELTDetector) Detect(storage observer.StorageReader, dataTime int64) observer.MultiSeriesDetectionResult {
	minPoints := d.MinPoints
	if minPoints <= 0 {
		minPoints = 10
	}
	minSegLen := d.MinSegmentLen
	if minSegLen <= 0 {
		minSegLen = 5
	}
	penaltyFactor := d.PenaltyFactor
	if penaltyFactor <= 0 {
		penaltyFactor = 5.0
	}
	baselineFrac := d.BaselineFraction
	if baselineFrac <= 0 {
		baselineFrac = 0.25
	}
	minRelChange := d.MinRelativeChange
	if minRelChange <= 0 {
		minRelChange = 0.10
	}
	topK := d.TopK
	if topK <= 0 {
		topK = 30
	}
	topFraction := d.TopFraction
	if topFraction <= 0 {
		topFraction = 0.02
	}

	// Step 1: Discover all series
	allKeys := storage.ListSeries(observer.SeriesFilter{})

	// Step 2: Run PELT on each series, collect candidates
	var candidates []peltCandidate
	excluded := 0
	for _, key := range allKeys {
		// Skip internal/observer/telemetry metrics
		if key.Namespace == "observer" || key.Namespace == "internal" || key.Namespace == "telemetry" {
			continue
		}

		// Skip infrastructure metric families
		if d.isExcluded(key.Name) {
			excluded++
			continue
		}

		pc := storage.PointCount(key)
		if pc < minPoints {
			continue
		}

		// Run on avg aggregation (primary signal)
		series := storage.GetSeriesRange(key, 0, dataTime, observer.AggregateAverage)
		if series == nil || len(series.Points) < minPoints {
			continue
		}

		candidate := d.scoreSeries(series, key, minPoints, minSegLen, penaltyFactor, baselineFrac, minRelChange)
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}

	if len(candidates) == 0 {
		return observer.MultiSeriesDetectionResult{}
	}

	// Step 3: Rank by severity descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].severity > candidates[j].severity
	})

	// Step 4: Select top-K.
	// Use TopK as the cap, but also bound by TopFraction of total series (not just scored)
	// to avoid over-reporting when many series have changepoints.
	k := topK
	if k > len(candidates) {
		k = len(candidates)
	}

	selected := candidates[:k]

	log.Printf("  PELT: scored %d series, selected top-%d (of %d candidates), total keys=%d, excluded=%d",
		len(candidates), len(selected), len(candidates), len(allKeys), excluded)
	// Log top-5 for operational visibility
	debugN := 5
	if debugN > len(candidates) {
		debugN = len(candidates)
	}
	for i := 0; i < debugN; i++ {
		c := candidates[i]
		log.Printf("  PELT[%d]: %s severity=%.1f ts=%d", i, c.metricID, c.severity, c.timestamp)
	}

	// Step 5: Emit anomalies for selected series
	var anomalies []observer.Anomaly
	for _, c := range selected {
		if d.fired[c.metricID] {
			continue
		}
		d.fired[c.metricID] = true

		score := c.severity
		sourceSeriesID := observer.SeriesID(
			c.key.Namespace + "|" + c.key.Name + ":avg|" + strings.Join(c.key.Tags, ","))

		direction := "above"
		devSigma := c.deviationSigma
		if c.postMean < c.baselineMean {
			direction = "below"
			devSigma = -c.deviationSigma
		}

		anomaly := observer.Anomaly{
			Type:           observer.AnomalyTypeMetric,
			Source:         observer.MetricName(c.metricID),
			SourceSeriesID: sourceSeriesID,
			DetectorName:   d.Name(),
			Title:          fmt.Sprintf("PELT changepoint: %s", c.label),
			Description: fmt.Sprintf("PELT ranked changepoint: %s severity=%.1f (%.2f -> %.2f, %.1f sigma %s baseline)",
				c.label, c.severity, c.baselineMean, c.postMean, c.severity, direction),
			Tags:      c.tags,
			Timestamp: c.timestamp,
			Score:     &score,
			DebugInfo: &observer.AnomalyDebugInfo{
				BaselineStart:  c.baselineStart,
				BaselineEnd:    c.baselineEnd,
				BaselineMean:   c.baselineMean,
				BaselineStddev: c.baselineStd,
				Threshold:      c.penalty,
				CurrentValue:   c.postMean,
				DeviationSigma: devSigma,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	log.Printf("  PELT: emitting %d anomalies (after dedup)", len(anomalies))

	return observer.MultiSeriesDetectionResult{
		Anomalies: anomalies,
	}
}

// isExcluded checks whether a metric name matches any of the configured exclude prefixes.
func (d *PELTDetector) isExcluded(name string) bool {
	for _, prefix := range d.ExcludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// scoreSeries runs PELT on a single series and returns a candidate if a significant
// changepoint is found. Returns nil if no qualifying changepoint exists.
// Unlike the threshold-based approach, this does NOT filter by MinSeverity —
// all changepoints that pass MinRelativeChange are candidates for global ranking.
func (d *PELTDetector) scoreSeries(
	series *observer.Series,
	key observer.SeriesKey,
	minPoints, minSegLen int,
	penaltyFactor, baselineFrac, minRelChange float64,
) *peltCandidate {
	n := len(series.Points)
	if n < minPoints {
		return nil
	}

	// Extract values
	values := make([]float64, n)
	for i, p := range series.Points {
		values[i] = p.Value
	}

	// Compute stable baseline from first portion of data
	baselineEnd := int(float64(n) * baselineFrac)
	if baselineEnd < 2 {
		baselineEnd = 2
	}
	if baselineEnd >= n {
		baselineEnd = n - 1
	}
	baselineValues := values[:baselineEnd]
	baselineMean := meanValues(baselineValues)
	baselineStd := stddevValues(baselineValues, baselineMean)

	// Handle constant/near-constant baseline
	const epsilon = 1e-10
	if baselineStd < epsilon {
		if math.Abs(baselineMean) > epsilon {
			baselineStd = math.Abs(baselineMean) * 0.1
		} else {
			return nil
		}
	}

	// BIC penalty
	penalty := math.Log(float64(n)) * penaltyFactor

	// Run PELT
	changepoints := pelt(values, penalty, minSegLen)
	if len(changepoints) == 0 {
		return nil
	}

	// Find the FIRST qualifying changepoint after the baseline period.
	// Using the earliest changepoint gives the best timestamp alignment with
	// disruption onset (same as original PELT). The severity of this first
	// changepoint is used for global ranking across all series.
	var firstSeverity float64
	var firstIdx int
	var firstPostMean float64
	found := false

	for _, cp := range changepoints {
		if cp < baselineEnd {
			continue
		}

		postValues := values[cp:]
		if len(postValues) < 2 {
			continue
		}
		postMean := meanValues(postValues)

		severity := math.Abs(postMean-baselineMean) / baselineStd

		// Require minimum relative change
		absChange := math.Abs(postMean - baselineMean)
		refScale := math.Max(math.Abs(baselineMean), epsilon)
		relChange := absChange / refScale
		if relChange < minRelChange {
			continue
		}

		// Take the first qualifying changepoint (earliest detection)
		firstSeverity = severity
		firstIdx = cp
		firstPostMean = postMean
		found = true
		break
	}

	if !found {
		return nil
	}

	metricID := peltMetricID(key)
	label := peltSeriesLabel(key)

	return &peltCandidate{
		key:            key,
		metricID:       metricID,
		label:          label,
		severity:       firstSeverity,
		timestamp:      series.Points[firstIdx].Timestamp,
		baselineMean:   baselineMean,
		baselineStd:    baselineStd,
		baselineStart:  series.Points[0].Timestamp,
		baselineEnd:    series.Points[baselineEnd-1].Timestamp,
		postMean:       firstPostMean,
		deviationSigma: firstSeverity,
		penalty:        penalty,
		tags:           key.Tags,
	}
}

// pelt implements the Pruned Exact Linear Time changepoint detection algorithm.
// Returns a sorted list of changepoint indices into the values slice.
func pelt(values []float64, penalty float64, minSegLen int) []int {
	n := len(values)
	if n < 2*minSegLen {
		return nil
	}

	// Precompute cumulative sums for O(1) segment cost
	cumSum := make([]float64, n+1)
	cumSumSq := make([]float64, n+1)
	for i, v := range values {
		cumSum[i+1] = cumSum[i] + v
		cumSumSq[i+1] = cumSumSq[i] + v*v
	}

	// segmentCost computes the cost of segment [start, end) under normal model.
	// C = n * log(variance) where variance = (sumSq/n - mean^2)
	// Returns +Inf for degenerate segments (zero variance).
	segmentCost := func(start, end int) float64 {
		segLen := end - start
		if segLen < 2 {
			return 0
		}
		s := cumSum[end] - cumSum[start]
		ss := cumSumSq[end] - cumSumSq[start]
		nf := float64(segLen)
		mean := s / nf
		variance := ss/nf - mean*mean
		if variance <= 0 {
			return 0 // constant segment, perfect fit
		}
		return nf * math.Log(variance)
	}

	// F[t] = optimal cost of segmenting values[0:t]
	// lastCP[t] = last changepoint before t in optimal segmentation
	inf := math.Inf(1)
	F := make([]float64, n+1)
	lastCP := make([]int, n+1)
	F[0] = -penalty // offset so first segment doesn't pay double penalty

	for i := 1; i <= n; i++ {
		F[i] = inf
		lastCP[i] = 0
	}

	// R is the set of candidate changepoint positions (pruned)
	R := []int{0}

	for tauStar := 2 * minSegLen; tauStar <= n; tauStar++ {
		// Find optimal last changepoint for values[0:tauStar]
		bestCost := inf
		bestS := 0

		var newR []int
		for _, s := range R {
			// Segment [s, tauStar) must have at least minSegLen points
			if tauStar-s < minSegLen {
				newR = append(newR, s) // keep for future consideration
				continue
			}

			cost := F[s] + segmentCost(s, tauStar) + penalty
			if cost < bestCost {
				bestCost = cost
				bestS = s
			}

			// PELT pruning: keep s only if it could still be optimal in future
			// Prune if F[s] + C(s, tauStar) > F[tauStar] (after we set F[tauStar])
			// We'll prune after updating F[tauStar]
			newR = append(newR, s)
		}

		if bestCost < F[tauStar] {
			F[tauStar] = bestCost
			lastCP[tauStar] = bestS
		}

		// Prune: remove candidates that can never be optimal
		pruned := make([]int, 0, len(newR))
		for _, s := range newR {
			if tauStar-s < minSegLen {
				pruned = append(pruned, s)
				continue
			}
			if F[s]+segmentCost(s, tauStar) <= F[tauStar] {
				pruned = append(pruned, s)
			}
		}
		// Add tauStar as a new candidate
		pruned = append(pruned, tauStar)
		R = pruned
	}

	// Backtrack to find changepoints
	var changepoints []int
	idx := n
	for idx > 0 {
		cp := lastCP[idx]
		if cp > 0 {
			changepoints = append(changepoints, cp)
		}
		idx = cp
	}

	// Reverse to get chronological order
	for i, j := 0, len(changepoints)-1; i < j; i, j = i+1, j-1 {
		changepoints[i], changepoints[j] = changepoints[j], changepoints[i]
	}

	return changepoints
}

// peltSeriesLabel builds a human-readable label from a SeriesKey.
func peltSeriesLabel(key observer.SeriesKey) string {
	service := ""
	for _, tag := range key.Tags {
		if strings.HasPrefix(tag, "service:") {
			service = tag[len("service:"):]
			break
		}
	}
	if service != "" {
		return service + "/" + key.Name
	}
	if key.Namespace != "" {
		return key.Namespace + "/" + key.Name
	}
	return key.Name
}

// peltMetricID builds a metric identifier matching the scorer's expected format: "service:metricName:avg".
func peltMetricID(key observer.SeriesKey) string {
	service := ""
	for _, tag := range key.Tags {
		if strings.HasPrefix(tag, "service:") {
			service = tag[len("service:"):]
			break
		}
	}
	if service != "" {
		return service + ":" + key.Name + ":avg"
	}
	return key.Name + ":avg"
}

// meanValues computes the mean of a float64 slice.
func meanValues(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

// stddevValues computes the sample standard deviation of a float64 slice.
func stddevValues(v []float64, mean float64) float64 {
	n := len(v)
	if n < 2 {
		return 0
	}
	var sumSq float64
	for _, x := range v {
		d := x - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n-1))
}

// medianValues and madValues are defined in metrics_detector_edivisive.go
