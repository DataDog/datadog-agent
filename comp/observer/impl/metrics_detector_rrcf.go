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

// RRCFScoredPoint records a CoDisp score at a specific timestamp.
type RRCFScoredPoint struct {
	Timestamp int64   `json:"timestamp"`
	Score     float64 `json:"score"`
}

// RRCFScoreStats contains distribution statistics and full score history for threshold analysis.
type RRCFScoreStats struct {
	Enabled       bool              `json:"enabled"`
	SampleCount   int               `json:"sampleCount"`
	AlignedPoints int               `json:"alignedPoints"`
	ShinglesBuilt int               `json:"shinglesBuilt"`
	MinScore      float64           `json:"minScore"`
	MaxScore      float64           `json:"maxScore"`
	MeanScore     float64           `json:"meanScore"`
	StddevScore   float64           `json:"stddevScore"`
	P50           float64           `json:"p50"`
	P75           float64           `json:"p75"`
	P90           float64           `json:"p90"`
	P95           float64           `json:"p95"`
	P99           float64           `json:"p99"`
	Config        RRCFConfigSummary `json:"config"`
	Metrics       []string          `json:"metrics"`
	Scores        []RRCFScoredPoint `json:"scores"`
}

// RRCFConfigSummary is a JSON-friendly summary of RRCF configuration.
type RRCFConfigSummary struct {
	NumTrees       int     `json:"numTrees"`
	TreeSize       int     `json:"treeSize"`
	ShingleSize    int     `json:"shingleSize"`
	ShingleDim     int     `json:"shingleDim"`
	ThresholdSigma float64 `json:"thresholdSigma"`
}

// RRCFMetricDef defines a metric to include in the RRCF analysis.
type RRCFMetricDef struct {
	Namespace string
	Name      string
	Agg       observer.Aggregate
}

// RRCFConfig holds configuration for the RRCF analysis.
type RRCFConfig struct {
	// NumTrees is the number of trees in the forest. More trees = more robust but slower.
	NumTrees int
	// TreeSize is the maximum number of points per tree (sliding window size).
	TreeSize int
	// ShingleSize is the number of consecutive timestamps to combine into one point.
	// ShingleSize=4 means each "point" is 4 consecutive samples, enabling temporal pattern detection.
	ShingleSize int
	// ThresholdSigma controls dynamic anomaly thresholding. A point is flagged if its
	// CoDisp score exceeds mean + ThresholdSigma*stddev of the recent score window.
	// Set to 0 to disable anomaly detection (scores still computed for analysis).
	ThresholdSigma float64
	// Metrics defines which series to include. If nil, uses DefaultRRCFMetrics().
	Metrics []RRCFMetricDef
}

// DefaultRRCFConfig returns sensible defaults for RRCF.
func DefaultRRCFConfig() RRCFConfig {
	return RRCFConfig{
		NumTrees:       100,
		TreeSize:       256,
		ShingleSize:    4,
		ThresholdSigma: 3.0,
	}
}

// DefaultRRCFMetrics returns the default metric set for RRCF in the live observer.
// These are standard Datadog system metrics from DogStatsD.
func DefaultRRCFMetrics() []RRCFMetricDef {
	return []RRCFMetricDef{
		{Namespace: "system", Name: "cpu.user", Agg: observer.AggregateAverage},
		{Namespace: "system", Name: "cpu.system", Agg: observer.AggregateAverage},
		{Namespace: "system", Name: "cpu.iowait", Agg: observer.AggregateAverage},
		{Namespace: "system", Name: "memory.used", Agg: observer.AggregateAverage},
		{Namespace: "system", Name: "memory.rss", Agg: observer.AggregateAverage},
		{Namespace: "system", Name: "disk.read_bytes", Agg: observer.AggregateSum},
		{Namespace: "system", Name: "disk.write_bytes", Agg: observer.AggregateSum},
	}
}

// TestBenchRRCFMetrics returns a metric set for RRCF matching cgroup.v2 data
// from FGM parquet exports (namespace "parquet").
func TestBenchRRCFMetrics() []RRCFMetricDef {
	return []RRCFMetricDef{
		// CPU
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.user_usec", Agg: observer.AggregateAverage},
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.system_usec", Agg: observer.AggregateAverage},
		{Namespace: "parquet", Name: "cgroup.v2.cpu.pressure.some.avg10", Agg: observer.AggregateAverage},
		// Memory
		{Namespace: "parquet", Name: "cgroup.v2.memory.current", Agg: observer.AggregateAverage},
		{Namespace: "parquet", Name: "smaps_rollup.rss", Agg: observer.AggregateAverage},
		// IO
		{Namespace: "parquet", Name: "cgroup.v2.io.stat.rbytes", Agg: observer.AggregateAverage},
		{Namespace: "parquet", Name: "cgroup.v2.io.stat.wbytes", Agg: observer.AggregateAverage},
	}
}

// RRCFDetector implements multivariate anomaly detection using Robust Random Cut Forest.
// It queries multiple system metrics and detects unusual combinations/trajectories.
type RRCFDetector struct {
	config RRCFConfig

	// metrics defines which series to include in the multivariate analysis.
	// Each metric becomes a dimension in the feature vector.
	metrics []RRCFMetricDef

	// resolvedKeys caches the numeric series ID for each metric.
	// Populated lazily on first Detect call via ListSeries discovery.
	resolvedKeys map[string]observer.SeriesHandle

	// cursors tracks read position per metric for incremental reads.
	cursors map[string]int64

	// forest is the RRCF forest structure.
	forest *rcForest

	// recentScores tracks recent CoDisp scores for dynamic thresholding.
	// Only populated after warmup (first TreeSize points are skipped).
	recentScores []float64

	// totalScored counts total shingles scored (including warmup).
	totalScored int

	// allScores tracks every score with its timestamp for offline threshold analysis.
	allScores []RRCFScoredPoint

	// alignedCount and shingleCount track pipeline throughput for diagnostics.
	alignedCount int
	shingleCount int
}

// NewRRCFDetector creates an RRCF detector with the given config.
func NewRRCFDetector(config RRCFConfig) *RRCFDetector {
	metrics := config.Metrics
	if len(metrics) == 0 {
		metrics = DefaultRRCFMetrics()
	}

	// Compute shingle dimension: numMetrics * shingleSize
	numMetrics := len(metrics)
	shingleDim := numMetrics * config.ShingleSize

	// Create forest with fixed seed for reproducibility (can be made configurable)
	forest := newRCForest(config.NumTrees, config.TreeSize, shingleDim, 42)

	return &RRCFDetector{
		config:       config,
		metrics:      metrics,
		resolvedKeys: make(map[string]observer.SeriesHandle),
		cursors:      make(map[string]int64),
		forest:       forest,
		recentScores: make([]float64, 0, 100),
		allScores:    make([]RRCFScoredPoint, 0, 1024),
	}
}

// Name returns the detector name.
func (r *RRCFDetector) Name() string {
	return "rrcf"
}

// Detect implements Detector. It queries storage for system metrics,
// builds multivariate shingles, and detects anomalies using RRCF.
func (r *RRCFDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	// Step 0: Resolve all metric keys to the same tag set (on first call)
	if !r.resolveAllKeys(storage) {
		return observer.DetectionResult{}
	}

	// Step 1: Read new points for each metric since last cursor
	newPointsByMetric := r.readNewPoints(storage, dataTime)
	if len(newPointsByMetric) == 0 {
		return observer.DetectionResult{}
	}

	// Step 2: Align points by timestamp and build multivariate vectors
	alignedPoints := r.alignByTimestamp(newPointsByMetric)
	if len(alignedPoints) == 0 {
		return observer.DetectionResult{}
	}

	r.alignedCount += len(alignedPoints)

	// Step 3: Build shingles from aligned points
	shingles := r.buildShingles(alignedPoints)
	if len(shingles) == 0 {
		return observer.DetectionResult{}
	}

	r.shingleCount += len(shingles)

	// Step 4: Score shingles with RRCF and detect anomalies
	return r.scoreAndDetect(shingles, dataTime)
}

// resolveKey returns the cached numeric series ID for a metric definition.
// Keys are populated by resolveAllKeys on the first Detect call.
func (r *RRCFDetector) resolveKey(m RRCFMetricDef) (observer.SeriesHandle, bool) {
	cursorKey := m.Namespace + "|" + m.Name
	id, ok := r.resolvedKeys[cursorKey]
	return id, ok
}

// resolveAllKeys discovers series keys for all metrics at once, ensuring they share
// the same tag set (e.g., same container_id). This is necessary because data from
// parquet exports has per-container tags, and alignment only works if all metrics
// come from the same container.
func (r *RRCFDetector) resolveAllKeys(storage observer.StorageReader) bool {
	if len(r.resolvedKeys) > 0 {
		return true // already resolved
	}

	// For each metric, collect all matching series grouped by tag string
	seriesByMetric := make(map[string][]observer.SeriesMeta) // cursorKey -> all matching series
	for _, m := range r.metrics {
		cursorKey := m.Namespace + "|" + m.Name
		matches := storage.ListSeries(observer.SeriesFilter{
			Namespace:   m.Namespace,
			NamePattern: m.Name,
		})
		for _, meta := range matches {
			if meta.Name == m.Name {
				seriesByMetric[cursorKey] = append(seriesByMetric[cursorKey], meta)
			}
		}
	}

	// Build a tag signature for each series
	tagSig := func(tags []string) string {
		sorted := make([]string, len(tags))
		copy(sorted, tags)
		sort.Strings(sorted)
		return strings.Join(sorted, ",")
	}

	// Group series by tag signature and find a tag set that has ALL metrics
	tagSetMetrics := make(map[string]map[string]observer.SeriesMeta) // tagSig -> cursorKey -> SeriesMeta
	for cursorKey, metas := range seriesByMetric {
		for _, meta := range metas {
			sig := tagSig(meta.Tags)
			if tagSetMetrics[sig] == nil {
				tagSetMetrics[sig] = make(map[string]observer.SeriesMeta)
			}
			tagSetMetrics[sig][cursorKey] = meta
		}
	}

	// Find tag set with most metrics, breaking ties by total data points.
	numMetrics := len(r.metrics)
	var bestSig string
	bestMetricCount := 0
	bestPointCount := 0
	for sig, metricsMap := range tagSetMetrics {
		mc := len(metricsMap)
		if mc < bestMetricCount {
			continue
		}
		// Count total points across all metrics for this tag set
		pc := 0
		for _, meta := range metricsMap {
			pc += storage.PointCount(meta.Handle)
		}
		if mc > bestMetricCount || (mc == bestMetricCount && pc > bestPointCount) {
			bestMetricCount = mc
			bestPointCount = pc
			bestSig = sig
		}
	}

	if bestMetricCount == 0 {
		return false
	}

	if bestMetricCount < numMetrics {
		log.Printf("  RRCF WARNING: only %d/%d configured metrics found (tags=%s); alignment requires all metrics so no vectors will be produced until the missing metrics appear\n", bestMetricCount, numMetrics, bestSig)
	}
	log.Printf("  RRCF: resolved %d metrics to tag set with %d total points\n", bestMetricCount, bestPointCount)

	// Resolve all metrics to this tag set
	for cursorKey, meta := range tagSetMetrics[bestSig] {
		r.resolvedKeys[cursorKey] = meta.Handle
	}

	return true
}

// readNewPoints reads new data points for each metric since the last read.
func (r *RRCFDetector) readNewPoints(storage observer.StorageReader, dataTime int64) map[string][]observer.Point {
	result := make(map[string][]observer.Point)

	for _, m := range r.metrics {
		id, found := r.resolveKey(m)
		if !found {
			continue
		}

		cursorKey := m.Namespace + "|" + m.Name
		cursor := r.cursors[cursorKey]

		series := storage.GetSeriesRange(id, cursor, dataTime, m.Agg)
		if series == nil || len(series.Points) == 0 {
			continue
		}

		result[cursorKey] = series.Points
		r.cursors[cursorKey] = series.Points[len(series.Points)-1].Timestamp
	}

	return result
}

// timestampedVector represents a multivariate point at a specific timestamp.
type timestampedVector struct {
	timestamp int64
	values    []float64 // One value per metric, in order of r.metrics
}

// alignByTimestamp aligns points from different metrics by timestamp.
// Only timestamps that have data for ALL metrics are included.
func (r *RRCFDetector) alignByTimestamp(pointsByMetric map[string][]observer.Point) []timestampedVector {
	// Collect all timestamps and their values per metric
	type metricValue struct {
		metricIdx int
		value     float64
	}
	timestampData := make(map[int64][]metricValue)

	for i, m := range r.metrics {
		cursorKey := m.Namespace + "|" + m.Name
		points, ok := pointsByMetric[cursorKey]
		if !ok {
			continue
		}
		for _, p := range points {
			timestampData[p.Timestamp] = append(timestampData[p.Timestamp], metricValue{
				metricIdx: i,
				value:     p.Value,
			})
		}
	}

	// Build aligned vectors (only timestamps with all metrics present)
	numMetrics := len(r.metrics)
	var result []timestampedVector

	for ts, values := range timestampData {
		if len(values) != numMetrics {
			continue // Skip timestamps with missing metrics
		}

		vec := timestampedVector{
			timestamp: ts,
			values:    make([]float64, numMetrics),
		}
		for _, mv := range values {
			vec.values[mv.metricIdx] = mv.value
		}
		result = append(result, vec)
	}

	// Sort by timestamp
	sortTimestampedVectors(result)

	return result
}

// sortTimestampedVectors sorts vectors by timestamp ascending.
func sortTimestampedVectors(vecs []timestampedVector) {
	// Simple insertion sort (vectors are typically small and nearly sorted)
	for i := 1; i < len(vecs); i++ {
		for j := i; j > 0 && vecs[j].timestamp < vecs[j-1].timestamp; j-- {
			vecs[j], vecs[j-1] = vecs[j-1], vecs[j]
		}
	}
}

// shingle represents a temporal pattern combining multiple consecutive timestamps.
type shingle struct {
	endTimestamp int64     // Timestamp of the last point in the shingle
	vector       []float64 // Flattened: [t0_m0, t0_m1, ..., t1_m0, t1_m1, ..., etc.]
}

// buildShingles creates shingles by combining consecutive aligned points.
// A shingle of size 4 with 7 metrics produces a 28-dimensional vector.
func (r *RRCFDetector) buildShingles(aligned []timestampedVector) []shingle {
	if len(aligned) < r.config.ShingleSize {
		return nil
	}

	numMetrics := len(r.metrics)
	shingleDim := r.config.ShingleSize * numMetrics

	var result []shingle

	// Sliding window over aligned points
	for i := r.config.ShingleSize - 1; i < len(aligned); i++ {
		vec := make([]float64, 0, shingleDim)

		// Concatenate values from ShingleSize consecutive points
		for j := i - r.config.ShingleSize + 1; j <= i; j++ {
			vec = append(vec, aligned[j].values...)
		}

		result = append(result, shingle{
			endTimestamp: aligned[i].timestamp,
			vector:       vec,
		})
	}

	return result
}

// scoreAndDetect scores shingles using RRCF and returns anomalies and telemetry.
// Uses rolling z-score thresholding: after a warmup period (TreeSize points), a point
// is anomalous if its score exceeds mean + ThresholdSigma*stddev of the recent window.
func (r *RRCFDetector) scoreAndDetect(shingles []shingle, _ int64) observer.DetectionResult {
	var anomalies []observer.Anomaly
	var telemetry []observer.ObserverTelemetry
	warmup := r.config.TreeSize

	for _, s := range shingles {
		score := r.scoreShingle(s)
		r.totalScored++

		// Track all scores for offline threshold analysis
		r.allScores = append(r.allScores, RRCFScoredPoint{
			Timestamp: s.endTimestamp,
			Score:     score,
		})

		// Emit telemetry for the CoDisp score at every scored shingle
		telemetry = append(telemetry, observer.ObserverTelemetry{
			DetectorName: r.Name(),
			Metric: &metricObs{
				name:      "score",
				value:     score,
				timestamp: s.endTimestamp,
			},
		})

		// Skip warmup phase — scores are artificial during forest filling
		if r.totalScored <= warmup {
			continue
		}

		// Compute dynamic threshold from recent scores
		threshold := r.dynamicThreshold()

		// Emit telemetry for the dynamic threshold (only after warmup when threshold is meaningful)
		if threshold > 0 {
			telemetry = append(telemetry, observer.ObserverTelemetry{
				DetectorName: r.Name(),
				Metric: &metricObs{
					name:      "threshold",
					value:     threshold,
					timestamp: s.endTimestamp,
				},
			})
		}

		// Update rolling window (after computing threshold, so current score
		// doesn't influence its own threshold)
		r.recentScores = append(r.recentScores, score)
		if len(r.recentScores) > 100 {
			r.recentScores = r.recentScores[1:]
		}

		if r.config.ThresholdSigma > 0 && threshold > 0 && score > threshold {
			anomaly := observer.Anomaly{
				Source:       "score",
				DetectorName: r.Name(),
				Title:        "RRCF multivariate anomaly",
				Description:  fmt.Sprintf("Unusual system metric combination (CoDisp=%.1f, threshold=%.1f)", score, threshold),
				Timestamp:    s.endTimestamp,
				DebugInfo: &observer.AnomalyDebugInfo{
					CurrentValue:   score,
					Threshold:      threshold,
					DeviationSigma: (score - r.rollingMean()) / math.Max(r.rollingStddev(), 1),
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return observer.DetectionResult{
		Anomalies: anomalies,
		Telemetry: telemetry,
	}
}

// dynamicThreshold returns mean + ThresholdSigma*stddev of the recent score window.
func (r *RRCFDetector) dynamicThreshold() float64 {
	if len(r.recentScores) < 10 {
		return 0 // not enough data yet
	}
	return r.rollingMean() + r.config.ThresholdSigma*r.rollingStddev()
}

func (r *RRCFDetector) rollingMean() float64 {
	if len(r.recentScores) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range r.recentScores {
		sum += v
	}
	return sum / float64(len(r.recentScores))
}

func (r *RRCFDetector) rollingStddev() float64 {
	n := len(r.recentScores)
	if n < 2 {
		return 0
	}
	mean := r.rollingMean()
	sumSq := 0.0
	for _, v := range r.recentScores {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n))
}

// scoreShingle computes the CoDisp (collusive displacement) score for a shingle.
// Inserts the shingle into the RRCF forest and returns the average CoDisp score.
func (r *RRCFDetector) scoreShingle(s shingle) float64 {
	// Insert shingle into forest (handles eviction of oldest point if at capacity)
	_, avgCodisp := r.forest.insertPoint(s.vector)
	return avgCodisp
}

// Reset clears all state, useful for testing or after major regime changes.
func (r *RRCFDetector) Reset() {
	r.resolvedKeys = make(map[string]observer.SeriesHandle)
	r.cursors = make(map[string]int64)
	r.recentScores = r.recentScores[:0]
	r.allScores = r.allScores[:0]
	r.totalScored = 0
	r.alignedCount = 0
	r.shingleCount = 0
	r.forest.reset()
}

// GetExtraData implements ComponentDataProvider, exposing score stats via /api/components/rrcf/data.
func (r *RRCFDetector) GetExtraData() interface{} {
	return r.GetScoreStats()
}

// GetScoreStats returns distribution statistics and full score history.
func (r *RRCFDetector) GetScoreStats() RRCFScoreStats {
	stats := RRCFScoreStats{
		Enabled:       true,
		SampleCount:   len(r.allScores),
		AlignedPoints: r.alignedCount,
		ShinglesBuilt: r.shingleCount,
		Config: RRCFConfigSummary{
			NumTrees:       r.config.NumTrees,
			TreeSize:       r.config.TreeSize,
			ShingleSize:    r.config.ShingleSize,
			ShingleDim:     r.config.ShingleSize * len(r.metrics),
			ThresholdSigma: r.config.ThresholdSigma,
		},
		Scores: r.allScores,
	}

	for _, m := range r.metrics {
		stats.Metrics = append(stats.Metrics, m.Namespace+"|"+m.Name)
	}

	if len(r.allScores) == 0 {
		return stats
	}

	// Compute distribution stats
	sorted := make([]float64, len(r.allScores))
	sum := 0.0
	for i, sp := range r.allScores {
		sorted[i] = sp.Score
		sum += sp.Score
	}
	sort.Float64s(sorted)

	n := float64(len(sorted))
	stats.MinScore = sorted[0]
	stats.MaxScore = sorted[len(sorted)-1]
	stats.MeanScore = sum / n

	// Stddev
	sumSq := 0.0
	for _, v := range sorted {
		d := v - stats.MeanScore
		sumSq += d * d
	}
	stats.StddevScore = math.Sqrt(sumSq / n)

	// Percentiles (nearest-rank method)
	pct := func(p float64) float64 {
		idx := int(math.Ceil(p/100.0*n)) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	stats.P50 = pct(50)
	stats.P75 = pct(75)
	stats.P90 = pct(90)
	stats.P95 = pct(95)
	stats.P99 = pct(99)

	return stats
}
