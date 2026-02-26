// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"gonum.org/v1/gonum/stat/distuv"
)

// LightESDConfig configures the LightESD anomaly detector.
// LightESD (Lightweight Extreme Studentized Deviate) is a statistical
// anomaly detection framework designed for edge computing.
//
// Paper: "LightESD: Fully-Automated and Lightweight Anomaly Detection
// Framework for Edge Computing" (Das & Luo, IEEE EDGE 2023)
// https://arxiv.org/pdf/2305.12266
type LightESDConfig struct {
	// MinWindowSize is the minimum number of points required for detection.
	// Should be large enough for meaningful decomposition (default: 50).
	MinWindowSize int

	// MaxOutliers is the maximum number of outliers to detect (parameter 'r' in GESD).
	// Default: 10% of window size
	MaxOutliers int

	// Alpha is the significance level for GESD test (default: 0.05 = 95% confidence).
	Alpha float64

	// EnablePeriodicity enables periodicity detection and seasonal decomposition.
	// If false, only applies GESD to detrended data (simpler, faster).
	// Default: true
	EnablePeriodicity bool

	// TrendWindowFraction is the fraction of data used for robust trend smoothing.
	// Default: 0.15 (15% of window)
	TrendWindowFraction float64

	// PeriodicitySignificance is the p-value threshold for periodicity detection.
	// Lower = more strict (fewer false periodicities). Default: 0.01
	PeriodicitySignificance float64

	// MaxPeriods is the maximum number of seasonal components to extract.
	// Default: 2 (e.g., daily + weekly)
	MaxPeriods int
}

// DefaultLightESDConfig returns a LightESDConfig with sensible defaults.
func DefaultLightESDConfig() LightESDConfig {
	return LightESDConfig{
		MinWindowSize:           50,
		MaxOutliers:             0, // Will be set to 10% of actual window size
		Alpha:                   0.05,
		EnablePeriodicity:       true,
		TrendWindowFraction:     0.15,
		PeriodicitySignificance: 0.01,
		MaxPeriods:              2,
	}
}

// LightESDEmitter detects anomalies using the LightESD algorithm.
// It implements the TimeSeriesAnalysis interface.
//
// Algorithm (from paper):
//  1. Periodicity Detection: Welch periodogram + permutation-based significance testing
//  2. Seasonal-Trend Decomposition: Robust trend extraction + multi-seasonal decomposition
//  3. Generalized ESD Test: Robust outlier detection in residual using median + MAD
//
// Key improvements over naive implementation:
//   - Uses Welch PSD (not autocorrelation) for periodicity
//   - Robust trend via median filtering (not moving average)
//   - Robust ESD with median + MAD (not mean + stddev)
//   - Proper t-distribution critical values (not approximations)
//   - Multi-seasonality support
type LightESDEmitter struct {
	config LightESDConfig
}

// NewLightESDEmitter creates a new LightESD anomaly detector.
func NewLightESDEmitter(config LightESDConfig) *LightESDEmitter {
	return &LightESDEmitter{
		config: config,
	}
}

// Name returns the emitter name for debugging.
func (l *LightESDEmitter) Name() string {
	return "lightesd"
}

// Analyze examines a time series for outliers using robust GESD after seasonal-trend decomposition.
func (l *LightESDEmitter) Analyze(series observer.Series) observer.TimeSeriesAnalysisResult {
	if len(series.Points) < l.config.MinWindowSize {
		return observer.TimeSeriesAnalysisResult{}
	}

	// Extract values for processing
	values := make([]float64, len(series.Points))
	for i, p := range series.Points {
		values[i] = p.Value
	}

	// Phase 1 & 2: Decomposition (robust trend + optional multi-seasonal)
	residual := l.robustDecompose(values)

	// Phase 3: Apply Robust Generalized ESD test to residual
	maxOutliers := l.config.MaxOutliers
	if maxOutliers == 0 {
		maxOutliers = int(float64(len(values)) * 0.1) // Default: 10% of data
		if maxOutliers < 1 {
			maxOutliers = 1
		}
	}

	outlierIndices := robustGeneralizedESD(residual, maxOutliers, l.config.Alpha)

	// Convert outlier indices to AnomalyOutput
	var anomalies []observer.AnomalyOutput
	for _, idx := range outlierIndices {
		// Calculate score using robust statistics (median + MAD)
		median := computeMedian(residual)
		mad := medianAbsoluteDeviation(residual)
		sigma := math.Abs(residual[idx]-median) / (mad*1.4826 + 1e-10) // MAD to stddev conversion

		ts := series.Points[idx].Timestamp
		anomalies = append(anomalies, observer.AnomalyOutput{
			Source:      observer.MetricName(series.Name),
			Title:       fmt.Sprintf("LightESD: %s", series.Name),
			Description: fmt.Sprintf("%s (score: %.2f) at timestamp %d", series.Name, sigma, ts),
			Tags:        series.Tags,
			Timestamp:   ts,
			TimeRange: observer.TimeRange{
				Start: ts,
				End:   ts,
			},
			DebugInfo: &observer.AnomalyDebugInfo{
				CurrentValue:   series.Points[idx].Value,
				DeviationSigma: sigma,
			},
		})
	}

	return observer.TimeSeriesAnalysisResult{Anomalies: anomalies}
}

// robustDecompose extracts trend and optionally seasonal components using robust methods.
// Returns the residual after removing trend and seasonality.
func (l *LightESDEmitter) robustDecompose(values []float64) []float64 {
	n := len(values)

	// Step 1: Extract robust trend using median filter (approximation of RobustTrend)
	// Use small window to avoid smoothing away outliers
	trendWindowSize := int(float64(n) * l.config.TrendWindowFraction)
	if trendWindowSize < 3 {
		trendWindowSize = 3
	}
	// Cap at 11 to avoid overly aggressive smoothing
	if trendWindowSize > 11 {
		trendWindowSize = 11
	}
	if trendWindowSize%2 == 0 {
		trendWindowSize++ // Make it odd for symmetric window
	}

	trend := extractRobustTrend(values, trendWindowSize)

	// Step 2: Detrend
	detrended := make([]float64, n)
	for i := 0; i < n; i++ {
		detrended[i] = values[i] - trend[i]
	}

	// Step 3: Extract seasonal components (if enabled)
	var residual []float64
	if l.config.EnablePeriodicity {
		// Detect multiple periods using Welch periodogram
		periods := l.detectPeriodsWelch(detrended)

		if len(periods) > 0 {
			// Extract seasonal components for each detected period
			currentResidual := detrended
			for i, period := range periods {
				if i >= l.config.MaxPeriods {
					break
				}
				seasonal := extractRobustSeasonal(currentResidual, period)
				newResidual := make([]float64, n)
				for j := 0; j < n; j++ {
					newResidual[j] = currentResidual[j] - seasonal[j]
				}
				currentResidual = newResidual
			}
			residual = currentResidual
		} else {
			// No significant periodicity found
			residual = detrended
		}
	} else {
		// Periodicity disabled
		residual = detrended
	}

	return residual
}

// extractRobustTrend computes a robust trend using median filtering.
// This is more robust to outliers than moving average.
func extractRobustTrend(values []float64, windowSize int) []float64 {
	n := len(values)
	trend := make([]float64, n)
	halfWindow := windowSize / 2

	for i := 0; i < n; i++ {
		// Calculate window bounds
		start := i - halfWindow
		end := i + halfWindow + 1

		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}

		// Extract window and compute median (robust to outliers)
		window := values[start:end]
		trend[i] = computeMedian(window)
	}

	return trend
}

// detectPeriodsWelch detects significant periods using Welch's periodogram
// with permutation-based significance testing (as described in LightESD paper).
func (l *LightESDEmitter) detectPeriodsWelch(values []float64) []int {
	n := len(values)
	if n < 20 {
		return nil
	}

	// Compute Power Spectral Density using Welch's method
	// Simplified: Use full FFT (Welch would segment and average)
	psd := computeSimplePSD(values)

	// Find peaks in PSD
	peaks := findPSDPeaks(psd)

	// Test each peak for significance using permutation test
	var significantPeriods []int
	numPermutations := 100 // For speed; paper uses more

	for _, peakIdx := range peaks {
		period := n / peakIdx // Convert frequency index to period
		if period < 2 || period > n/2 {
			continue
		}

		// Compute test statistic: PSD value at peak
		testStat := psd[peakIdx]

		// Permutation test: shuffle data and recompute PSD many times
		exceedCount := 0
		for perm := 0; perm < numPermutations; perm++ {
			shuffled := permute(values, perm) // Deterministic permutation
			permPSD := computeSimplePSD(shuffled)
			if permPSD[peakIdx] >= testStat {
				exceedCount++
			}
		}

		pValue := float64(exceedCount) / float64(numPermutations)
		if pValue < l.config.PeriodicitySignificance {
			significantPeriods = append(significantPeriods, period)
		}

		if len(significantPeriods) >= l.config.MaxPeriods {
			break
		}
	}

	return significantPeriods
}

// computeSimplePSD computes a simplified Power Spectral Density.
// This is a basic DFT-based PSD; full Welch would segment and average.
func computeSimplePSD(values []float64) []float64 {
	n := len(values)

	// Remove mean (important for PSD)
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	centered := make([]float64, n)
	for i, v := range values {
		centered[i] = v - mean
	}

	// Compute DFT magnitudes (simplified FFT)
	psd := make([]float64, n/2)
	for k := 0; k < n/2; k++ {
		real, imag := 0.0, 0.0
		for t := 0; t < n; t++ {
			angle := 2 * math.Pi * float64(k) * float64(t) / float64(n)
			real += centered[t] * math.Cos(angle)
			imag += centered[t] * math.Sin(angle)
		}
		psd[k] = real*real + imag*imag
	}

	return psd
}

// findPSDPeaks finds local maxima in PSD spectrum.
func findPSDPeaks(psd []float64) []int {
	var peaks []int
	n := len(psd)

	for i := 1; i < n-1; i++ {
		// Simple peak detection: value > neighbors
		if psd[i] > psd[i-1] && psd[i] > psd[i+1] {
			peaks = append(peaks, i)
		}
	}

	// Sort peaks by magnitude (descending)
	sort.Slice(peaks, func(i, j int) bool {
		return psd[peaks[i]] > psd[peaks[j]]
	})

	return peaks
}

// permute creates a deterministic permutation of data for permutation testing.
func permute(data []float64, seed int) []float64 {
	n := len(data)
	result := make([]float64, n)
	copy(result, data)

	// Simple deterministic shuffle based on seed
	for i := n - 1; i > 0; i-- {
		j := (seed * (i + 1)) % (i + 1)
		result[i], result[j] = result[j], result[i]
		seed = (seed*1103515245 + 12345) & 0x7fffffff // LCG
	}

	return result
}

// extractRobustSeasonal extracts seasonal component using robust cycle averaging.
func extractRobustSeasonal(detrended []float64, period int) []float64 {
	n := len(detrended)
	seasonal := make([]float64, n)

	// Use median (not mean) for each position within cycle (robust to outliers)
	for pos := 0; pos < period; pos++ {
		var cycleValues []float64
		for i := pos; i < n; i += period {
			cycleValues = append(cycleValues, detrended[i])
		}
		if len(cycleValues) > 0 {
			cycleMedian := computeMedian(cycleValues)
			for i := pos; i < n; i += period {
				seasonal[i] = cycleMedian
			}
		}
	}

	// Center the seasonal component (median = 0)
	seasonalMedian := computeMedian(seasonal)
	for i := 0; i < n; i++ {
		seasonal[i] -= seasonalMedian
	}

	return seasonal
}

// robustGeneralizedESD performs the Generalized Extreme Studentized Deviate test
// using ROBUST statistics (median + MAD instead of mean + stddev).
// Returns indices of detected outliers.
//
// Algorithm from Rosner (1983) with robustness modifications:
//  1. For k = 1 to maxOutliers:
//     - Compute robust test statistic R_k = max|x_i - median| / MAD
//     - Remove the most extreme point
//  2. Compare R_k against critical values from t-distribution
//  3. Find largest k where R_k > critical value
func robustGeneralizedESD(data []float64, maxOutliers int, alpha float64) []int {
	n := len(data)
	if n < 3 || maxOutliers < 1 {
		return nil
	}

	// Make a copy to avoid modifying original
	workingData := make([]float64, n)
	copy(workingData, data)

	// Track which indices have been removed
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		indices[i] = i
	}

	var outlierIndices []int
	var testStats []float64

	// Iteratively remove outliers
	for k := 0; k < maxOutliers && len(workingData) > 2; k++ {
		// Compute ROBUST center and scale (median + MAD)
		median := computeMedian(workingData)
		mad := medianAbsoluteDeviation(workingData)

		// Convert MAD to equivalent standard deviation (for normal distribution)
		// Use epsilon to handle case where most values are identical (MAD ≈ 0)
		// In this case, use a small fraction of the data range as the scale
		robustScale := mad * 1.4826
		if robustScale < 1e-10 {
			// Fallback: use range-based scale when MAD is too small
			dataMin, dataMax := math.Inf(1), math.Inf(-1)
			for _, v := range workingData {
				if v < dataMin {
					dataMin = v
				}
				if v > dataMax {
					dataMax = v
				}
			}
			dataRange := dataMax - dataMin
			if dataRange < 1e-10 {
				break // Truly all identical
			}
			// Use ~1% of range as scale (conservative)
			robustScale = dataRange * 0.01
		}

		// Find point with maximum absolute deviation from median
		maxIdx := 0
		maxDev := 0.0

		for i, val := range workingData {
			dev := math.Abs(val - median)
			if dev > maxDev {
				maxDev = dev
				maxIdx = i
			}
		}

		// Compute robust test statistic R
		R := maxDev / robustScale
		testStats = append(testStats, R)

		// Store the index in original data
		outlierIndices = append(outlierIndices, indices[maxIdx])

		// Remove the outlier from working data
		workingData = append(workingData[:maxIdx], workingData[maxIdx+1:]...)
		indices = append(indices[:maxIdx], indices[maxIdx+1:]...)
	}

	// Determine how many outliers are significant using critical values
	numOutliers := 0
	for k := 0; k < len(testStats); k++ {
		// Critical value from t-distribution (Rosner's formula)
		// p-value: 1 - alpha/(2*(n-k+1)) for Bonferroni correction
		nk := n - k
		p := 1.0 - alpha/float64(2*(nk))
		df := float64(nk - 2)

		if df < 1 {
			break
		}

		// Get t critical value using proper t-distribution
		tDist := distuv.StudentsT{Mu: 0, Sigma: 1, Nu: df}
		tCrit := tDist.Quantile(p)

		// Compute lambda (critical value for ESD test)
		// lambda = ((n-k-1) * t_p) / sqrt((n-k-2 + t_p^2) * (n-k))
		// FIX: Keep all as float, no int casts!
		numerator := float64(nk-1) * tCrit
		denominator := math.Sqrt((df + tCrit*tCrit) * float64(nk))
		lambda := numerator / denominator

		if testStats[k] > lambda {
			numOutliers = k + 1
		}
	}

	// Return only the significant outliers
	if numOutliers == 0 {
		return nil
	}

	return outlierIndices[:numOutliers]
}

// computeMedian computes the median of a dataset.
func computeMedian(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// medianAbsoluteDeviation computes the MAD (robust alternative to stddev).
// MAD = median(|x_i - median(x)|)
func medianAbsoluteDeviation(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	// Compute median
	median := computeMedian(data)

	// Compute absolute deviations
	absDevs := make([]float64, len(data))
	for i, v := range data {
		absDevs[i] = math.Abs(v - median)
	}

	// Compute median of absolute deviations
	return computeMedian(absDevs)
}
