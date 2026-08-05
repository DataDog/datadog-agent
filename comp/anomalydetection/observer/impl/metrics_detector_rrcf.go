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

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const maxRRCFLogScoreHistory = 100_000

// RRCFScoredPoint records a CoDisp score at a specific timestamp.
type RRCFScoredPoint struct {
	Timestamp int64   `json:"timestamp"`
	Score     float64 `json:"score"`
}

// RRCFScoreStats contains distribution statistics and retained score history
// for threshold analysis. Log score history is capped to bound live-agent memory.
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
	LogShingleDim  int     `json:"logShingleDim"`
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
	NumTrees int `json:"num_trees"`
	// TreeSize is the maximum number of points per tree (sliding window size).
	TreeSize int `json:"tree_size"`
	// ShingleSize is the number of consecutive timestamps to combine into one point.
	// ShingleSize=4 means each "point" is 4 consecutive samples, enabling temporal pattern detection.
	ShingleSize int `json:"shingle_size"`
	// ThresholdSigma controls dynamic anomaly thresholding. A point is flagged if its
	// CoDisp score exceeds mean + ThresholdSigma*stddev of the recent score window.
	// Set to 0 to disable anomaly detection (scores still computed for analysis).
	ThresholdSigma float64 `json:"threshold_sigma"`
	// Metrics defines which series to include. If nil, uses DefaultRRCFMetrics().
	Metrics []RRCFMetricDef `json:"-"`
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

// DefaultRRCFMetrics returns the default metric set for RRCF.
// These match cgroup v2 metrics from FGM parquet exports, which is the
// format used by the testbench scenarios where RRCF has been validated.
func DefaultRRCFMetrics() []RRCFMetricDef {
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

// RRCFDetector implements anomaly detection using Robust Random Cut Forest.
// It uses a multivariate forest for configured system metrics and a separate,
// bounded forest of scalar count shingles for dynamically named log series.
type RRCFDetector struct {
	config RRCFConfig
	// telemetry is optional and wired by the observer component.
	telemetry *observerTelemetry

	// metrics defines which series to include in the multivariate analysis.
	// Each metric becomes a dimension in the feature vector.
	metrics []RRCFMetricDef

	// resolvedKeys caches the numeric series ID for each metric.
	// Populated lazily on first Detect call via ListSeries discovery.
	resolvedKeys map[string]observer.SeriesRef
	// resolveGeneration prevents rescanning the fixed metric set until storage
	// gains another series. Partial matches are never committed.
	resolveGeneration uint64
	resolveAttempted  bool

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
	anomalous    bool
	// alignedPrefix carries the final ShingleSize-1 aligned vectors across
	// Detect calls. Without it, one-point streaming advances never build a
	// shingle even though batch replay does.
	alignedPrefix []timestampedVector

	// Log-derived series have dynamic names, so they cannot participate in the
	// fixed metric allowlist above. They share one bounded forest of scalar
	// temporal shingles. Per-series state only retains the shingle prefix and
	// incremental read cursor.
	logForest       *rcForest
	logSeries       map[observer.SeriesRef]*rrcfLogSeriesState
	logRefs         []observer.SeriesRef
	logSeriesGen    uint64
	logSeriesCached bool
	logRecentScores []float64
	logTotalScored  int
	logScores       []RRCFScoredPoint
	logScoreStart   int
	logAlignedCount int
	logShingleCount int
}

type rrcfLogSeriesState struct {
	lastProcessedTime  int64
	lastProcessedCount int
	lastWriteGen       int64
	shinglePrefix      []float64
	source             observer.SeriesDescriptor
	anomalous          bool
}

type rrcfLogShingle struct {
	shingle
	ref    observer.SeriesRef
	source observer.SeriesDescriptor
}

// NewRRCFDetector creates an RRCF detector with the given config.
func NewRRCFDetector(config RRCFConfig) *RRCFDetector {
	defaults := DefaultRRCFConfig()
	if config.NumTrees <= 0 {
		config.NumTrees = defaults.NumTrees
	}
	if config.TreeSize <= 0 {
		config.TreeSize = defaults.TreeSize
	}
	if config.ShingleSize <= 0 {
		config.ShingleSize = defaults.ShingleSize
	}

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
		resolvedKeys: make(map[string]observer.SeriesRef),
		cursors:      make(map[string]int64),
		forest:       forest,
		recentScores: make([]float64, 0, 100),
		allScores:    make([]RRCFScoredPoint, 0, 1024),
		logForest:    newRCForest(config.NumTrees, config.TreeSize, config.ShingleSize, 43),
		logSeries:    make(map[observer.SeriesRef]*rrcfLogSeriesState),
		logScores:    make([]RRCFScoredPoint, 0, 1024),
	}
}

// Name returns the detector name.
func (r *RRCFDetector) Name() string {
	return "rrcf"
}

// SetObserverTelemetry wires direct observer telemetry emission.
func (r *RRCFDetector) SetObserverTelemetry(t *observerTelemetry) {
	r.telemetry = t
}

// Detect implements Detector. It analyzes both configured system metrics and
// dynamically named count series emitted by log extractors.
func (r *RRCFDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	var result observer.DetectionResult

	// Step 0: Resolve all metric keys to the same tag set (on first call)
	if r.resolveAllKeys(storage) {
		result = r.detectConfiguredMetrics(storage, dataTime)
	}

	logResult := r.detectLogSeries(storage, dataTime)
	result.Anomalies = append(result.Anomalies, logResult.Anomalies...)
	return result
}

func (r *RRCFDetector) detectConfiguredMetrics(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
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
func (r *RRCFDetector) resolveKey(m RRCFMetricDef) (observer.SeriesRef, bool) {
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

	generation := storage.SeriesGeneration()
	if r.resolveAttempted && r.resolveGeneration == generation {
		return false
	}
	r.resolveAttempted = true
	r.resolveGeneration = generation

	// Collect all configured metrics with one catalog scan per namespace.
	// The default seven-metric set shares a namespace, so this avoids seven
	// large ListSeries result allocations whenever resolution is retried.
	seriesByMetric := make(map[string][]observer.SeriesMeta) // cursorKey -> all matching series
	metricKeysByNamespace := make(map[string]map[string]string)
	for _, m := range r.metrics {
		cursorKey := m.Namespace + "|" + m.Name
		if metricKeysByNamespace[m.Namespace] == nil {
			metricKeysByNamespace[m.Namespace] = make(map[string]string)
		}
		metricKeysByNamespace[m.Namespace][m.Name] = cursorKey
	}
	for namespace, metricKeys := range metricKeysByNamespace {
		matches := storage.ListSeries(observer.SeriesFilter{Namespace: namespace})
		for _, meta := range matches {
			if cursorKey, ok := metricKeys[meta.Name]; ok {
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
			pc += storage.PointCount(meta.Ref)
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
		return false
	}
	log.Printf("  RRCF: resolved %d metrics to tag set with %d total points\n", bestMetricCount, bestPointCount)

	// Resolve all metrics to this tag set
	for cursorKey, meta := range tagSetMetrics[bestSig] {
		r.resolvedKeys[cursorKey] = meta.Ref
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

func (r *RRCFDetector) detectLogSeries(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	r.refreshLogSeries(storage)
	if len(r.logRefs) == 0 {
		return observer.DetectionResult{}
	}

	shingles, rebuild := r.collectLogShingles(storage, dataTime, false)
	if rebuild {
		r.resetLogDetection()
		shingles, _ = r.collectLogShingles(storage, dataTime, true)
	}
	if len(shingles) == 0 {
		return observer.DetectionResult{}
	}

	sort.Slice(shingles, func(i, j int) bool {
		if shingles[i].endTimestamp != shingles[j].endTimestamp {
			return shingles[i].endTimestamp < shingles[j].endTimestamp
		}
		return shingles[i].ref < shingles[j].ref
	})

	var anomalies []observer.Anomaly
	for _, item := range shingles {
		_, score := r.logForest.insertPoint(item.vector)
		r.logTotalScored++
		r.appendLogScore(RRCFScoredPoint{
			Timestamp: item.endTimestamp,
			Score:     score,
		})
		if r.telemetry != nil {
			r.telemetry.recordRRCFScore(r.Name(), score)
		}

		if r.logTotalScored <= r.config.TreeSize {
			continue
		}

		threshold, ready := rrcfDynamicThreshold(r.logRecentScores, r.config.ThresholdSigma)
		baselineMean := rrcfMean(r.logRecentScores)
		baselineStddev := rrcfStddev(r.logRecentScores, baselineMean)
		if ready && r.telemetry != nil {
			r.telemetry.recordRRCFThreshold(r.Name(), threshold)
		}

		r.logRecentScores = appendRRCFScore(r.logRecentScores, score)
		state := r.logSeries[item.ref]
		if !rrcfShouldEmitAnomaly(&state.anomalous, r.config.ThresholdSigma > 0, ready, score, threshold) {
			continue
		}

		anomalyScore := score
		anomalies = append(anomalies, observer.Anomaly{
			Source:       item.source,
			SourceRef:    &observer.QueryHandle{Ref: item.ref, Aggregate: observer.AggregateCount},
			DetectorName: r.Name(),
			Title:        "RRCF log-count anomaly",
			Description:  fmt.Sprintf("Unusual log count trajectory (CoDisp=%.1f, threshold=%.1f)", score, threshold),
			Timestamp:    item.endTimestamp,
			Score:        &anomalyScore,
			DebugInfo: &observer.AnomalyDebugInfo{
				CurrentValue:   score,
				Threshold:      threshold,
				DeviationSigma: (score - baselineMean) / math.Max(baselineStddev, 1),
			},
		})
	}

	return observer.DetectionResult{Anomalies: anomalies}
}

func (r *RRCFDetector) appendLogScore(score RRCFScoredPoint) {
	if len(r.logScores) < maxRRCFLogScoreHistory {
		r.logScores = append(r.logScores, score)
		return
	}
	r.logScores[r.logScoreStart] = score
	r.logScoreStart = (r.logScoreStart + 1) % len(r.logScores)
}

func (r *RRCFDetector) orderedLogScores() []RRCFScoredPoint {
	if len(r.logScores) < maxRRCFLogScoreHistory || r.logScoreStart == 0 {
		return r.logScores
	}
	scores := make([]RRCFScoredPoint, 0, len(r.logScores))
	scores = append(scores, r.logScores[r.logScoreStart:]...)
	scores = append(scores, r.logScores[:r.logScoreStart]...)
	return scores
}

func (r *RRCFDetector) refreshLogSeries(storage observer.StorageReader) {
	generation := storage.SeriesGeneration()
	if r.logSeriesCached && r.logSeriesGen == generation {
		return
	}

	r.logRefs = r.logRefs[:0]
	for _, meta := range storage.ListSeries(observer.WorkloadSeriesFilter()) {
		if isLogDerivedNamespace(meta.Namespace) {
			r.logRefs = append(r.logRefs, meta.Ref)
		}
	}
	sort.Slice(r.logRefs, func(i, j int) bool { return r.logRefs[i] < r.logRefs[j] })
	r.logSeriesGen = generation
	r.logSeriesCached = true
}

func isLogDerivedNamespace(namespace string) bool {
	switch namespace {
	case LogMetricsExtractorName, LogPatternExtractorName, "connection_error_extractor":
		return true
	default:
		return false
	}
}

// collectLogShingles incrementally builds scalar temporal shingles for every
// log-derived series. The forest is shared across series to keep memory bounded:
// each shingle still carries its source identity for anomaly attribution.
//
// A same-bucket merge or out-of-order backfill invalidates an incremental
// prefix. The caller then resets the shared log forest and rebuilds all visible
// log series, preserving batch/replay equivalence.
func (r *RRCFDetector) collectLogShingles(storage observer.StorageReader, dataTime int64, forceRebuild bool) ([]rrcfLogShingle, bool) {
	var result []rrcfLogShingle

	for _, ref := range r.logRefs {
		visibleCount := storage.PointCountUpTo(ref, dataTime)
		writeGen := storage.WriteGeneration(ref)
		state := r.logSeries[ref]

		var start int64
		if state != nil && !forceRebuild {
			if state.lastProcessedCount == visibleCount && state.lastWriteGen == writeGen {
				continue
			}
			start = state.lastProcessedTime
		}

		series := storage.GetSeriesRange(ref, start, dataTime, observer.AggregateCount)
		if series == nil {
			continue
		}

		if state != nil && !forceRebuild {
			expectedNewPoints := visibleCount - state.lastProcessedCount
			sameBucketChanged := expectedNewPoints == 0 && state.lastWriteGen != writeGen
			if expectedNewPoints < 0 || sameBucketChanged || len(series.Points) != expectedNewPoints {
				return nil, true
			}
		}

		if state == nil || forceRebuild {
			state = &rrcfLogSeriesState{
				shinglePrefix: make([]float64, 0, r.config.ShingleSize),
			}
			r.logSeries[ref] = state
		}
		state.source = observer.SeriesDescriptor{
			Namespace: series.Namespace,
			Name:      series.Name,
			Tags:      series.Tags,
			Aggregate: observer.AggregateCount,
		}

		for _, point := range series.Points {
			state.shinglePrefix = append(state.shinglePrefix, point.Value)
			if len(state.shinglePrefix) > r.config.ShingleSize {
				copy(state.shinglePrefix, state.shinglePrefix[1:])
				state.shinglePrefix = state.shinglePrefix[:r.config.ShingleSize]
			}
			if len(state.shinglePrefix) < r.config.ShingleSize {
				continue
			}

			vector := make([]float64, len(state.shinglePrefix))
			copy(vector, state.shinglePrefix)
			result = append(result, rrcfLogShingle{
				shingle: shingle{
					endTimestamp: point.Timestamp,
					vector:       vector,
				},
				ref:    ref,
				source: state.source,
			})
		}

		r.logAlignedCount += len(series.Points)
		state.lastProcessedCount = visibleCount
		state.lastWriteGen = writeGen
		if len(series.Points) > 0 {
			state.lastProcessedTime = series.Points[len(series.Points)-1].Timestamp
		}
	}

	r.logShingleCount += len(result)
	return result, false
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
	// Simple insertion sort (vectors are typically small)
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
	combined := make([]timestampedVector, 0, len(r.alignedPrefix)+len(aligned))
	combined = append(combined, r.alignedPrefix...)
	combined = append(combined, aligned...)

	prefixLen := len(r.alignedPrefix)
	keep := r.config.ShingleSize - 1
	if keep > len(combined) {
		keep = len(combined)
	}
	r.alignedPrefix = append(r.alignedPrefix[:0], combined[len(combined)-keep:]...)

	if len(combined) < r.config.ShingleSize {
		return nil
	}

	numMetrics := len(r.metrics)
	shingleDim := r.config.ShingleSize * numMetrics

	var result []shingle

	// Sliding window over aligned points. Only emit shingles whose final point
	// came from this Detect call; prefix-only shingles were already scored.
	firstEnd := r.config.ShingleSize - 1
	if prefixLen > firstEnd {
		firstEnd = prefixLen
	}
	for i := firstEnd; i < len(combined); i++ {
		vec := make([]float64, 0, shingleDim)

		// Concatenate values from ShingleSize consecutive points
		for j := i - r.config.ShingleSize + 1; j <= i; j++ {
			vec = append(vec, combined[j].values...)
		}

		result = append(result, shingle{
			endTimestamp: combined[i].timestamp,
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
	warmup := r.config.TreeSize

	for _, s := range shingles {
		score := r.scoreShingle(s)
		r.totalScored++

		// Track all scores for offline threshold analysis
		r.allScores = append(r.allScores, RRCFScoredPoint{
			Timestamp: s.endTimestamp,
			Score:     score,
		})
		if r.telemetry != nil {
			r.telemetry.recordRRCFScore(r.Name(), score)
		}

		// Skip warmup phase — scores are artificial during forest filling
		if r.totalScored <= warmup {
			continue
		}

		// Compute dynamic threshold from recent scores
		threshold, ready := rrcfDynamicThreshold(r.recentScores, r.config.ThresholdSigma)
		baselineMean := r.rollingMean()
		baselineStddev := r.rollingStddev()
		if ready && r.telemetry != nil {
			r.telemetry.recordRRCFThreshold(r.Name(), threshold)
		}

		// Update rolling window (after computing threshold, so current score
		// doesn't influence its own threshold)
		r.recentScores = appendRRCFScore(r.recentScores, score)

		if rrcfShouldEmitAnomaly(&r.anomalous, r.config.ThresholdSigma > 0, ready, score, threshold) {
			anomalyScore := score
			anomaly := observer.Anomaly{
				Source:       observer.SeriesDescriptor{Namespace: "rrcf", Name: "score"},
				DetectorName: r.Name(),
				Title:        "RRCF multivariate anomaly",
				Description:  fmt.Sprintf("Unusual system metric combination (CoDisp=%.1f, threshold=%.1f)", score, threshold),
				Timestamp:    s.endTimestamp,
				Score:        &anomalyScore,
				DebugInfo: &observer.AnomalyDebugInfo{
					CurrentValue:   score,
					Threshold:      threshold,
					DeviationSigma: (score - baselineMean) / math.Max(baselineStddev, 1),
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return observer.DetectionResult{
		Anomalies: anomalies,
	}
}

func (r *RRCFDetector) rollingMean() float64 {
	return rrcfMean(r.recentScores)
}

func rrcfMean(scores []float64) float64 {
	sum := 0.0
	for _, v := range scores {
		sum += v
	}
	if len(scores) == 0 {
		return 0
	}
	return sum / float64(len(scores))
}

func (r *RRCFDetector) rollingStddev() float64 {
	return rrcfStddev(r.recentScores, r.rollingMean())
}

func rrcfStddev(scores []float64, mean float64) float64 {
	n := len(scores)
	if n < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range scores {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n))
}

func rrcfDynamicThreshold(scores []float64, thresholdSigma float64) (float64, bool) {
	if len(scores) < 10 {
		return 0, false
	}
	mean := rrcfMean(scores)
	return mean + thresholdSigma*rrcfStddev(scores, mean), true
}

func appendRRCFScore(scores []float64, score float64) []float64 {
	scores = append(scores, score)
	if len(scores) > 100 {
		copy(scores, scores[len(scores)-100:])
		scores = scores[:100]
	}
	return scores
}

// rrcfShouldEmitAnomaly reports only the transition into an anomalous state.
// Continued above-threshold scores belong to the same episode and are
// suppressed until a normal score resets the state.
func rrcfShouldEmitAnomaly(anomalous *bool, enabled, ready bool, score, threshold float64) bool {
	if !enabled || !ready || score <= threshold {
		*anomalous = false
		return false
	}
	if *anomalous {
		return false
	}
	*anomalous = true
	return true
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
	r.resetConfiguredMetricDetection()
	r.logRefs = nil
	r.logSeriesGen = 0
	r.logSeriesCached = false
	r.resetLogDetection()
}

func (r *RRCFDetector) resetConfiguredMetricDetection() {
	r.resolvedKeys = make(map[string]observer.SeriesRef)
	r.resolveAttempted = false
	r.resolveGeneration = 0
	r.cursors = make(map[string]int64)
	r.recentScores = r.recentScores[:0]
	r.allScores = r.allScores[:0]
	r.totalScored = 0
	r.alignedCount = 0
	r.shingleCount = 0
	r.anomalous = false
	r.alignedPrefix = nil
	r.forest.reset()
}

func (r *RRCFDetector) resetLogDetection() {
	r.logSeries = make(map[observer.SeriesRef]*rrcfLogSeriesState)
	r.logRecentScores = r.logRecentScores[:0]
	r.logScores = r.logScores[:0]
	r.logScoreStart = 0
	r.logTotalScored = 0
	r.logAlignedCount = 0
	r.logShingleCount = 0
	r.logForest.reset()
}

// RemoveSeries drops state for series evicted from storage. Removing any
// configured metric invalidates the whole multivariate pipeline because its
// forest and shingles combine every configured input. Log-series state is
// independent; only states for the removed refs are discarded, while their
// historical shingles remain in the bounded shared forest until FIFO eviction.
func (r *RRCFDetector) RemoveSeries(refs []observer.SeriesRef) {
	configuredMetricRemoved := false
	for _, ref := range refs {
		delete(r.logSeries, ref)
		if configuredMetricRemoved {
			continue
		}
		for _, resolvedRef := range r.resolvedKeys {
			if ref == resolvedRef {
				configuredMetricRemoved = true
				break
			}
		}
	}
	if configuredMetricRemoved {
		r.resetConfiguredMetricDetection()
	}
	r.logRefs = nil
	r.logSeriesCached = false
}

// GetExtraData implements ComponentDataProvider, exposing score stats via /api/components/rrcf/data.
func (r *RRCFDetector) GetExtraData() interface{} {
	return r.GetScoreStats()
}

// GetScoreStats returns distribution statistics and retained score history.
func (r *RRCFDetector) GetScoreStats() RRCFScoreStats {
	logScores := r.orderedLogScores()
	scores := make([]RRCFScoredPoint, 0, len(r.allScores)+len(logScores))
	scores = append(scores, r.allScores...)
	scores = append(scores, logScores...)
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Timestamp != scores[j].Timestamp {
			return scores[i].Timestamp < scores[j].Timestamp
		}
		return scores[i].Score < scores[j].Score
	})

	stats := RRCFScoreStats{
		Enabled:       true,
		SampleCount:   len(scores),
		AlignedPoints: r.alignedCount + r.logAlignedCount,
		ShinglesBuilt: r.shingleCount + r.logShingleCount,
		Config: RRCFConfigSummary{
			NumTrees:       r.config.NumTrees,
			TreeSize:       r.config.TreeSize,
			ShingleSize:    r.config.ShingleSize,
			ShingleDim:     r.config.ShingleSize * len(r.metrics),
			LogShingleDim:  r.config.ShingleSize,
			ThresholdSigma: r.config.ThresholdSigma,
		},
		Scores: scores,
	}

	for _, m := range r.metrics {
		stats.Metrics = append(stats.Metrics, m.Namespace+"|"+m.Name)
	}
	for _, ref := range r.logRefs {
		if state := r.logSeries[ref]; state != nil && state.source.Name != "" {
			stats.Metrics = append(stats.Metrics, state.source.Key())
		}
	}

	if len(scores) == 0 {
		return stats
	}

	// Compute distribution stats
	sorted := make([]float64, len(scores))
	sum := 0.0
	for i, sp := range scores {
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
