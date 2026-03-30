// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// seriesStatus holds point count and write generation for a single series.
// Used by bulkSeriesStatus and scan-based detectors.
type seriesStatus struct {
	pointCount      int
	writeGeneration int64
}

// bulkStatusReader is an optional optimization interface for StorageReader
// implementations that support batch status queries in a single lock acquisition.
type bulkStatusReader interface {
	BulkSeriesStatus(refs []observer.SeriesRef, endTime int64) []seriesStatus
}

// bulkSeriesStatus returns the point count and write generation for each ref.
// If storage implements bulkStatusReader (e.g. timeSeriesStorage), it uses a
// single lock acquisition. Otherwise falls back to individual PointCountUpTo +
// WriteGeneration calls per ref.
func bulkSeriesStatus(storage observer.StorageReader, refs []observer.SeriesRef, endTime int64) []seriesStatus {
	if br, ok := storage.(bulkStatusReader); ok {
		return br.BulkSeriesStatus(refs, endTime)
	}
	// Fallback: individual calls (2 lock acquisitions per ref).
	result := make([]seriesStatus, len(refs))
	for i, h := range refs {
		result[i] = seriesStatus{
			pointCount:      storage.PointCountUpTo(h, endTime),
			writeGeneration: storage.WriteGeneration(h),
		}
	}
	return result
}

// detectorMedian computes the median of a float64 slice without modifying the input.
func detectorMedian(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// detectorMAD computes the Median Absolute Deviation from a given median.
// MAD = median(|x_i - median|).
// When scaleToSigma is true, the result is scaled by 1.4826 to estimate the
// standard deviation for normally distributed data. Use scaleToSigma=true when
// comparing against sigma-based thresholds (e.g. Mann-Whitney's deviation check),
// and false when using raw MAD as a denominator for relative change scores (e.g. TopK).
func detectorMAD(vals []float64, median float64, scaleToSigma bool) float64 {
	if len(vals) == 0 {
		return 0
	}
	absDevs := make([]float64, len(vals))
	for i, v := range vals {
		absDevs[i] = math.Abs(v - median)
	}
	sort.Float64s(absDevs)
	n := len(absDevs)
	var mad float64
	if n%2 == 0 {
		mad = (absDevs[n/2-1] + absDevs[n/2]) / 2
	} else {
		mad = absDevs[n/2]
	}
	if scaleToSigma {
		mad *= 1.4826
	}
	return mad
}

// medianPointInterval computes the median gap between consecutive point
// timestamps. Returns 0 if fewer than 2 points.
//
// Perf note: this is O(N log N) due to the sort. For hot paths it could be
// replaced with O(N) mean: (last-first)/(len-1), or O(1) if the storage
// tracks per-series intervals. N is typically 30-100 (MinPoints), so the
// sort is negligible in practice.
func medianPointInterval(points []observer.Point) int64 {
	if len(points) < 2 {
		return 0
	}
	intervals := make([]int64, len(points)-1)
	for i := 1; i < len(points); i++ {
		intervals[i-1] = points[i].Timestamp - points[i-1].Timestamp
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	return intervals[len(intervals)/2]
}

// rankBiserialCorrelation computes the rank-biserial correlation from a Mann-Whitney U statistic.
// Used by ScanMW and ScanWelch for effect size verification.
func rankBiserialCorrelation(u float64, n1, n2 int) float64 {
	fn1 := float64(n1)
	fn2 := float64(n2)
	product := fn1 * fn2
	if product == 0 {
		return 0
	}
	return 1 - 2*u/product
}

// normalCDFUpper computes P(Z > z) for z >= 0 using the Abramowitz & Stegun approximation.
// Used by ScanMW and ScanWelch for p-value computation.
func normalCDFUpper(z float64) float64 {
	if z < 0 {
		return 1 - normalCDFUpper(-z)
	}
	// Rational approximation (Abramowitz & Stegun 26.2.17)
	const (
		p  = 0.2316419
		b1 = 0.319381530
		b2 = -0.356563782
		b3 = 1.781477937
		b4 = -1.821255978
		b5 = 1.330274429
	)
	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t
	phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
	return phi * (b1*t + b2*t2 + b3*t3 + b4*t4 + b5*t5)
}
