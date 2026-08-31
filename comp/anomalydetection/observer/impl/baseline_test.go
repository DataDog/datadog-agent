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

// baselineTestDetector can model independently timed detector baselines and
// records series reclamation from another detector's baseline completion.
type baselineTestDetector struct {
	name          string
	readyAtSec    int64
	ready         bool
	source        observerdef.SeriesDescriptor
	ref           observerdef.SeriesRef
	emitAfterSec  int64
	includeSource bool
	removed       []observerdef.SeriesRef
}

func (d *baselineTestDetector) Name() string { return d.name }
func (d *baselineTestDetector) Ready() bool  { return d.ready }
func (d *baselineTestDetector) Detect(_ observerdef.StorageReader, dataSec int64) observerdef.DetectionResult {
	if dataSec >= d.readyAtSec {
		d.ready = true
	}
	if dataSec < d.emitAfterSec {
		return observerdef.DetectionResult{}
	}
	anomaly := observerdef.Anomaly{
		Source:       d.source,
		DetectorName: d.name,
		Timestamp:    dataSec,
		Title:        "anomaly",
	}
	if d.includeSource {
		anomaly.SourceRef = &observerdef.QueryHandle{Ref: d.ref, Aggregate: AggregateAverage}
	}
	return observerdef.DetectionResult{Anomalies: []observerdef.Anomaly{anomaly}}
}
func (d *baselineTestDetector) RemoveSeries(refs []observerdef.SeriesRef) {
	d.removed = append(d.removed, refs...)
}

func (d *alwaysFiringDetector) Name() string { return "always_firing" }
func (*alwaysFiringDetector) Ready() bool    { return true }
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

func TestBaselineController_DetectorReadinessStartsIndependentWindows(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600}, []string{"fast", "slow"})
	assert.False(t, b.debugStatus().Started)
	b.start(1000)
	status := b.debugStatus()
	assert.True(t, status.Started)
	require.Len(t, status.Detectors, 2)
	assert.Equal(t, BaselineDetectorDebugStatus{Name: "fast"}, status.Detectors[0])
	assert.Equal(t, BaselineDetectorDebugStatus{Name: "slow"}, status.Detectors[1])
	assert.True(t, b.isAnalyzingAt("fast", 1000))
	assert.True(t, b.isAnalyzingAt("slow", 1299))
	assert.True(t, b.ready("fast", 1000))
	assert.False(t, b.ready("fast", 1001))
	assert.True(t, b.ready("slow", 1300))
	assert.True(t, b.isAnalyzingAt("slow", 1899))
	assert.False(t, b.isAnalyzingAt("slow", 1900))
	assert.False(t, b.isAnalyzingAt("unknown", 1000))
	assert.Equal(t, []string{"fast"}, b.due(1600))

	b.mark("fast", 1)
	b.mark("fast", 1)
	newHashes, changed, count, allComplete := b.complete("fast")
	assert.True(t, changed)
	assert.Equal(t, 2, count)
	assert.Len(t, newHashes, 1)
	assert.False(t, allComplete)
	status = b.debugStatus()
	assert.True(t, status.Detectors[0].Completed)
	assert.False(t, status.Detectors[1].Completed)
	assert.Equal(t, 1, status.Detectors[0].MutedCount)
	assert.Equal(t, []string{"slow"}, b.due(1900))
	_, _, _, allComplete = b.complete("slow")
	assert.True(t, allComplete)
	assert.False(t, b.isAnalyzingAt("slow", 1900))
	assert.True(t, b.debugStatus().AllComplete)
}

func TestBaselineController_WaitingDetectorSuppressesWithoutMuting(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 60}, []string{"waiting"})
	b.start(100)

	assert.True(t, b.isAnalyzingAt("waiting", 100))
	b.mark("waiting", 1) // defensive suppression before Ready must not mute
	assert.Empty(t, b.detectors["waiting"].pendingHashes)
	assert.Empty(t, b.due(1_000))
	assert.False(t, b.allComplete())

	b.ready("waiting", 130)
	b.mark("waiting", 1)
	assert.Equal(t, []string{"waiting"}, b.due(190))
}

func TestBaselineController_CompletionPublishesImmutableUnionAndReleasesPendingHashes(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600}, []string{"first", "second"})
	b.start(1000)
	b.ready("first", 1000)
	b.ready("second", 1000)
	b.mark("first", 1)
	b.mark("second", 1)
	b.mark("second", 2)

	firstDelta, changed, _, _ := b.complete("first")
	require.True(t, changed)
	assert.Equal(t, map[uint64]struct{}{1: {}}, firstDelta)
	firstSnapshot := b.mutedHashes
	assert.Nil(t, b.detectors["first"].pendingHashes)
	assert.Equal(t, 1, b.detectors["first"].mutedCount)

	secondDelta, changed, _, _ := b.complete("second")
	require.True(t, changed)
	assert.Equal(t, map[uint64]struct{}{2: {}}, secondDelta)
	assert.Equal(t, map[uint64]struct{}{1: {}}, firstSnapshot)
	assert.Equal(t, map[uint64]struct{}{1: {}, 2: {}}, b.mutedHashes)
	assert.Nil(t, b.detectors["second"].pendingHashes)
	assert.Equal(t, 2, b.detectors["second"].mutedCount)

	status := b.debugStatus()
	assert.Equal(t, 1, status.Detectors[0].MutedCount)
	assert.Equal(t, 2, status.Detectors[1].MutedCount)
}

func TestBaselineController_DuplicateCompletionDoesNotReplaceUnionSnapshot(t *testing.T) {
	b := newBaselineController(BaselineConfig{DurationSec: 600}, []string{"first", "second"})
	b.start(1000)
	b.ready("first", 1000)
	b.ready("second", 1000)
	b.mark("first", 1)
	b.mark("second", 1)

	_, changed, _, _ := b.complete("first")
	require.True(t, changed)
	newHashes, changed, _, _ := b.complete("second")
	assert.Empty(t, newHashes)
	assert.False(t, changed)
	assert.Equal(t, map[uint64]struct{}{1: {}}, b.mutedHashes)
}

func TestBaselineController_VerboseNamesAreLazyAndReleased(t *testing.T) {
	b := newBaselineController(BaselineConfig{}, nil)
	assert.Nil(t, b.mutedNames)

	b.recordMutedNames([]string{"ns/cpu", "ns/memory", "ns/cpu"})
	assert.Equal(t, map[string]struct{}{"ns/cpu": {}, "ns/memory": {}}, b.mutedNames)
	assert.Equal(t, []string{"ns/cpu", "ns/memory"}, b.takeMutedDisplayNames())
	assert.Nil(t, b.mutedNames)
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

func TestBaseline_WaitingDetectorDoesNotMuteUntilReady(t *testing.T) {
	storage := newTimeSeriesStorage()
	ref := storage.Add("ns", "cpu", 1.0, 100, nil).Ref
	detector := &baselineTestDetector{
		name:          "waiting",
		readyAtSec:    200,
		emitAfterSec:  100,
		includeSource: true,
		ref:           ref,
		source:        observerdef.SeriesDescriptor{Namespace: "ns", Name: "cpu", Aggregate: AggregateAverage},
	}
	e := newEngine(engineConfig{storage: storage, detectors: []observerdef.Detector{detector}, baseline: BaselineConfig{Enabled: true, DurationSec: 100, MuteNoisyMetrics: true}})

	e.Advance(100) // detector emits before Ready; it is suppressed but cannot mute
	assert.Empty(t, e.baseline.mutedHashes)
	assert.False(t, e.baseline.detectors["waiting"].ready)

	e.Advance(200) // readiness transition anomaly is included in qualification
	assert.True(t, e.baseline.detectors["waiting"].ready)
	assert.Len(t, e.baseline.detectors["waiting"].pendingHashes, 1)
	e.Advance(300)
	assert.Len(t, e.baseline.mutedHashes, 1)
}

func TestBaseline_FastCompletionRemovesSeriesFromSlowerDetector(t *testing.T) {
	storage := newTimeSeriesStorage()
	ref := storage.Add("ns", "cpu", 1.0, 100, nil).Ref
	source := observerdef.SeriesDescriptor{Namespace: "ns", Name: "cpu", Aggregate: AggregateAverage}
	fast := &baselineTestDetector{name: "fast", source: source, ref: ref, includeSource: true}
	slow := &baselineTestDetector{
		name:          "slow",
		readyAtSec:    400,
		source:        source,
		ref:           ref,
		includeSource: true,
	}
	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{fast, slow},
		baseline:  BaselineConfig{Enabled: true, DurationSec: 100, MuteNoisyMetrics: true},
	})

	e.Advance(100) // both detectors are analysing and nominate cpu for muting
	e.Advance(200) // fast completes, immediately reclaiming cpu everywhere

	assert.Zero(t, storage.TotalSeriesCount())
	assert.Equal(t, []observerdef.SeriesRef{ref}, fast.removed)
	assert.Equal(t, []observerdef.SeriesRef{ref}, slow.removed)
	assert.True(t, e.baseline.detectors["fast"].completed)
	assert.False(t, e.baseline.detectors["slow"].completed)
	assert.False(t, e.baseline.allComplete())
}

func TestBaseline_FastDetectorForwardsWhileSlowerDetectorStillAnalyses(t *testing.T) {
	storage := newTimeSeriesStorage()
	ref := storage.Add("ns", "memory", 1.0, 100, nil).Ref
	fast := &baselineTestDetector{
		name:          "fast",
		source:        observerdef.SeriesDescriptor{Namespace: "ns", Name: "memory", Aggregate: AggregateAverage},
		ref:           ref,
		emitAfterSec:  200,
		includeSource: true,
	}
	slow := &baselineTestDetector{name: "slow", readyAtSec: 400, emitAfterSec: 1<<62 - 1}
	correlator := &recordingCorrelator{}
	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observerdef.Detector{fast, slow},
		correlators: []observerdef.Correlator{correlator},
		baseline:    BaselineConfig{Enabled: true, DurationSec: 100, MuteNoisyMetrics: true},
	})

	e.Advance(100) // starts both windows; neither detector emits
	e.Advance(200) // fast completes and its first anomaly is forwarded

	require.Len(t, correlator.received, 1)
	assert.Equal(t, "fast", correlator.received[0].DetectorName)
	assert.True(t, e.baseline.detectors["fast"].completed)
	assert.False(t, e.baseline.detectors["slow"].completed)
	assert.False(t, e.baseline.allComplete())
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
	assert.False(t, e.baseline.allComplete())

	e.Advance(start + dur) // synthetic anomaly has no storage reference to mute
	assert.Len(t, correlator.received, 1)
	assert.True(t, e.baseline.allComplete())
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

	assert.Len(t, correlator.received, 1) // synthetic anomaly has no storage reference to mute
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
	assert.False(t, e.baseline.config.MuteNoisyMetrics)
}

// storageAwareDetector fires one anomaly per series found in storage.
// Used when the series ref is only known after the extractor creates it.
type storageAwareDetector struct{}

func (d *storageAwareDetector) Name() string { return "storage_aware" }
func (*storageAwareDetector) Ready() bool    { return true }
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
	countBefore := storage.TotalSeriesCount()
	e.IngestLog("src", &logObs{timestampMs: 200_000})
	assert.Equal(t, countBefore, storage.TotalSeriesCount())

	e.Advance(700) // freeze: series removed from storage
	assert.Equal(t, 0, storage.TotalSeriesCount())

	// After freeze: virtual metric is dropped at ingest and not re-created.
	e.IngestLog("src", &logObs{timestampMs: 800_000})
	assert.Equal(t, 0, storage.TotalSeriesCount())
}
