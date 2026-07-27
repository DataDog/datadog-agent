// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const testLogCountNamespace = "test_log_extractor"

func addTestLogCountPoint(t *testing.T, storage *timeSeriesStorage, timestamp int64, value float64) observerdef.SeriesRef {
	t.Helper()
	result := storage.Add(testLogCountNamespace, "log.test.pattern.count", value, timestamp, []string{"service:test"})
	require.NotEqual(t, observerdef.SeriesRef(-1), result.Ref)
	storage.SetContext(result.Ref, &observerdef.MetricContext{
		Pattern: "test pattern",
		Source:  testLogCountNamespace,
	})
	return result.Ref
}

func newTestLogCountView(storage *timeSeriesStorage, bucketSeconds, ttlSeconds int64) *timeAwareLogCountStorage {
	return newTimeAwareLogCountStorage(
		storage,
		storage,
		TestbenchLogCountViewConfig{
			BucketSeconds:  bucketSeconds,
			IdleTTLSeconds: ttlSeconds,
		},
		[]string{testLogCountNamespace},
	)
}

func TestTimeAwareLogCountViewFillsOnlyCausalTimestampGaps(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 1)
	addTestLogCountPoint(t, storage, 103, 1)
	view := newTestLogCountView(storage, 1, 3)

	assert.Nil(t, view.GetSeriesRange(ref, 0, 99, observerdef.AggregateAverage))

	series := view.GetSeriesRange(ref, 0, 103, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 0},
		{Timestamp: 102, Value: 0},
		{Timestamp: 103, Value: 1},
	}, series.Points)

	// A future preloaded occurrence must not causally extend the active window.
	series = view.GetSeriesRange(ref, 0, 106, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, int64(106), series.Points[len(series.Points)-1].Timestamp)
}

func TestTimeAwareLogCountViewUsesCompletedFixedWindows(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 1)
	addTestLogCountPoint(t, storage, 103, 2)
	view := newTestLogCountView(storage, 5, 10)

	beforeComplete := view.GetSeriesRange(ref, 0, 103, observerdef.AggregateAverage)
	require.NotNil(t, beforeComplete)
	assert.Empty(t, beforeComplete.Points)

	complete := view.GetSeriesRange(ref, 0, 104, observerdef.AggregateAverage)
	require.NotNil(t, complete)
	assert.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 3}}, complete.Points)
}

func TestTimeAwareLogCountViewStopsAtIdleTTLAndReactivates(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 1)
	addTestLogCountPoint(t, storage, 110, 1)
	view := newTestLogCountView(storage, 1, 2)

	series := view.GetSeriesRange(ref, 0, 110, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 0},
		{Timestamp: 102, Value: 0},
		{Timestamp: 110, Value: 1},
	}, series.Points)
}

func TestTimeAwareLogCountViewLeavesOrdinaryMetricsSparse(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := storage.Add("check", "cpu.user", 10, 100, nil).Ref
	storage.Add("check", "cpu.user", 20, 110, nil)
	view := newTestLogCountView(storage, 1, 300)

	series := view.GetSeriesRange(ref, 0, 110, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 10},
		{Timestamp: 110, Value: 20},
	}, series.Points)
}

func TestTimeAwareLogCountViewExposesAverageOnly(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 2)
	view := newTestLogCountView(storage, 1, 2)

	assert.Nil(t, view.GetSeriesRange(ref, 0, 100, observerdef.AggregateCount))
	assert.Zero(t, view.SumRange(ref, 0, 100, observerdef.AggregateCount))

	series := view.GetSeriesRange(ref, 0, 100, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{{Timestamp: 100, Value: 2}}, series.Points)
}

func TestTimeAwareLogCountViewPreventsAggregateCountDetectorState(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 1)
	view := newTestLogCountView(storage, 1, 2)
	detector := NewBOCPDDetector(DefaultBOCPDConfig())

	detector.Detect(view, 100)

	assert.Contains(t, detector.series, bocpdStateKey{ref: ref, agg: observerdef.AggregateAverage})
	assert.NotContains(t, detector.series, bocpdStateKey{ref: ref, agg: observerdef.AggregateCount})
	assert.Len(t, detector.series, 1)
}

type countViewRecordingDetector struct {
	calls  int
	points [][]observerdef.Point
}

func (d *countViewRecordingDetector) Name() string { return "count_view_recording" }

func (d *countViewRecordingDetector) Detect(series observerdef.Series) observerdef.DetectionResult {
	d.calls++
	points := append([]observerdef.Point(nil), series.Points...)
	d.points = append(d.points, points)
	return observerdef.DetectionResult{}
}

type countViewStorageDetector struct {
	series map[string][]observerdef.Point
}

func (d *countViewStorageDetector) Name() string { return "count_view_storage" }

func (d *countViewStorageDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	d.series = make(map[string][]observerdef.Point)
	for _, meta := range storage.ListSeries(observerdef.WorkloadSeriesFilter()) {
		series := storage.GetSeriesRange(meta.Ref, 0, dataTime, observerdef.AggregateAverage)
		if series != nil {
			d.series[meta.Namespace+"/"+meta.Name] = append([]observerdef.Point(nil), series.Points...)
		}
	}
	return observerdef.DetectionResult{}
}

type countViewTestExtractor struct{}

func (*countViewTestExtractor) Name() string { return testLogCountNamespace }
func (*countViewTestExtractor) ProcessLog(observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{}
}

func TestTimeAwareLogCountViewAdvancesSeriesDetectorGateWithoutRawWrites(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	addTestLogCountPoint(t, storage, 100, 1)
	// This point is preloaded but must remain invisible until its data time.
	addTestLogCountPoint(t, storage, 200, 1)
	view := newTestLogCountView(storage, 1, 2)
	detector := &countViewRecordingDetector{}
	adapter := newSeriesDetectorAdapter(detector, []observerdef.Aggregate{observerdef.AggregateAverage})

	adapter.Detect(view, 99)
	assert.Zero(t, detector.calls)

	adapter.Detect(view, 100)
	require.Equal(t, 1, detector.calls)
	assert.Equal(t, []observerdef.Point{{Timestamp: 100, Value: 1}}, detector.points[0])

	// No raw write occurred at 101, but the completed logical zero bucket
	// advances PointCountUpTo and causes the detector to run.
	adapter.Detect(view, 101)
	require.Equal(t, 2, detector.calls)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 0},
	}, detector.points[1])

	adapter.Detect(view, 101)
	assert.Equal(t, 2, detector.calls, "the same logical data time must not be replayed twice")

	adapter.Detect(view, 102)
	require.Equal(t, 3, detector.calls)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 0},
		{Timestamp: 102, Value: 0},
	}, detector.points[2])

	adapter.Detect(view, 103)
	assert.Equal(t, 3, detector.calls, "the gate must stop after the idle TTL")
}

func TestTimeAwareLogCountViewStatsSeparateRawAndLogicalPoints(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	ref := addTestLogCountPoint(t, storage, 100, 1)
	addTestLogCountPoint(t, storage, 102, 1)
	view := newTestLogCountView(storage, 1, 2)

	view.beginDetect(102)
	series := view.GetSeriesRange(ref, 0, 102, observerdef.AggregateAverage)
	require.NotNil(t, series)
	stats := view.snapshotStats()

	assert.Equal(t, 2, stats.RawStoredPoints)
	assert.Equal(t, 1, stats.RawStoredSeries)
	assert.Equal(t, int64(3), stats.LogicalDetectorObservations)
	assert.Equal(t, int64(1), stats.LogicalZeroObservations)
	assert.Equal(t, 1, stats.PeakActiveSeries)
	assert.Zero(t, stats.RetainedViewSeriesState)
}

func TestEngineUsesTimeAwareViewOnlyForLogOccurrenceSeries(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	logRef := addTestLogCountPoint(t, storage, 100, 1)
	ordinaryRef := storage.Add("check", "cpu.user", 10, 100, nil).Ref
	storage.Add("check", "cpu.user", 20, 103, nil)

	detector := &countViewStorageDetector{}
	engine := newEngine(engineConfig{
		storage:    storage,
		detectors:  []observerdef.Detector{detector},
		extractors: []observerdef.LogMetricsExtractor{&countViewTestExtractor{}},
	})
	engine.configureTestbenchLogCountView(TestbenchLogCountViewConfig{
		BucketSeconds:  1,
		IdleTTLSeconds: 2,
	})
	engine.Advance(102)

	assert.Equal(t, []observerdef.Point{
		{Timestamp: 100, Value: 1},
		{Timestamp: 101, Value: 0},
		{Timestamp: 102, Value: 0},
	}, detector.series[testLogCountNamespace+"/log.test.pattern.count"])
	assert.Equal(t, []observerdef.Point{{Timestamp: 100, Value: 10}}, detector.series["check/cpu.user"])
	assert.Equal(t, 1, storage.PointCount(logRef), "virtual zeros must not be stored")
	assert.Equal(t, 2, storage.PointCount(ordinaryRef), "ordinary storage must remain untouched")
}
