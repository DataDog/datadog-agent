// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const scanMaxPoints = 120

// scanDetectorWorkspace is bounded detector-local scratch space for scan
// statistics. Detectors are single-writer, so a workspace can be reused across
// series and aggregations instead of allocating proportional to every scan.
type scanDetectorWorkspace struct {
	values      []float64
	ranks       []float64
	indexed     []scanIndexedValue
	sortScratch []float64
	intervals   []int64
}

type scanIndexedValue struct {
	value float64
	index int
}

func (w *scanDetectorWorkspace) valuesFromPoints(points []observer.Point) []float64 {
	w.values = reuseFloat64(w.values, len(points))
	for i, point := range points {
		w.values[i] = point.Value
	}
	return w.values
}

func (w *scanDetectorWorkspace) assignRanks(values []float64) ([]float64, float64) {
	n := len(values)
	w.indexed = reuseScanIndexedValues(w.indexed, n)
	for i, value := range values {
		w.indexed[i] = scanIndexedValue{value: value, index: i}
	}
	sort.Slice(w.indexed, func(i, j int) bool {
		return w.indexed[i].value < w.indexed[j].value
	})

	w.ranks = reuseFloat64(w.ranks, n)
	tieCorrection := 0.0
	for i := 0; i < n; {
		j := i
		for j < n && w.indexed[j].value == w.indexed[i].value {
			j++
		}
		avgRank := float64(i+1+j) / 2.0
		tieSize := float64(j - i)
		for k := i; k < j; k++ {
			w.ranks[w.indexed[k].index] = avgRank
		}
		tieCorrection += tieSize*tieSize*tieSize - tieSize
		i = j
	}
	return w.ranks, tieCorrection
}

func (w *scanDetectorWorkspace) median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	w.sortScratch = reuseFloat64(w.sortScratch, len(values))
	copy(w.sortScratch, values)
	sort.Float64s(w.sortScratch)
	n := len(w.sortScratch)
	if n%2 == 0 {
		return (w.sortScratch[n/2-1] + w.sortScratch[n/2]) / 2
	}
	return w.sortScratch[n/2]
}

func (w *scanDetectorWorkspace) mad(values []float64, median float64) float64 {
	if len(values) == 0 {
		return 0
	}
	w.sortScratch = reuseFloat64(w.sortScratch, len(values))
	for i, value := range values {
		w.sortScratch[i] = math.Abs(value - median)
	}
	sort.Float64s(w.sortScratch)
	n := len(w.sortScratch)
	if n%2 == 0 {
		return (w.sortScratch[n/2-1] + w.sortScratch[n/2]) / 2
	}
	return w.sortScratch[n/2]
}

func (w *scanDetectorWorkspace) medianPointInterval(points []observer.Point) int64 {
	if len(points) < 2 {
		return 0
	}
	w.intervals = reuseInt64(w.intervals, len(points)-1)
	for i := 1; i < len(points); i++ {
		w.intervals[i-1] = points[i].Timestamp - points[i-1].Timestamp
	}
	sort.Slice(w.intervals, func(i, j int) bool { return w.intervals[i] < w.intervals[j] })
	return w.intervals[len(w.intervals)/2]
}

func reuseFloat64(dst []float64, size int) []float64 {
	if cap(dst) < size {
		return make([]float64, size)
	}
	return dst[:size]
}

func reuseInt64(dst []int64, size int) []int64 {
	if cap(dst) < size {
		return make([]int64, size)
	}
	return dst[:size]
}

func reuseScanIndexedValues(dst []scanIndexedValue, size int) []scanIndexedValue {
	if cap(dst) < size {
		return make([]scanIndexedValue, size)
	}
	return dst[:size]
}

// appendPointWindow retains the newest maxPoints points in buf.
func appendPointWindow(buf []observer.Point, maxPoints int, point observer.Point) []observer.Point {
	if len(buf) < maxPoints {
		return append(buf, point)
	}
	copy(buf, buf[1:])
	buf[len(buf)-1] = point
	return buf
}

func parseAggregateConfig(names []string) []observer.Aggregate {
	if len(names) == 0 {
		return nil
	}
	aggregations := make([]observer.Aggregate, 0, len(names))
	for _, name := range names {
		if agg, ok := parseAggregateSuffix(name); ok {
			aggregations = append(aggregations, agg)
		}
	}
	return aggregations
}

func parseAggregateSuffix(s string) (observer.Aggregate, bool) {
	switch s {
	case "avg":
		return observer.AggregateAverage, true
	case "sum":
		return observer.AggregateSum, true
	case "count":
		return observer.AggregateCount, true
	default:
		return 0, false
	}
}

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

// seriesRefLister is an optional optimization interface for StorageReader
// implementations that can list matching series refs without materializing
// full SeriesMeta values. The dst slice may be reused for the returned refs.
type seriesRefLister interface {
	ListSeriesRefsInto(filter observer.SeriesFilter, dst []observer.SeriesRef) []observer.SeriesRef
}

// seriesAggregateSupport is an optional policy for series whose stored point
// representation gives only some aggregations useful semantics.
type seriesAggregateSupport interface {
	SupportsAggregate(ref observer.SeriesRef, agg observer.Aggregate) bool
}

// tailPointReader is implemented by storage that can snapshot a bounded tail
// without first materializing the whole retained range.
type tailPointReader interface {
	ForEachLastPoints(observer.SeriesRef, int64, int, observer.Aggregate, func(*observer.Series, observer.Point)) bool
}

// collectLastPoints appends the newest maxPoints visible points to dst and
// returns their series metadata. Production storage uses its bounded tail
// primitive; the fallback keeps StorageReader test doubles compatible.
func collectLastPoints(storage observer.StorageReader, ref observer.SeriesRef, end int64, maxPoints int, agg observer.Aggregate, dst []observer.Point) (*observer.Series, []observer.Point) {
	dst = dst[:0]
	var meta *observer.Series
	appendPoint := func(series *observer.Series, p observer.Point) {
		if meta == nil {
			copy := *series
			meta = &copy
		}
		dst = append(dst, p)
	}
	if reader, ok := storage.(tailPointReader); ok {
		reader.ForEachLastPoints(ref, end, maxPoints, agg, appendPoint)
		return meta, dst
	}
	storage.ForEachPoint(ref, 0, end, agg, func(series *observer.Series, p observer.Point) {
		if len(dst) == maxPoints {
			copy(dst, dst[1:])
			dst[len(dst)-1] = p
			return
		}
		appendPoint(series, p)
	})
	return meta, dst
}

func supportsSeriesAggregate(storage observer.StorageReader, ref observer.SeriesRef, agg observer.Aggregate) bool {
	if support, ok := storage.(seriesAggregateSupport); ok {
		return support.SupportsAggregate(ref, agg)
	}
	return true
}

// workloadSeriesRefs returns the workload series refs used by detector hot
// paths. It avoids the metadata-heavy ListSeries allocation when the storage
// implementation provides a ref-only listing.
func workloadSeriesRefs(storage observer.StorageReader, dst []observer.SeriesRef) []observer.SeriesRef {
	return listSeriesRefs(storage, observer.WorkloadSeriesFilter(), dst)
}

func listSeriesRefs(storage observer.StorageReader, filter observer.SeriesFilter, dst []observer.SeriesRef) []observer.SeriesRef {
	if lister, ok := storage.(seriesRefLister); ok {
		return lister.ListSeriesRefsInto(filter, dst)
	}

	metas := storage.ListSeries(filter)
	if cap(dst) < len(metas) {
		dst = make([]observer.SeriesRef, 0, len(metas))
	} else {
		dst = dst[:0]
	}
	for _, meta := range metas {
		dst = append(dst, meta.Ref)
	}
	return dst
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
// and false when using raw MAD as a denominator for relative change scores
// (e.g. ScanMW/ScanWelch preMAD checks).
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

// medianTimestampInterval computes the median gap between consecutive
// timestamps. It is the timestamp-array equivalent of medianPointInterval for
// detectors that keep a compact timestamp ring instead of retaining Points.
func medianTimestampInterval(timestamps []int64) int64 {
	if len(timestamps) < 2 {
		return 0
	}
	intervals := make([]int64, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		intervals[i-1] = timestamps[i] - timestamps[i-1]
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
