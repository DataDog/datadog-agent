// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sync"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectingSink collects all events for test assertions.
type collectingSink struct {
	mu     sync.Mutex
	events []engineEvent
}

func (s *collectingSink) onEngineEvent(evt engineEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, evt)
}

func (s *collectingSink) getEvents() []engineEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]engineEvent, len(s.events))
	copy(result, s.events)
	return result
}

func (s *collectingSink) eventsOfKind(kind engineEventKind) []engineEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []engineEvent
	for _, evt := range s.events {
		if evt.kind == kind {
			result = append(result, evt)
		}
	}
	return result
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})

	sink := &collectingSink{}
	unsub := e.Subscribe(sink)

	// Emit an event manually to verify delivery.
	e.emit(engineEvent{kind: eventAdvanceCompleted, timestamp: 100})

	events := sink.getEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, eventAdvanceCompleted, events[0].kind)

	// Unsubscribe and verify no more delivery.
	unsub()
	e.emit(engineEvent{kind: eventAdvanceCompleted, timestamp: 200})

	events = sink.getEvents()
	assert.Len(t, events, 1, "should not receive events after unsubscribe")
}

func TestMultipleSinksReceiveEvents(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})

	sink1 := &collectingSink{}
	sink2 := &collectingSink{}
	e.Subscribe(sink1)
	e.Subscribe(sink2)

	e.emit(engineEvent{kind: eventAnomalyCreated, timestamp: 50})

	assert.Len(t, sink1.getEvents(), 1)
	assert.Len(t, sink2.getEvents(), 1)
}

func TestUnsubscribeOnlyAffectsTargetSink(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})

	sink1 := &collectingSink{}
	sink2 := &collectingSink{}
	unsub1 := e.Subscribe(sink1)
	e.Subscribe(sink2)

	// Unsubscribe sink1 only.
	unsub1()

	e.emit(engineEvent{kind: eventAdvanceCompleted, timestamp: 100})

	assert.Len(t, sink1.getEvents(), 0, "sink1 should not receive events after unsubscribe")
	assert.Len(t, sink2.getEvents(), 1, "sink2 should still receive events")
}

// anomalyDetector produces anomalies on Detect for testing event emission.
type anomalyDetector struct {
	name      string
	anomalies []observerdef.Anomaly
}

func (d *anomalyDetector) Name() string { return d.name }
func (d *anomalyDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: d.anomalies,
	}
}

type resettableDetector struct {
	name       string
	resetCount int
}

func (d *resettableDetector) Name() string { return d.name }
func (d *resettableDetector) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}
func (d *resettableDetector) Reset() { d.resetCount++ }

type resettableCorrelator struct {
	name       string
	resetCount int
}

func (c *resettableCorrelator) Name() string                         { return c.name }
func (c *resettableCorrelator) ProcessAnomaly(_ observerdef.Anomaly) {}
func (c *resettableCorrelator) Advance(_ int64)                      {}
func (c *resettableCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation {
	return nil
}
func (c *resettableCorrelator) Reset() { c.resetCount++ }

func TestAdvanceEmitsAdvanceCompletedEvent(t *testing.T) {
	e := newEngine(engineConfig{
		storage:   newTimeSeriesStorage(),
		detectors: []observerdef.Detector{&mockDetector{name: "noop"}},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.Advance(100)

	advances := sink.eventsOfKind(eventAdvanceCompleted)
	require.Len(t, advances, 1)
	assert.Equal(t, int64(100), advances[0].timestamp)
	assert.NotNil(t, advances[0].advanceCompleted)
	assert.Equal(t, int64(100), advances[0].advanceCompleted.advancedToSec)
	assert.Equal(t, 0, advances[0].advanceCompleted.anomalyCount)
}

func TestAdvanceEmitsAnomalyCreatedEvents(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{Source: "cpu", DetectorName: "test", Timestamp: 99, SourceSeriesID: "ns|cpu|"},
		{Source: "mem", DetectorName: "test", Timestamp: 99, SourceSeriesID: "ns|mem|"},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test", anomalies: anomalies},
		},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.Advance(100)

	anomalyEvents := sink.eventsOfKind(eventAnomalyCreated)
	assert.Len(t, anomalyEvents, 2)
	assert.Equal(t, "cpu", string(anomalyEvents[0].anomalyCreated.anomaly.Source))
	assert.Equal(t, "mem", string(anomalyEvents[1].anomalyCreated.anomaly.Source))
}

func TestAdvanceEmitsCorrelationUpdatedEvents(t *testing.T) {
	e := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		correlators: []observerdef.Correlator{&mockCorrelator{name: "time_cluster"}},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.Advance(100)

	corrEvents := sink.eventsOfKind(eventCorrelationUpdated)
	require.Len(t, corrEvents, 1)
	assert.Equal(t, "time_cluster", corrEvents[0].correlationUpdated.correlatorName)
}

func TestAdvanceWithReasonPreservesReason(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.advanceWithReason(100, advanceReasonInputDriven)

	advances := sink.eventsOfKind(eventAdvanceCompleted)
	require.Len(t, advances, 1)
	assert.Equal(t, advanceReasonInputDriven, advances[0].advanceCompleted.reason)
}

func TestNoEventOnSkippedAdvance(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	e.Advance(100) // advance first without sink

	sink := &collectingSink{}
	e.Subscribe(sink)

	// Advancing to same or earlier time should be a no-op.
	e.Advance(100)
	e.Advance(50)

	assert.Len(t, sink.getEvents(), 0, "no events should be emitted for skipped advances")
}

func TestReplayStoredDataEmitsEventsViaScheduler(t *testing.T) {
	storage := newTimeSeriesStorage()
	// Add data at timestamps 10, 11, 12, 13.
	for sec := int64(10); sec <= 13; sec++ {
		storage.Add("ns", "cpu", 1.0, sec, nil)
	}

	e := newEngine(engineConfig{storage: storage})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.ReplayStoredData()

	advances := sink.eventsOfKind(eventAdvanceCompleted)
	// currentBehaviorPolicy: observation at ts=10 -> advance to 9,
	// ts=11 -> advance to 10, ts=12 -> advance to 11, ts=13 -> advance to 12
	// onReplayEnd -> advance to 13 (latestDataTime)
	require.Len(t, advances, 5)
	assert.Equal(t, int64(9), advances[0].advanceCompleted.advancedToSec)
	assert.Equal(t, advanceReasonInputDriven, advances[0].advanceCompleted.reason)
	assert.Equal(t, int64(10), advances[1].advanceCompleted.advancedToSec)
	assert.Equal(t, int64(11), advances[2].advanceCompleted.advancedToSec)
	assert.Equal(t, int64(12), advances[3].advanceCompleted.advancedToSec)
	assert.Equal(t, int64(13), advances[4].advanceCompleted.advancedToSec)
	assert.Equal(t, advanceReasonReplayEnd, advances[4].advanceCompleted.reason)
}

func TestEngineResetResetsDetectorsAndCorrelators(t *testing.T) {
	detector := &resettableDetector{name: "detector"}
	correlator := &resettableCorrelator{name: "correlator"}
	e := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		detectors:   []observerdef.Detector{detector},
		correlators: []observerdef.Correlator{correlator},
	})

	e.Reset()

	assert.Equal(t, 1, detector.resetCount)
	assert.Equal(t, 1, correlator.resetCount)
}

func TestReporterEventSink(t *testing.T) {
	reported := 0
	reporter := &countingReporter{count: &reported}

	sink := &reporterEventSink{
		reporters: []observerdef.Reporter{reporter},
	}

	// advanceCompleted should trigger Report.
	sink.onEngineEvent(engineEvent{
		kind:             eventAdvanceCompleted,
		timestamp:        100,
		advanceCompleted: &advanceCompletedEvent{advancedToSec: 100},
	})
	assert.Equal(t, 1, reported)

	// anomalyCreated should NOT trigger Report.
	sink.onEngineEvent(engineEvent{
		kind:      eventAnomalyCreated,
		timestamp: 100,
		anomalyCreated: &anomalyCreatedEvent{
			anomaly: observerdef.Anomaly{Source: "cpu"},
		},
	})
	assert.Equal(t, 1, reported, "anomaly events should not trigger reporter")
}

// countingReporter counts how many times Report is called.
type countingReporter struct {
	count *int
}

func (r *countingReporter) Name() string                      { return "counting" }
func (r *countingReporter) Report(_ observerdef.ReportOutput) { *r.count++ }

func TestFindingM1_DedupKeyTooCoarse(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike detected",
			Description:    "CPU spike",
			Timestamp:      100,
		},
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Trend change detected",
			Description:    "CPU trend shift",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test_detector", anomalies: anomalies},
		},
	})

	e.Advance(100)

	sv := e.StateView()
	raw := sv.Anomalies()
	assert.Len(t, raw, 2,
		"two anomalies with same seriesID+detector+timestamp but different titles should both survive dedup")
}

func TestFindingM2_EmptySourceSeriesIDCollision(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{
			Type:           observerdef.AnomalyTypeLog,
			Source:         "logs",
			SourceSeriesID: "", // empty for log anomalies
			DetectorName:   "log_detector",
			Title:          "Error pattern A detected",
			Description:    "Pattern A",
			Timestamp:      100,
		},
		{
			Type:           observerdef.AnomalyTypeLog,
			Source:         "logs",
			SourceSeriesID: "", // empty for log anomalies
			DetectorName:   "log_detector",
			Title:          "Error pattern B detected",
			Description:    "Pattern B",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "log_detector", anomalies: anomalies},
		},
	})

	e.Advance(100)

	sv := e.StateView()
	raw := sv.Anomalies()
	assert.Len(t, raw, 2,
		"two log anomalies with empty SourceSeriesID but different content should both survive dedup")
}

func TestFindingM3_DedupAsymmetry(t *testing.T) {
	// Two identical anomalies (same dedup key) -- one will be deduped from rawAnomalies.
	anomalies := []observerdef.Anomaly{
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike",
			Timestamp:      100,
		},
		{
			Source:         "cpu",
			SourceSeriesID: "ns|cpu:avg|",
			DetectorName:   "test_detector",
			Title:          "Spike",
			Timestamp:      100,
		},
	}

	e := newEngine(engineConfig{
		storage: newTimeSeriesStorage(),
		detectors: []observerdef.Detector{
			&anomalyDetector{name: "test_detector", anomalies: anomalies},
		},
	})

	sink := &collectingSink{}
	e.Subscribe(sink)

	e.Advance(100)

	sv := e.StateView()
	rawCount := len(sv.Anomalies())
	eventCount := len(sink.eventsOfKind(eventAnomalyCreated))

	// The bug: events will have 2 (no dedup) but rawAnomalies will have 1 (deduped).
	// If the system were consistent, these should match.
	assert.Equal(t, rawCount, eventCount,
		"anomalyCreated event count (%d) should match rawAnomalies count (%d); "+
			"mismatch means events/reporters see duplicates that rawAnomalies filtered out",
		eventCount, rawCount)
}

func TestFindingM4_UnboundedGrowthOfUniqueAnomalySources(t *testing.T) {
	// Run the engine with many unique anomaly source names. The
	// uniqueAnomalySources map should be bounded, but the finding says it grows
	// without eviction.

	storage := newTimeSeriesStorage()

	// We need a detector that emits anomalies with unique source names.
	// Use a custom detector that generates a unique source on each Detect call.
	det := &dynamicAnomalyDetector{prefix: "metric_"}

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{det},
	})

	// Generate 1000 unique anomaly sources across many advance cycles.
	for i := 0; i < 1000; i++ {
		det.currentIndex = i
		e.Advance(int64(i + 1))
	}

	sourceCount := e.UniqueAnomalySourceCount()
	t.Logf("uniqueAnomalySources size after 1000 unique anomalies: %d", sourceCount)

	// The bug: all 1000 unique sources are retained forever.
	// A bounded implementation would cap or evict old entries.
	// Assert that the map is bounded (e.g., under 500).
	// This WILL FAIL because the map grows unbounded.
	assert.LessOrEqual(t, sourceCount, 500,
		"uniqueAnomalySources has %d entries after 1000 anomalies; "+
			"expected bounded growth but map grows without eviction", sourceCount)
}

func TestFindingM4_UnboundedGrowthOfAccumulatedCorrelations(t *testing.T) {
	storage := newTimeSeriesStorage()

	// A correlator that produces unique patterns on each Advance.
	corr := &dynamicCorrelator{prefix: "pattern_"}

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{corr},
	})

	for i := 0; i < 1000; i++ {
		corr.currentIndex = i
		e.Advance(int64(i + 1))
	}

	corrCount := len(e.AccumulatedCorrelations())
	t.Logf("accumulatedCorrelations size after 1000 unique patterns: %d", corrCount)

	assert.LessOrEqual(t, corrCount, 500,
		"accumulatedCorrelations has %d entries after 1000 unique patterns; "+
			"expected bounded growth but map grows without eviction", corrCount)
}

func TestFindingM9_SetDetectorsRace(t *testing.T) {
	// SetDetectors replaces engine slices without a lock.
	// Running concurrently with Advance should trigger the race detector.

	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{&mockDetector{name: "initial"}},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: Advance in a loop
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	// Goroutine B: SetDetectors in a loop
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetDetectors([]observerdef.Detector{
				&mockDetector{name: fmt.Sprintf("det_%d", i)},
			})
		}
	}()

	wg.Wait()
}

func TestFindingM9_SetCorrelatorsRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "initial"}},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	wg.Wait()
}

func TestFindingM10_ResetRace(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	det := &resettableDetector{name: "det"}
	corr := &resettableCorrelator{name: "corr"}

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observerdef.Detector{det},
		correlators: []observerdef.Correlator{corr},
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Advance(int64(i + 2))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Reset()
		}
	}()

	wg.Wait()
}

func TestFindingM12_LogOnlyTimestampsSkippedInReplay(t *testing.T) {
	// DataTimestamps() only returns metric timestamps. A log at timestamp 103
	// that produces no virtual metrics won't appear, so replay skips it.
	//
	// In live-style ingestion, every IngestLog call triggers onObservation,
	// generating advance requests for that timestamp. In replay, only
	// DataTimestamps() are iterated.

	storage := newTimeSeriesStorage()

	extractor := &noopLogExtractor{}

	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	// --- Live-style ingestion ---
	liveSink := &collectingSink{}
	e.Subscribe(liveSink)

	// Ingest metrics at 100, 101, 102, 105
	for _, ts := range []int64{100, 101, 102, 105} {
		requests := e.IngestMetric("ns", &metricObs{
			name:      "cpu",
			value:     1.0,
			timestamp: ts,
		})
		for _, req := range requests {
			e.advanceWithReason(req.upToSec, req.reason)
		}
	}

	// Ingest log at 103 (no virtual metrics produced)
	logRequests := e.IngestLog("ns", &logObs{
		content:     []byte("error happened"),
		status:      "error",
		timestampMs: 103000, // 103 seconds in millis
	})
	for _, req := range logRequests {
		e.advanceWithReason(req.upToSec, req.reason)
	}

	// Flush remaining
	endRequests := e.scheduler.onReplayEnd(e.schedulerState())
	for _, req := range endRequests {
		e.advanceWithReason(req.upToSec, req.reason)
	}

	liveAdvances := liveSink.eventsOfKind(eventAdvanceCompleted)
	var liveTimestamps []int64
	for _, evt := range liveAdvances {
		liveTimestamps = append(liveTimestamps, evt.advanceCompleted.advancedToSec)
	}

	// --- Now reset and do replay ---
	unsub := e.Subscribe(&collectingSink{}) // dummy to capture unsub
	unsub()

	e.resetFull()

	replaySink := &collectingSink{}
	e.Subscribe(replaySink)

	e.ReplayStoredData()

	replayAdvances := replaySink.eventsOfKind(eventAdvanceCompleted)
	var replayTimestamps []int64
	for _, evt := range replayAdvances {
		replayTimestamps = append(replayTimestamps, evt.advanceCompleted.advancedToSec)
	}

	t.Logf("live advance timestamps:   %v", liveTimestamps)
	t.Logf("replay advance timestamps: %v", replayTimestamps)

	// The bug: replay's DataTimestamps() only has metric timestamps [100,101,102,105],
	// missing the log's timestamp 103. So replay doesn't advance through 103.
	// In live mode, the log at 103 DID trigger onObservation and potentially an advance.
	//
	// Check that DataTimestamps doesn't include 103.
	dataTS := storage.DataTimestamps()
	has103 := false
	for _, ts := range dataTS {
		if ts == 103 {
			has103 = true
			break
		}
	}
	assert.True(t, has103,
		"DataTimestamps() should include timestamp 103 from the log observation, "+
			"but it only returns metric timestamps: %v", dataTS)
}
