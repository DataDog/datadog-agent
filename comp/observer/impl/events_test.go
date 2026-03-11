// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
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
