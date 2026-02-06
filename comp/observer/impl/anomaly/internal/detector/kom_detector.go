// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"math"
	"sort"
	"time"
)

// KofMDetector implements robust per-signal thresholding with K-of-M voting
type KofMDetector struct {
	// Hyperparameters
	windowSize           int     // W: number of historical points per signal
	zThreshold           float64 // z_thresh: what counts as "extreme"
	k                    int     // K: minimum number of signals that must be extreme
	persistence          int     // must satisfy condition for N of last 3 points
	suppressMinutes      int     // cooldown period after alert
	minSignalsForHighSev int     // threshold for critical vs warning severity
	minWindowForStats    int     // minimum window size before computing stats

	// State
	windows       [][]float64 // rolling windows per signal [M][W]
	zHistory      [][]float64 // z-score history per signal [M][history_len]
	lastAlertTime time.Time   // timestamp of last alert
	numSignals    int         // M: number of signals
	initialized   bool
}

// NewKofMDetector creates a new K-of-M voting detector with default hyperparameters
func NewKofMDetector() *KofMDetector {
	return NewKofMDetectorWithParams(
		12*24, // W = 2 days of 5-min points (576)
		4.0,   // z_thresh
		0,     // K will be computed based on M
		2,     // persistence
		30,    // suppress_minutes
		50,    // min_window_for_stats
	)
}

// NewKofMDetectorWithParams creates a K-of-M detector with custom parameters
func NewKofMDetectorWithParams(windowSize int, zThreshold float64, k int, persistence int, suppressMinutes int, minWindowForStats int) *KofMDetector {
	return &KofMDetector{
		windowSize:        windowSize,
		zThreshold:        zThreshold,
		k:                 k,
		persistence:       persistence,
		suppressMinutes:   suppressMinutes,
		minWindowForStats: minWindowForStats,
		initialized:       false,
	}
}

// Name returns the detector name
func (d *KofMDetector) Name() string {
	return "K-of-M"
}

// HigherIsAnomalous returns true since higher scores indicate anomalies
func (d *KofMDetector) HigherIsAnomalous() bool {
	return true
}

// ComputeScore processes telemetry results and returns anomaly score
func (d *KofMDetector) ComputeScore(result TelemetryResult) (float64, error) {
	// Initialize on first call
	signals := result.ToArray()

	if !d.initialized {
		d.numSignals = len(signals)
		d.windows = make([][]float64, d.numSignals)
		d.zHistory = make([][]float64, d.numSignals)
		for i := 0; i < d.numSignals; i++ {
			d.windows[i] = make([]float64, 0, d.windowSize)
			d.zHistory[i] = make([]float64, 0, 10) // keep last 10 z-scores
		}

		// Set K based on M if not specified
		if d.k == 0 {
			d.k = int(math.Max(1, math.Ceil(0.2*float64(d.numSignals))))
		}

		// Set min signals for high severity
		d.minSignalsForHighSev = int(math.Ceil(0.5 * float64(d.numSignals)))

		d.initialized = true
	}

	// Update windows and compute z-scores
	for s := 0; s < d.numSignals; s++ {
		// Add new value to window
		d.windows[s] = append(d.windows[s], signals[s])

		// Trim window if too large
		if len(d.windows[s]) > d.windowSize {
			d.windows[s] = d.windows[s][1:]
		}

		// Compute z-score
		var zScore float64
		if len(d.windows[s]) < d.minWindowForStats {
			// Not enough data, use simple z-score with available data
			mean, stddev := computeMeanStd(d.windows[s])
			if stddev > 1e-10 {
				zScore = (signals[s] - mean) / stddev
			}
		} else {
			// Use robust z-score with median and MAD
			median := computeMedian(d.windows[s])
			mad := computeMAD(d.windows[s], median)
			scale := 1.4826 * mad
			if scale < 1e-10 {
				scale = 1e-10
			}
			zScore = (signals[s] - median) / scale
		}

		// Cap z-score to avoid numerical issues
		if zScore > 1e3 {
			zScore = 1e3
		}
		if zScore < -1e3 {
			zScore = -1e3
		}

		// Store z-score history (keep last 10)
		d.zHistory[s] = append(d.zHistory[s], zScore)
		if len(d.zHistory[s]) > 10 {
			d.zHistory[s] = d.zHistory[s][1:]
		}
	}

	// Count signals that are persistently extreme
	persistentExtreme := 0
	extremeSignals := make([]int, 0, d.numSignals)

	for s := 0; s < d.numSignals; s++ {
		if len(d.zHistory[s]) < 1 {
			continue
		}

		// Check persistence: count extreme z-scores in last 3 points
		histLen := len(d.zHistory[s])
		start := histLen - 3
		if start < 0 {
			start = 0
		}

		extremeCount := 0
		for i := start; i < histLen; i++ {
			// Detect negative extremes only (metrics dropping below mean)
			// Since 1.0=normal and 0.0=anomalous, drops are the anomalies
			if d.zHistory[s][i] < -d.zThreshold {
				extremeCount++
			}
		}

		// Check if this signal is persistently extreme
		pointsToCheck := histLen - start
		if pointsToCheck < 3 {
			pointsToCheck = 1 // early in history, just check if current is extreme
		}

		if extremeCount >= d.persistence || (pointsToCheck == 1 && extremeCount >= 1) {
			persistentExtreme++
			extremeSignals = append(extremeSignals, s)
		}
	}

	// Check if we should alert
	now := time.Now()
	suppressDuration := time.Duration(d.suppressMinutes) * time.Minute
	timeSinceLastAlert := now.Sub(d.lastAlertTime)

	if persistentExtreme >= d.k && timeSinceLastAlert > suppressDuration {
		d.lastAlertTime = now

		// Return severity-weighted score
		// critical: 2.0, warning: 1.0, normal: 0.0
		if persistentExtreme >= d.minSignalsForHighSev {
			return 2.0, nil // critical
		}
		return 1.0, nil // warning
	}

	// Return continuous score based on number of extreme signals
	// This allows monitoring even when not alerting
	return float64(persistentExtreme) / float64(d.numSignals), nil
}

// computeMedian returns the median of a slice
func computeMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
}

// computeMAD returns the Median Absolute Deviation
func computeMAD(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}

	absDeviations := make([]float64, len(values))
	for i, v := range values {
		absDeviations[i] = math.Abs(v - median)
	}

	return computeMedian(absDeviations)
}

// computeMeanStd returns mean and standard deviation
func computeMeanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	return mean, math.Sqrt(variance)
}
