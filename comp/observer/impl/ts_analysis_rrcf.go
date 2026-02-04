// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// RRCFConfig holds configuration for the RRCF analysis.
type RRCFConfig struct {
	// NumTrees is the number of trees in the forest. More trees = more robust but slower.
	NumTrees int
	// TreeSize is the maximum number of points per tree (sliding window size).
	TreeSize int
	// ShingleSize is the number of consecutive timestamps to combine into one point.
	// ShingleSize=4 means each "point" is 4 consecutive samples, enabling temporal pattern detection.
	ShingleSize int
	// AnomalyThreshold is the CoDisp score above which a point is considered anomalous.
	// This may need tuning based on empirical data.
	AnomalyThreshold float64
}

// DefaultRRCFConfig returns sensible defaults for RRCF.
func DefaultRRCFConfig() RRCFConfig {
	return RRCFConfig{
		NumTrees:         100,
		TreeSize:         256,
		ShingleSize:      4,
		AnomalyThreshold: 0.0, // TODO: Determine good threshold empirically
	}
}

// RRCFAnalysis implements multivariate anomaly detection using Robust Random Cut Forest.
// It queries multiple system metrics and detects unusual combinations/trajectories.
type RRCFAnalysis struct {
	config RRCFConfig

	// metrics defines which series to include in the multivariate analysis.
	// Each metric becomes a dimension in the feature vector.
	metrics []metricDef

	// cursors tracks read position per metric for incremental reads.
	cursors map[string]int64

	// shingleBuffer accumulates recent points to form shingles.
	// Key is metric name, value is recent values (up to ShingleSize).
	shingleBuffer map[string][]float64

	// forest is the RRCF forest structure.
	forest *rcForest

	// recentScores tracks recent CoDisp scores for dynamic thresholding.
	recentScores []float64
}

// metricDef defines a metric to include in the RRCF analysis.
type metricDef struct {
	namespace string
	name      string
	agg       observer.Aggregate
}

// NewRRCFAnalysis creates an RRCF analysis with the given config.
func NewRRCFAnalysis(config RRCFConfig) *RRCFAnalysis {
	metrics := []metricDef{
		// CPU metrics
		{namespace: "system", name: "cpu.user", agg: observer.AggregateAverage},
		{namespace: "system", name: "cpu.system", agg: observer.AggregateAverage},
		{namespace: "system", name: "cpu.iowait", agg: observer.AggregateAverage},
		// Memory metrics
		{namespace: "system", name: "memory.used", agg: observer.AggregateAverage},
		{namespace: "system", name: "memory.rss", agg: observer.AggregateAverage},
		// Disk IO metrics
		{namespace: "system", name: "disk.read_bytes", agg: observer.AggregateSum},
		{namespace: "system", name: "disk.write_bytes", agg: observer.AggregateSum},
	}

	// Compute shingle dimension: numMetrics * shingleSize
	numMetrics := len(metrics)
	shingleDim := numMetrics * config.ShingleSize

	// Create forest with fixed seed for reproducibility (can be made configurable)
	forest := newRCForest(config.NumTrees, config.TreeSize, shingleDim, 42)

	return &RRCFAnalysis{
		config:        config,
		metrics:       metrics,
		cursors:       make(map[string]int64),
		shingleBuffer: make(map[string][]float64),
		forest:        forest,
		recentScores:  make([]float64, 0, 100),
	}
}

// Name returns the analysis name.
func (r *RRCFAnalysis) Name() string {
	return "rrcf"
}

// Analyze implements MultiSeriesAnalysis. It queries storage for system metrics,
// builds multivariate shingles, and detects anomalies using RRCF.
func (r *RRCFAnalysis) Analyze(storage observer.StorageReader, dataTime int64) []observer.AnomalyOutput {
	// Step 1: Read new points for each metric since last cursor
	newPointsByMetric := r.readNewPoints(storage, dataTime)
	if len(newPointsByMetric) == 0 {
		return nil
	}

	// Step 2: Align points by timestamp and build multivariate vectors
	alignedPoints := r.alignByTimestamp(newPointsByMetric)
	if len(alignedPoints) == 0 {
		return nil
	}

	// Step 3: Build shingles from aligned points
	shingles := r.buildShingles(alignedPoints)
	if len(shingles) == 0 {
		return nil
	}

	// Step 4: Score shingles with RRCF and detect anomalies
	anomalies := r.scoreAndDetect(shingles, dataTime)

	return anomalies
}

// readNewPoints reads new data points for each metric since the last cursor position.
func (r *RRCFAnalysis) readNewPoints(storage observer.StorageReader, dataTime int64) map[string][]observer.Point {
	result := make(map[string][]observer.Point)

	for _, m := range r.metrics {
		key := observer.SeriesKey{
			Namespace: m.namespace,
			Name:      m.name,
			Tags:      nil, // System metrics typically have no tags or we'd filter here
		}

		cursorKey := m.namespace + "|" + m.name
		cursor := r.cursors[cursorKey]

		points, newCursor := storage.ReadSince(key, cursor, m.agg)

		// Only include points up to dataTime for determinism
		var filteredPoints []observer.Point
		for _, p := range points {
			if p.Timestamp <= dataTime {
				filteredPoints = append(filteredPoints, p)
			}
		}

		if len(filteredPoints) > 0 {
			result[cursorKey] = filteredPoints
			r.cursors[cursorKey] = newCursor
		}
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
func (r *RRCFAnalysis) alignByTimestamp(pointsByMetric map[string][]observer.Point) []timestampedVector {
	// Collect all timestamps and their values per metric
	type metricValue struct {
		metricIdx int
		value     float64
	}
	timestampData := make(map[int64][]metricValue)

	for i, m := range r.metrics {
		cursorKey := m.namespace + "|" + m.name
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
func (r *RRCFAnalysis) buildShingles(aligned []timestampedVector) []shingle {
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

// scoreAndDetect scores shingles using RRCF and returns anomalies for high scores.
func (r *RRCFAnalysis) scoreAndDetect(shingles []shingle, _ int64) []observer.AnomalyOutput {
	var anomalies []observer.AnomalyOutput

	for _, s := range shingles {
		score := r.scoreShingle(s)

		// Track recent scores for potential dynamic thresholding
		r.recentScores = append(r.recentScores, score)
		if len(r.recentScores) > 100 {
			r.recentScores = r.recentScores[1:]
		}

		// Check against threshold
		if score > r.config.AnomalyThreshold && r.config.AnomalyThreshold > 0 {
			anomaly := observer.AnomalyOutput{
				Source:       "rrcf:system_metrics",
				AnalyzerName: r.Name(),
				Title:        "RRCF multivariate anomaly",
				Description:  fmt.Sprintf("Unusual system metric combination detected (CoDisp=%.2f)", score),
				Timestamp:    s.endTimestamp,
				DebugInfo: &observer.AnomalyDebugInfo{
					CurrentValue:   score,
					Threshold:      r.config.AnomalyThreshold,
					DeviationSigma: score, // CoDisp score
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

// scoreShingle computes the CoDisp (collusive displacement) score for a shingle.
// Inserts the shingle into the RRCF forest and returns the average CoDisp score.
func (r *RRCFAnalysis) scoreShingle(s shingle) float64 {
	// Insert shingle into forest (handles eviction of oldest point if at capacity)
	_, avgCodisp := r.forest.insertPoint(s.vector)
	return avgCodisp
}

// Reset clears all state, useful for testing or after major regime changes.
func (r *RRCFAnalysis) Reset() {
	r.cursors = make(map[string]int64)
	r.shingleBuffer = make(map[string][]float64)
	r.recentScores = r.recentScores[:0]
	r.forest.reset()
}
