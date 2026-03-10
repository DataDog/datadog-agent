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

// CorrelationConfig holds configuration for the cross-correlation changepoint detector.
type CorrelationConfig struct {
	// MinPoints is the minimum number of data points a series must have to be considered.
	MinPoints int
	// MaxSeries caps how many series we analyze (top-K by variance).
	MaxSeries int
	// WindowSize is the number of data points in each sliding window for correlation computation.
	WindowSize int
	// StepSize is how many points to advance the window each step.
	StepSize int
	// ThresholdSigma controls dynamic thresholding: flag if Frobenius norm delta > mean + sigma*stddev.
	ThresholdSigma float64
	// MinFrobeniusDelta is an absolute minimum for the Frobenius norm difference to fire.
	MinFrobeniusDelta float64
	// TopPairsToReport is how many top changing pairs to include in anomaly description.
	TopPairsToReport int
	// PairDeltaMin is the minimum absolute correlation change for a pair to be reported.
	PairDeltaMin float64
	// ExcludePrefixes is a list of metric name prefixes to exclude from series selection.
	// Infrastructure/platform metrics matching these prefixes are skipped so that
	// application-level metrics can enter the correlation analysis pool.
	ExcludePrefixes []string
	// PreferServiceTags boosts the effective variance of series that have a service: tag
	// by ServiceTagBoost multiplier, making them more likely to be selected.
	PreferServiceTags bool
	// ServiceTagBoost is the variance multiplier for series with a service: tag.
	// Only used when PreferServiceTags is true.
	ServiceTagBoost float64
	// BaselineThresholdFraction controls what fraction of initial norm values are used
	// to compute a fixed baseline threshold. The threshold is computed once from the
	// first N norm values and never updated, preventing gradual drift from defeating detection.
	// 0 means use rolling threshold (legacy behavior).
	BaselineThresholdFraction float64
}

// defaultCorrExcludePrefixes lists infrastructure/platform metric families that
// are excluded from correlation series selection. These are the same prefixes
// used by TopK — general Kubernetes and agent infrastructure metrics that
// appear in every deployment and dominate variance rankings over application metrics.
var defaultCorrExcludePrefixes = []string{
	"datadog.agent.",
	"kube_apiserver.",
	"kubernetes.",
	"kubelet.",
	"container.",
	"containerd.",
	"cri.",
	"system.disk.",
	"system.io.",
}

// DefaultCorrelationConfig returns sensible defaults.
func DefaultCorrelationConfig() CorrelationConfig {
	return CorrelationConfig{
		MinPoints:                 40,
		MaxSeries:                 80,
		WindowSize:                15,
		StepSize:                  3,
		ThresholdSigma:            2.0,
		MinFrobeniusDelta:         0.10,
		TopPairsToReport:          8,
		PairDeltaMin:              0.3,
		ExcludePrefixes:           defaultCorrExcludePrefixes,
		PreferServiceTags:         false,
		ServiceTagBoost:           2.0,
		BaselineThresholdFraction: 0.25,
	}
}

// CorrelationDetector implements MultiSeriesDetector using cross-correlation changepoint detection.
// It tracks the pairwise correlation matrix across sliding windows and flags when
// the correlation structure changes significantly (Lung-Yut-Fong et al., 2015).
type CorrelationDetector struct {
	config CorrelationConfig

	// lastProcessedTime tracks the latest data timestamp we've analyzed.
	lastProcessedTime int64

	// recentNorms tracks recent Frobenius norm deltas for dynamic thresholding.
	recentNorms []float64

	// firedSeries tracks which series have already fired at which timestamp to avoid dups.
	firedSeries map[string]bool
}

// NewCorrelationDetector creates a new cross-correlation changepoint detector.
func NewCorrelationDetector() *CorrelationDetector {
	return NewCorrelationDetectorWithConfig(DefaultCorrelationConfig())
}

// NewCorrelationDetectorWithConfig creates a new detector with the given config.
func NewCorrelationDetectorWithConfig(config CorrelationConfig) *CorrelationDetector {
	return &CorrelationDetector{
		config:      config,
		recentNorms: make([]float64, 0, 200),
		firedSeries: make(map[string]bool),
	}
}

// Name returns the detector name.
func (d *CorrelationDetector) Name() string {
	return "correlation"
}

// corrSeriesInfo holds a resolved series with its data for correlation analysis.
type corrSeriesInfo struct {
	key      observer.SeriesKey
	label    string // human-readable label: "service:name"
	metricID string // "service:metricName" format for Source field
	points   []observer.Point
	variance float64
}

// Detect implements MultiSeriesDetector. It queries storage for all metric series,
// selects the most variable ones, computes correlation matrices in sliding windows,
// and flags when the correlation structure changes.
func (d *CorrelationDetector) Detect(storage observer.StorageReader, dataTime int64) observer.DetectionResult {
	// Step 1: Discover all series
	allKeys := storage.ListSeries(observer.SeriesFilter{})

	// Step 2: Filter to series with enough data, exclude internal and infrastructure metrics
	var candidates []corrSeriesInfo
	for _, key := range allKeys {
		// Skip internal/observer/telemetry metrics
		if key.Namespace == "observer" || key.Namespace == "internal" || key.Namespace == "telemetry" {
			continue
		}
		// Skip infrastructure metrics that dominate variance rankings
		if d.corrIsExcluded(key.Name) {
			continue
		}
		// Skip series with too few points
		pc := storage.PointCount(key)
		if pc < d.config.MinPoints {
			continue
		}

		// Read all data up to dataTime
		series := storage.GetSeriesRange(key, 0, dataTime, observer.AggregateAverage)
		if series == nil || len(series.Points) < d.config.MinPoints {
			continue
		}

		// Compute label and metric ID from tags
		label := detectorSeriesLabel(key)
		metricID := detectorMetricID(key)

		// Compute variance for ranking
		v := corrComputeVariance(series.Points)
		if v < 1e-12 {
			continue // skip constant series
		}

		// Boost variance for series with service tags — application metrics
		// are more likely to have service tags than infrastructure metrics.
		if d.config.PreferServiceTags && d.config.ServiceTagBoost > 0 {
			if detectorHasServiceTag(key.Tags) {
				v *= d.config.ServiceTagBoost
			}
		}

		candidates = append(candidates, corrSeriesInfo{
			key:      key,
			label:    label,
			metricID: metricID,
			points:   series.Points,
			variance: v,
		})
	}

	if len(candidates) < 2 {
		return observer.DetectionResult{}
	}

	// Step 3: Select top-K by variance
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].variance > candidates[j].variance
	})
	if len(candidates) > d.config.MaxSeries {
		candidates = candidates[:d.config.MaxSeries]
	}

	log.Printf("  Correlation: analyzing %d series (of %d total keys)", len(candidates), len(allKeys))

	// Step 4: Align series by timestamp
	aligned := d.corrAlignSeries(candidates)
	if len(aligned.timestamps) < 2*d.config.WindowSize {
		log.Printf("  Correlation: only %d timestamps, need >= %d", len(aligned.timestamps), 2*d.config.WindowSize)
		return observer.DetectionResult{}
	}

	log.Printf("  Correlation: aligned to %d timestamps", len(aligned.timestamps))

	// Step 5: Compute correlation matrices in sliding windows and detect changes
	return d.detectChanges(aligned, candidates)
}

// corrAlignedData holds multiple series aligned to a common timestamp grid.
type corrAlignedData struct {
	timestamps []int64
	// values[seriesIdx][timeIdx] = value
	values [][]float64
}

// corrAlignSeries aligns multiple series to a common timestamp grid using forward-fill.
func (d *CorrelationDetector) corrAlignSeries(series []corrSeriesInfo) corrAlignedData {
	// Collect all unique timestamps
	tsSet := make(map[int64]struct{})
	for _, s := range series {
		for _, p := range s.points {
			tsSet[p.Timestamp] = struct{}{}
		}
	}

	timestamps := make([]int64, 0, len(tsSet))
	for ts := range tsSet {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	n := len(series)
	values := make([][]float64, n)

	for i, s := range series {
		tsMap := make(map[int64]float64, len(s.points))
		for _, p := range s.points {
			tsMap[p.Timestamp] = p.Value
		}

		vals := make([]float64, len(timestamps))
		lastVal := math.NaN()
		for j, ts := range timestamps {
			if v, ok := tsMap[ts]; ok {
				vals[j] = v
				lastVal = v
			} else if !math.IsNaN(lastVal) {
				vals[j] = lastVal
			} else {
				vals[j] = 0
			}
		}
		values[i] = vals
	}

	return corrAlignedData{timestamps: timestamps, values: values}
}

// corrPairChange tracks how much a specific pair's correlation changed.
type corrPairChange struct {
	seriesA int
	seriesB int
	before  float64
	after   float64
	delta   float64
}

// detectChanges computes correlation matrices in sliding windows and detects changes.
// Uses a dual approach: consecutive window comparison (catches sudden shifts) and
// baseline comparison (catches gradual drift from normal correlation structure).
func (d *CorrelationDetector) detectChanges(data corrAlignedData, series []corrSeriesInfo) observer.DetectionResult {
	var anomalies []observer.Anomaly
	var telemetry []observer.ObserverTelemetry

	nSeries := len(series)
	nTime := len(data.timestamps)
	winSize := d.config.WindowSize
	stepSize := d.config.StepSize

	if nTime < 2*winSize {
		return observer.DetectionResult{}
	}

	// Phase 1: Compute all window matrices
	type windowInfo struct {
		start, end int
		timestamp  int64
		matrix     [][]float64
	}
	var windows []windowInfo
	for start := 0; start+winSize <= nTime; start += stepSize {
		end := start + winSize
		windows = append(windows, windowInfo{
			start:     start,
			end:       end,
			timestamp: data.timestamps[end-1],
			matrix:    corrComputeMatrix(data.values, start, end),
		})
	}

	if len(windows) < 2 {
		return observer.DetectionResult{}
	}

	// Phase 2: Compute baseline matrix as the average of the first ~30% of windows
	baselineCount := len(windows) / 3
	if baselineCount < 3 {
		baselineCount = 3
	}
	if baselineCount > len(windows)/2 {
		baselineCount = len(windows) / 2
	}
	baselineMatrices := make([][][]float64, baselineCount)
	for i := 0; i < baselineCount; i++ {
		baselineMatrices[i] = windows[i].matrix
	}
	baselineMatrix := corrAverageMatrices(baselineMatrices, nSeries)

	// Phase 3: Compute norms — both consecutive and vs-baseline
	type normEntry struct {
		timestamp    int64
		consNorm     float64 // vs previous window
		baselineNorm float64 // vs baseline
		maxNorm      float64 // max of the two
		windowIdx    int
	}
	var norms []normEntry

	for i := 1; i < len(windows); i++ {
		consNorm := corrFrobeniusNorm(windows[i-1].matrix, windows[i].matrix, nSeries)
		baseNorm := corrFrobeniusNorm(baselineMatrix, windows[i].matrix, nSeries)
		maxNorm := consNorm
		if baseNorm > maxNorm {
			maxNorm = baseNorm
		}

		norms = append(norms, normEntry{
			timestamp:    windows[i].timestamp,
			consNorm:     consNorm,
			baselineNorm: baseNorm,
			maxNorm:      maxNorm,
			windowIdx:    i,
		})
	}

	// Phase 4: Threshold computation and anomaly detection.
	// When BaselineThresholdFraction > 0, compute a fixed threshold from the first N% of norms.
	// This prevents gradual drift from defeating detection (the food_delivery problem).
	var fixedThreshold float64
	var fixedThresholdSet bool
	if d.config.BaselineThresholdFraction > 0 && len(norms) > 0 {
		baselineNormCount := int(float64(len(norms)) * d.config.BaselineThresholdFraction)
		if baselineNormCount < 3 {
			baselineNormCount = 3
		}
		if baselineNormCount > len(norms) {
			baselineNormCount = len(norms)
		}
		baselineNormValues := make([]float64, baselineNormCount)
		for i := 0; i < baselineNormCount; i++ {
			baselineNormValues[i] = norms[i].maxNorm
		}
		fixedThreshold = corrComputeThreshold(baselineNormValues, d.config.ThresholdSigma)
		fixedThresholdSet = fixedThreshold > 0
		log.Printf("  Correlation: fixed baseline threshold=%.4f (from %d/%d norms)", fixedThreshold, baselineNormCount, len(norms))
	}

	windowCount := 0
	for _, ne := range norms {
		d.recentNorms = append(d.recentNorms, ne.maxNorm)
		if len(d.recentNorms) > 200 {
			d.recentNorms = d.recentNorms[1:]
		}

		telemetry = append(telemetry, observer.ObserverTelemetry{
			DetectorName: d.Name(),
			Metric: &metricObs{
				name:      "frob_norm",
				value:     ne.maxNorm,
				timestamp: ne.timestamp,
			},
		})
		telemetry = append(telemetry, observer.ObserverTelemetry{
			DetectorName: d.Name(),
			Metric: &metricObs{
				name:      "baseline_norm",
				value:     ne.baselineNorm,
				timestamp: ne.timestamp,
			},
		})

		// Use fixed baseline threshold if configured, otherwise fall back to rolling.
		var threshold float64
		if fixedThresholdSet {
			threshold = fixedThreshold
		} else {
			threshold = corrComputeThreshold(d.recentNorms, d.config.ThresholdSigma)
		}
		if threshold > 0 {
			telemetry = append(telemetry, observer.ObserverTelemetry{
				DetectorName: d.Name(),
				Metric: &metricObs{
					name:      "threshold",
					value:     threshold,
					timestamp: ne.timestamp,
				},
			})
		}

		// Check for anomaly
		if ne.maxNorm >= d.config.MinFrobeniusDelta && threshold > 0 && ne.maxNorm > threshold {
			// Determine which matrix to compare against for pair analysis
			refMatrix := windows[ne.windowIdx-1].matrix
			if ne.baselineNorm > ne.consNorm {
				refMatrix = baselineMatrix
			}
			curMatrix := windows[ne.windowIdx].matrix

			topPairs := d.corrFindTopPairs(refMatrix, curMatrix, nSeries)

			involvedSeries := make(map[int]float64)
			for _, p := range topPairs {
				if delta, ok := involvedSeries[p.seriesA]; !ok || p.delta > delta {
					involvedSeries[p.seriesA] = p.delta
				}
				if delta, ok := involvedSeries[p.seriesB]; !ok || p.delta > delta {
					involvedSeries[p.seriesB] = p.delta
				}
			}

			for sIdx, maxDelta := range involvedSeries {
				fireKey := fmt.Sprintf("%s@%d", series[sIdx].metricID, ne.timestamp)
				if d.firedSeries[fireKey] {
					continue
				}
				d.firedSeries[fireKey] = true

				var pairDescs []string
				for _, p := range topPairs {
					if p.seriesA == sIdx || p.seriesB == sIdx {
						other := p.seriesB
						if p.seriesB == sIdx {
							other = p.seriesA
						}
						pairDescs = append(pairDescs, fmt.Sprintf("%s (corr %.2f->%.2f)",
							series[other].label, p.before, p.after))
					}
				}

				desc := fmt.Sprintf("Correlation structure change for %s (norm=%.3f, threshold=%.3f). Correlated with: %s",
					series[sIdx].label, ne.maxNorm, threshold,
					strings.Join(pairDescs, "; "))

				score := maxDelta
				seriesKey := series[sIdx].key
				sourceSeriesID := observer.SeriesID(
					seriesKey.Namespace + "|" + seriesKey.Name + "|" + strings.Join(seriesKey.Tags, ","))

				rMean := corrSliceMean(d.recentNorms)
				rStd := corrSliceStddev(d.recentNorms, rMean)

				anomaly := observer.Anomaly{
					Type:           observer.AnomalyTypeMetric,
					Source:         observer.MetricName(series[sIdx].metricID),
					SourceSeriesID: sourceSeriesID,
					DetectorName:   d.Name(),
					Title:          "Cross-correlation changepoint",
					Description:    desc,
					Tags:           seriesKey.Tags,
					Timestamp:      ne.timestamp,
					Score:          &score,
					DebugInfo: &observer.AnomalyDebugInfo{
						CurrentValue:   ne.maxNorm,
						Threshold:      threshold,
						DeviationSigma: (ne.maxNorm - rMean) / math.Max(rStd, 0.001),
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
		windowCount++
	}

	// Debug summary
	maxNorm := 0.0
	for _, n := range d.recentNorms {
		if n > maxNorm {
			maxNorm = n
		}
	}
	finalThreshold := corrComputeThreshold(d.recentNorms, d.config.ThresholdSigma)
	log.Printf("  Correlation: %d windows compared, maxNorm=%.4f, threshold=%.4f, %d anomalies emitted",
		windowCount, maxNorm, finalThreshold, len(anomalies))

	return observer.DetectionResult{
		Anomalies: anomalies,
		Telemetry: telemetry,
	}
}

// corrComputeMatrix computes pairwise Pearson correlations for the given time window.
func corrComputeMatrix(values [][]float64, start, end int) [][]float64 {
	n := len(values)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0
	}

	windowLen := end - start
	means := make([]float64, n)
	stddevs := make([]float64, n)

	for i := 0; i < n; i++ {
		sum := 0.0
		for t := start; t < end; t++ {
			sum += values[i][t]
		}
		means[i] = sum / float64(windowLen)

		sumSq := 0.0
		for t := start; t < end; t++ {
			diff := values[i][t] - means[i]
			sumSq += diff * diff
		}
		stddevs[i] = math.Sqrt(sumSq / float64(windowLen))
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if stddevs[i] < 1e-12 || stddevs[j] < 1e-12 {
				continue // leave as 0
			}

			cov := 0.0
			for t := start; t < end; t++ {
				cov += (values[i][t] - means[i]) * (values[j][t] - means[j])
			}
			cov /= float64(windowLen)

			corr := cov / (stddevs[i] * stddevs[j])
			if corr > 1 {
				corr = 1
			} else if corr < -1 {
				corr = -1
			}
			matrix[i][j] = corr
			matrix[j][i] = corr
		}
	}

	return matrix
}

// corrAverageMatrices computes the element-wise average of multiple correlation matrices.
func corrAverageMatrices(matrices [][][]float64, n int) [][]float64 {
	avg := make([][]float64, n)
	for i := range avg {
		avg[i] = make([]float64, n)
	}
	count := float64(len(matrices))
	for _, m := range matrices {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				avg[i][j] += m[i][j]
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			avg[i][j] /= count
		}
	}
	return avg
}

// corrFrobeniusNorm computes the normalized Frobenius norm of the difference between two matrices.
func corrFrobeniusNorm(a, b [][]float64, n int) float64 {
	sumSq := 0.0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			diff := a[i][j] - b[i][j]
			sumSq += diff * diff
		}
	}
	nPairs := float64(n*(n-1)) / 2.0
	if nPairs == 0 {
		return 0
	}
	return math.Sqrt(sumSq / nPairs)
}

// corrFindTopPairs returns the pairs whose correlation changed the most.
// If no pairs meet PairDeltaMin but the Frobenius norm was significant,
// falls back to returning the top pairs by raw delta regardless of minimum.
func (d *CorrelationDetector) corrFindTopPairs(before, after [][]float64, n int) []corrPairChange {
	var pairs []corrPairChange
	var allPairs []corrPairChange
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			delta := math.Abs(after[i][j] - before[i][j])
			pc := corrPairChange{
				seriesA: i,
				seriesB: j,
				before:  before[i][j],
				after:   after[i][j],
				delta:   delta,
			}
			if delta >= d.config.PairDeltaMin {
				pairs = append(pairs, pc)
			}
			if delta > 0.01 {
				allPairs = append(allPairs, pc)
			}
		}
	}

	// Fallback: if no pairs meet PairDeltaMin, use the top pairs by raw delta.
	// This ensures we still emit anomalies when many small correlation changes
	// aggregate into a significant Frobenius norm.
	if len(pairs) == 0 && len(allPairs) > 0 {
		pairs = allPairs
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].delta > pairs[j].delta
	})

	if len(pairs) > d.config.TopPairsToReport {
		pairs = pairs[:d.config.TopPairsToReport]
	}
	return pairs
}

// corrComputeThreshold computes mean + sigma*stddev from a slice of baseline norm values.
// Returns 0 if there are fewer than 3 values.
func corrComputeThreshold(values []float64, sigma float64) float64 {
	if len(values) < 3 {
		return 0
	}
	mean := corrSliceMean(values)
	std := corrSliceStddev(values, mean)
	return mean + sigma*std
}

// corrSliceMean computes the mean of a float64 slice.
func corrSliceMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// corrSliceStddev computes the population standard deviation of a float64 slice.
func corrSliceStddev(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range values {
		dd := v - mean
		sumSq += dd * dd
	}
	return math.Sqrt(sumSq / float64(len(values)))
}

// corrComputeVariance computes the variance of a point series.
func corrComputeVariance(points []observer.Point) float64 {
	if len(points) < 2 {
		return 0
	}
	sum := 0.0
	for _, p := range points {
		sum += p.Value
	}
	mean := sum / float64(len(points))

	sumSq := 0.0
	for _, p := range points {
		diff := p.Value - mean
		sumSq += diff * diff
	}
	return sumSq / float64(len(points))
}

// corrIsExcluded checks whether a metric name matches any of the configured exclude prefixes.
func (d *CorrelationDetector) corrIsExcluded(name string) bool {
	for _, prefix := range d.config.ExcludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
