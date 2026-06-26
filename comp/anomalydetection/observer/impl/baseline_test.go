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

// alwaysFiringDetector produces one anomaly per Detect call at dataTime,
// pointing to a known series ref.
type alwaysFiringDetector struct {
	namespace string
	name      string
	ref       observerdef.SeriesRef
}

func (d *alwaysFiringDetector) Name() string { return "always_firing" }
func (d *alwaysFiringDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: []observerdef.Anomaly{{
			Source:       observerdef.SeriesDescriptor{Namespace: d.namespace, Name: d.name, Aggregate: AggregateAverage},
			DetectorName: "always_firing",
			Timestamp:    dataTime,
			Title:        "anomaly",
			SourceRef:    &observerdef.QueryHandle{Ref: d.ref, Aggregate: AggregateAverage},
		}},
	}
}

// recordingCorrelator records all anomalies forwarded to it via ProcessAnomaly.
type recordingCorrelator struct {
	received []observerdef.Anomaly
}

func (c *recordingCorrelator) Name() string { return "recording" }
func (c *recordingCorrelator) ProcessAnomaly(a observerdef.Anomaly) {
	c.received = append(c.received, a)
}
func (c *recordingCorrelator) Advance(_ int64)                                     {}
func (c *recordingCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation { return nil }
func (c *recordingCorrelator) PendingEvents() []observerdef.CorrelatorEvent        { return nil }
func (c *recordingCorrelator) Reset()                                              { c.received = nil }

// makeBaselineEngine creates a minimal engine with one series in storage and a
// detector that fires anomalies pointing to that series on every Detect call.
func makeBaselineEngine(cfg BaselineConfig, correlator observerdef.Correlator) (*engine, observerdef.SeriesRef) {
	storage := newTimeSeriesStorage()
	res := storage.Add("ns", "cpu", 1.0, 100, nil)
	detector := &alwaysFiringDetector{namespace: "ns", name: "cpu", ref: res.Ref}
	var corrs []observerdef.Correlator
	if correlator != nil {
		corrs = []observerdef.Correlator{correlator}
	}
	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observerdef.Detector{detector},
		correlators: corrs,
		baseline:    cfg,
	})
	return e, res.Ref
}

// ---- baselineController unit tests ----

func TestBaselineController_WindowSeededOnFirstActiveAt(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})
	assert.Equal(t, int64(0), b.startSec)

	assert.True(t, b.activeAt(1000))
	assert.Equal(t, int64(1000), b.startSec)

	assert.True(t, b.activeAt(1599))
	assert.False(t, b.activeAt(1600))
}

func TestBaselineController_MarkAccumulatesAndDeduplicates(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})
	b.activeAt(0)

	b.mark(1)
	b.mark(2)
	b.mark(1) // duplicate hash

	assert.Equal(t, 3, b.windowAnomalyCount) // count includes re-fires
	assert.Len(t, b.mutedHashes, 2)          // set deduplicates
}

func TestBaselineController_MarkNoOpWhenFrozen(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})
	b.activeAt(0)

	b.frozen = true
	b.mark(1)
	assert.Empty(t, b.mutedHashes)
}

func TestBaselineController_ShouldFreeze(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})

	assert.False(t, b.shouldFreeze(1000)) // no startSec yet

	b.activeAt(1000) // seed startSec=1000
	assert.False(t, b.shouldFreeze(1599))
	assert.True(t, b.shouldFreeze(1600))
}

func TestBaselineController_FreezeReturnsCount(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})
	b.activeAt(1000)
	b.mark(100)
	b.mark(200)

	count := b.freeze()

	assert.Equal(t, 2, count)
	assert.Len(t, b.mutedHashes, 2)
	assert.True(t, b.frozen)
}

func TestBaselineController_Reset(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600})
	b.activeAt(1000)
	b.mark(100)
	b.freeze()
	require.True(t, b.frozen)

	b.reset()

	assert.Equal(t, int64(0), b.startSec)
	assert.Empty(t, b.mutedHashes)
	assert.Equal(t, 0, b.windowAnomalyCount)
	assert.False(t, b.frozen)
}

// ---- engine integration tests ----

func TestBaseline_AnomaliesHeldDuringWindow(t *testing.T) {
	correlator := &recordingCorrelator{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600}, correlator)

	e.Advance(100) // seeds window, anomaly at t=100 held back
	e.Advance(400) // still in window, anomaly at t=400 held back

	assert.Empty(t, correlator.received)
}

func TestBaseline_AnomaliesForwardedAfterWindow(t *testing.T) {
	correlator := &recordingCorrelator{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600}, correlator)

	e.Advance(100) // seeds window
	e.Advance(800) // past window end: anomaly forwarded, freeze fires

	assert.NotEmpty(t, correlator.received)
}

func TestBaseline_ExactFreezeTimeBoundary(t *testing.T) {
	// Window: [100, 700). activeAt uses strict <, so t=699 is the last in-window
	// second and t=700 is the first out-of-window second (exact freeze point).
	const start, dur = int64(100), int64(600)
	correlator := &recordingCorrelator{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: dur, MuteNoisyMetrics: true}, correlator)

	e.Advance(start)           // seeds window, anomaly held back and marked
	e.Advance(start + dur - 1) // t=699: still in window, anomaly held back
	assert.Empty(t, correlator.received)
	assert.False(t, e.baseline.frozen)

	e.Advance(start + dur) // t=700: exact freeze point — freeze fires, muted anomaly blocked
	assert.Empty(t, correlator.received)
	assert.True(t, e.baseline.frozen)
}

func TestBaseline_FreezeAdvanceAnomalyNotForwardedToCorrelator(t *testing.T) {
	// Regression test: on the advance that closes the baseline window, activeAt()
	// returns false so anomalies bypass the in-window gate. Without the second
	// gate that checks mutedHashes, noisy-series anomalies from this advance
	// reach processAnomaly and land in the correlator's sliding window, causing
	// false-positive reports immediately after freeze.
	correlator := &recordingCorrelator{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: true}, correlator)

	e.Advance(100) // seeds window, marks "ns/cpu" as noisy
	e.Advance(700) // freeze advance: activeAt(700)=false, anomaly must NOT reach correlator

	assert.Empty(t, correlator.received)
}

func TestBaseline_FreezeEmitsEvent(t *testing.T) {
	sink := &collectingSink{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: false}, nil)
	e.Subscribe(sink)

	e.Advance(100) // seeds window, marks series as noisy
	e.Advance(700) // triggers freeze

	evts := sink.eventsOfKind(eventBaselineCompleted)
	require.Len(t, evts, 1)
	assert.NotEmpty(t, evts[0].baselineCompleted.mutedHashes)
}

func TestBaseline_MutedHashesReachFilter(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: true}, nil)
	e.Subscribe(&baselineEventSink{filter: filter})

	e.Advance(100) // seeds window, marks "ns/cpu" as noisy
	e.Advance(700) // freeze: mute hashes propagated to filter

	assert.False(t, filter.isAllowed("cpu", "ns", nil))
	assert.True(t, filter.isAllowed("mem", "ns", nil)) // unrelated metric unaffected
}

func TestBaseline_DisabledByConfig(t *testing.T) {
	correlator := &recordingCorrelator{}
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: false}, correlator)

	e.Advance(100)
	e.Advance(400)

	// No baseline window — anomalies forwarded immediately
	assert.NotEmpty(t, correlator.received)
	assert.Nil(t, e.baseline)
}

func TestBaseline_MuteNoisyMetricsFalseDoesNotDropMetrics(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	// With MuteNoisyMetrics=false, baselineEventSink is NOT subscribed (matches
	// production wiring in NewComponent). The filter must remain untouched.
	e, _ := makeBaselineEngine(BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: false}, nil)

	e.Advance(100)
	e.Advance(700) // freeze

	assert.True(t, filter.isAllowed("cpu", "ns", nil))
	assert.False(t, e.baseline.frozen && e.baseline.config.MuteNoisyMetrics)
}

// storageAwareDetector fires one anomaly per series found in storage.
// Used when the series ref is only known after the extractor creates it.
type storageAwareDetector struct{}

func (d *storageAwareDetector) Name() string { return "storage_aware" }
func (d *storageAwareDetector) Detect(sr observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	metas := sr.ListSeries(observerdef.SeriesFilter{})
	anomalies := make([]observerdef.Anomaly, 0, len(metas))
	for _, meta := range metas {
		anomalies = append(anomalies, observerdef.Anomaly{
			Source:       observerdef.SeriesDescriptor{Namespace: meta.Namespace, Name: meta.Name, Tags: meta.Tags, Aggregate: AggregateAverage},
			DetectorName: "storage_aware",
			Timestamp:    dataTime,
			Title:        "anomaly",
			SourceRef:    &observerdef.QueryHandle{Ref: meta.Ref, Aggregate: AggregateAverage},
		})
	}
	return observerdef.DetectionResult{Anomalies: anomalies}
}

// fixedTagExtractor emits a virtual metric with a fixed name and no extra tags.
type fixedTagExtractor struct{ namespace, metricName string }

func (x *fixedTagExtractor) Name() string { return x.namespace }
func (x *fixedTagExtractor) ProcessLog(_ observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{Name: x.metricName, Value: 1}},
	}
}

func TestBaseline_VirtualMetricDroppedAfterFreeze(t *testing.T) {
	storage := newTimeSeriesStorage()
	extractor := &fixedTagExtractor{namespace: "virt", metricName: "rate"}

	e := newEngine(engineConfig{
		storage:    storage,
		detectors:  []observerdef.Detector{&storageAwareDetector{}},
		extractors: []observerdef.LogMetricsExtractor{extractor},
		baseline:   BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: true},
	})

	// First IngestLog creates the series; Advance marks it as noisy.
	e.IngestLog("src", &logObs{timestampMs: 100_000})
	e.Advance(100)

	// During the window: subsequent ingests must still reach storage.
	countBefore := storage.TotalSeriesCount("")
	e.IngestLog("src", &logObs{timestampMs: 200_000})
	assert.Equal(t, countBefore, storage.TotalSeriesCount(""))

	e.Advance(700) // freeze: series removed from storage
	assert.Equal(t, 0, storage.TotalSeriesCount(""))

	// After freeze: virtual metric is dropped at ingest and not re-created.
	e.IngestLog("src", &logObs{timestampMs: 800_000})
	assert.Equal(t, 0, storage.TotalSeriesCount(""))
}
