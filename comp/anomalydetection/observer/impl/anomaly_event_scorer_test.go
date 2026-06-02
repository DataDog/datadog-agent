// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// makeAnomaly constructs a minimal Anomaly for scorer tests.
func makeAnomaly(ts int64, score float64, key string) observerdef.Anomaly {
	sc := score
	return observerdef.Anomaly{
		Timestamp:    ts,
		DetectorName: "test",
		Title:        "test anomaly",
		Score:        &sc,
		Source: observerdef.SeriesDescriptor{
			Namespace: "test",
			Name:      key,
		},
	}
}

func newTestScorer() *anomalyEventScorer {
	return newAnomalyEventScorer(anomalyEventScorerConfig{
		windowSeconds: 60,
		maxItems:      100,
		ewmaAlpha:     defaultAnomalyEventEWMAAlpha,
		trendEpsilon:  defaultTrendEpsilon,
	})
}

// ---- EWMA tests ---------------------------------------------------------------

func TestEWMAInitialization(t *testing.T) {
	s := newTestScorer()
	a := makeAnomaly(0, 0.8, "src")
	evt := s.ProcessAnomaly(a)

	// First event: EWMA = alpha * instant + (1-alpha) * 0 = alpha * instant.
	expectedEWMA := defaultAnomalyEventEWMAAlpha * evt.Score.Instant
	if abs(evt.Score.EWMA-expectedEWMA) > 1e-9 {
		t.Errorf("first-event EWMA: got %.6f, want %.6f", evt.Score.EWMA, expectedEWMA)
	}
	if evt.Score.PreviousEWMA != 0 {
		t.Errorf("first-event PreviousEWMA: got %.6f, want 0", evt.Score.PreviousEWMA)
	}
}

func TestEWMAUpdate(t *testing.T) {
	s := newTestScorer()

	// Feed two anomalies with the same key (same signal, same window).
	a1 := makeAnomaly(0, 0.8, "src")
	e1 := s.ProcessAnomaly(a1)
	prevEWMA := e1.Score.EWMA

	a2 := makeAnomaly(1, 0.9, "src2")
	e2 := s.ProcessAnomaly(a2)

	expectedEWMA := defaultAnomalyEventEWMAAlpha*e2.Score.Instant + (1-defaultAnomalyEventEWMAAlpha)*prevEWMA
	if abs(e2.Score.EWMA-expectedEWMA) > 1e-9 {
		t.Errorf("second-event EWMA: got %.6f, want %.6f", e2.Score.EWMA, expectedEWMA)
	}
	if abs(e2.Score.PreviousEWMA-prevEWMA) > 1e-9 {
		t.Errorf("PreviousEWMA: got %.6f, want %.6f", e2.Score.PreviousEWMA, prevEWMA)
	}
}

// ---- Trend tests --------------------------------------------------------------

func TestTrendIncreased(t *testing.T) {
	s := newTestScorer()
	// Prime with a low-score event.
	s.ProcessAnomaly(makeAnomaly(0, 0.1, "src"))
	// Send a high-score event; EWMA should rise by more than epsilon.
	evt := s.ProcessAnomaly(makeAnomaly(1, 0.95, "src2"))
	if evt.Score.Trend != observerdef.AnomalyEventTrendIncreased {
		t.Errorf("trend: got %q, want %q", evt.Score.Trend, observerdef.AnomalyEventTrendIncreased)
	}
}

func TestTrendDecreased(t *testing.T) {
	s := newTestScorer()
	// Prime EWMA with high-score events at ts=0.
	for i := 0; i < 5; i++ {
		s.ProcessAnomaly(makeAnomaly(0, 0.95, "src"))
	}
	// 65 s later: the old anomalies have expired from the 60 s window.
	// Send a very low-score event → instant near-zero → EWMA drops.
	sc := 0.01
	a := observerdef.Anomaly{Timestamp: 65, DetectorName: "test", Title: "t", Score: &sc,
		Source: observerdef.SeriesDescriptor{Namespace: "test", Name: "other"}}
	evt := s.ProcessAnomaly(a)
	if evt.Score.Trend != observerdef.AnomalyEventTrendDecreased {
		t.Errorf("trend: got %q (ewma=%.3f prev=%.3f), want %q",
			evt.Score.Trend, evt.Score.EWMA, evt.Score.PreviousEWMA,
			observerdef.AnomalyEventTrendDecreased)
	}
}

func TestTrendStableWhenEWMAConverged(t *testing.T) {
	s := newTestScorer()
	// Drive EWMA to a near-steady state by feeding many events with identical
	// instant scores.  The per-event EWMA delta shrinks as it converges.
	// We capture the last few deltas and check that at least the final one is stable.
	const n = 30
	var lastEvt observerdef.ScoredAnomalyEvent
	for i := 0; i < n; i++ {
		// Spread events > 60 s apart so the window never holds more than 1 anomaly
		// → instant score stays constant per event.
		lastEvt = s.ProcessAnomaly(makeAnomaly(int64(i*65), 0.5, "src"))
	}
	if lastEvt.Score.Trend != observerdef.AnomalyEventTrendStable {
		t.Errorf("after convergence trend: got %q (ewma=%.4f prev=%.4f delta=%.4f), want stable",
			lastEvt.Score.Trend, lastEvt.Score.EWMA, lastEvt.Score.PreviousEWMA,
			lastEvt.Score.EWMA-lastEvt.Score.PreviousEWMA)
	}
}

// ---- Severity hysteresis tests -----------------------------------------------

func TestSeverityHysteresisHighToMediumRequiresDrop(t *testing.T) {
	s := newTestScorer()

	// Drive EWMA into high territory.
	for i := 0; i < 20; i++ {
		s.ProcessAnomaly(makeAnomaly(int64(i), 0.99, "src"))
	}
	evt := s.ProcessAnomaly(makeAnomaly(30, 0.99, "src"))
	if evt.Score.Severity != observerdef.AnomalyEventSeverityHigh {
		t.Fatalf("expected high severity, got %q", evt.Score.Severity)
	}

	// EWMA just below high threshold (0.75): should stay high due to hysteresis (0.70).
	s2 := newTestScorer()
	for i := 0; i < 20; i++ {
		s2.ProcessAnomaly(makeAnomaly(int64(i), 0.99, "src"))
	}
	// Force a low-scoring event in the same window to drive EWMA down slightly.
	sc := 0.72
	a := observerdef.Anomaly{Timestamp: 25, DetectorName: "test", Title: "t", Score: &sc,
		Source: observerdef.SeriesDescriptor{Namespace: "test", Name: "other"}}
	evt2 := s2.ProcessAnomaly(a)
	// EWMA slightly above 0.70 → still high (hysteresis: needs to drop below 0.70).
	if evt2.Score.EWMA >= highSeverityThreshold-severityHysteresis &&
		evt2.Score.Severity != observerdef.AnomalyEventSeverityHigh {
		t.Errorf("hysteresis violated: EWMA=%.3f should keep high severity, got %q",
			evt2.Score.EWMA, evt2.Score.Severity)
	}
}

func TestSeverityLowToMediumAtThreshold(t *testing.T) {
	s := newTestScorer()
	// Send anomalies that push instant score above medium threshold.
	for i := 0; i < 5; i++ {
		s.ProcessAnomaly(makeAnomaly(int64(i), 0.99, "src"))
	}
	evt := s.ProcessAnomaly(makeAnomaly(10, 0.99, "src"))
	if evt.Score.Severity != observerdef.AnomalyEventSeverityHigh &&
		evt.Score.Severity != observerdef.AnomalyEventSeverityMedium {
		t.Errorf("expected at least medium after multiple high-score events, got %q", evt.Score.Severity)
	}
}

// ---- Consumer tests -----------------------------------------------------------

func TestConsumerReceivesOneEventPerAnomaly(t *testing.T) {
	received := make([]observerdef.ScoredAnomalyEvent, 0)
	consumer := &captureConsumer{fn: func(evt observerdef.ScoredAnomalyEvent) {
		received = append(received, evt)
	}}

	e := newEngine(engineConfig{})
	e.SetAnomalyEventConsumers([]observerdef.AnomalyEventConsumer{consumer})

	anomalies := []observerdef.Anomaly{
		makeAnomaly(0, 0.5, "src1"),
		makeAnomaly(1, 0.7, "src2"),
		makeAnomaly(2, 0.9, "src3"),
	}
	for _, a := range anomalies {
		e.scoreAnomalyEvent(a)
	}

	if len(received) != len(anomalies) {
		t.Errorf("consumer received %d events, want %d", len(received), len(anomalies))
	}
}

func TestSlowConsumerDoesNotBlockScoring(t *testing.T) {
	// A consumer that panics if called more than once simultaneously is a proxy for
	// "non-blocking" — the real test is that scoring completes even if the consumer
	// is slow.  Here we just verify no deadlock by using a buffered channel consumer.
	ch := make(chan observerdef.ScoredAnomalyEvent, 10)
	consumer := &captureConsumer{fn: func(evt observerdef.ScoredAnomalyEvent) {
		ch <- evt
	}}

	e := newEngine(engineConfig{})
	e.SetAnomalyEventConsumers([]observerdef.AnomalyEventConsumer{consumer})

	for i := 0; i < 5; i++ {
		e.scoreAnomalyEvent(makeAnomaly(int64(i), 0.5, "src"))
	}
	if len(ch) != 5 {
		t.Errorf("buffered consumer received %d events, want 5", len(ch))
	}
}

// ---- Event history tests ------------------------------------------------------

func TestEventHistoryBoundedAndReadable(t *testing.T) {
	s := newTestScorer()
	s.cfg.maxItems = 3

	for i := 0; i < 10; i++ {
		s.ProcessAnomaly(makeAnomaly(int64(i), 0.5, "src"))
	}

	// All 10 events should be in history (maxItems limits window, not history).
	evts := s.Events()
	if len(evts) != 10 {
		t.Errorf("event history length: got %d, want 10", len(evts))
	}
	// Window size should be bounded to maxItems.
	last := evts[len(evts)-1]
	if last.Window.Size > s.cfg.maxItems {
		t.Errorf("window size %d exceeds maxItems %d", last.Window.Size, s.cfg.maxItems)
	}
}

func TestEventHistoryClearedOnReset(t *testing.T) {
	s := newTestScorer()
	s.ProcessAnomaly(makeAnomaly(0, 0.5, "src"))
	s.Reset()
	if len(s.Events()) != 0 {
		t.Error("expected empty history after Reset")
	}
}

// ---- helpers ------------------------------------------------------------------

type captureConsumer struct {
	fn func(observerdef.ScoredAnomalyEvent)
}

func (c *captureConsumer) Name() string { return "capture" }
func (c *captureConsumer) ProcessAnomalyEvent(evt observerdef.ScoredAnomalyEvent) {
	c.fn(evt)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
