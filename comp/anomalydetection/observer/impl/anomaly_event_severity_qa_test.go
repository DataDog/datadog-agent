// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// anomaly_event_severity_qa_test.go — local QA test for the severity pipeline.
//
// Run with:
//
//	go test ./comp/anomalydetection/observer/impl/ -run TestSeverityProgressionQA -v
//
// The StdoutAnomalyEventConsumer prints one "[anomaly-event]" line per scored
// event directly to stdout. When run with -v you will see the full progression
// of symbols: · (LOW) then ● (MEDIUM) then ▲ (HIGH), each with the trend
// arrow ↑ and transition annotations like [low→medium] and [medium→high].
//
// Pipeline exercised:
//
//	IngestMetric (7 distinct series)
//	  → timeSeriesStorage
//	    → seriesDetectorAdapter(CUSUM)   ← real detector, not a stub
//	      → engine.captureRawAnomaly
//	        → engine.scoreAnomalyEvent
//	          → anomalyEventScorer (EWMA + hysteresis)
//	            → StdoutAnomalyEventConsumer (prints [anomaly-event] lines)
//
// Why multiple series are required for HIGH severity
//
// The log-count cap (cap(N) = 0.45 + 0.50·ln(1+N)/ln(11)) prevents a single
// series from pushing the instant score above ~0.60. With 7 distinct signals
// in the 60 s window the cap reaches ~0.88, allowing the EWMA to cross 0.75.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestSeverityProgressionQA(t *testing.T) {
	// Seven distinct series — each produces one CUSUM anomaly on its spike.
	// All 7 anomalies land at timestamps 15–21 (within the 60 s scoring window).
	metrics := []string{
		"qa.anomaly.cpu",
		"qa.anomaly.mem",
		"qa.anomaly.disk",
		"qa.anomaly.net",
		"qa.anomaly.latency",
		"qa.anomaly.errors",
		"qa.anomaly.db",
	}

	// Real CUSUM detector wrapped in the production seriesDetectorAdapter.
	// DefaultCUSUMConfig: min_points=5, baseline_fraction=0.25, threshold_factor=4.0.
	cusumDetector := newSeriesDetectorAdapter(
		NewCUSUMDetector(DefaultCUSUMConfig()),
		[]observerdef.Aggregate{observerdef.AggregateAverage},
	)

	// Capture consumer collects events for assertions; StdoutAnomalyEventConsumer
	// prints them so they are visible in 'go test -v' output.
	collected := make([]observerdef.ScoredAnomalyEvent, 0, len(metrics))
	capture := &captureConsumer{fn: func(evt observerdef.ScoredAnomalyEvent) {
		collected = append(collected, evt)
	}}

	e := newEngine(engineConfig{
		storage:   newTimeSeriesStorage(),
		detectors: []observerdef.Detector{cusumDetector},
	})
	e.SetAnomalyEventConsumers([]observerdef.AnomalyEventConsumer{
		NewStdoutAnomalyEventConsumer("[qa]"),
		capture,
	})

	// ---- Baseline phase -------------------------------------------------------
	// 15 points at value=1.0 for every metric (timestamps 0–14).
	// CUSUM uses the first baseline_fraction (25%) = ~4 points as baseline.
	// 15 points > min_points=5, so CUSUM is ready to fire on any spike.
	const (
		baselineVal = 1.0
		spikeVal    = 5000.0 // 5000× baseline — reliably trips CUSUM threshold
		baselineN   = 15
	)
	for ts := int64(0); ts < baselineN; ts++ {
		for _, name := range metrics {
			e.IngestMetric("qa-source", &metricObs{name: name, value: baselineVal, timestamp: ts})
		}
	}

	// ---- Spike phase ----------------------------------------------------------
	// One spike per metric at timestamps 15, 16, …, 21 (7 s spread).
	// All spikes are added to storage before any advance so the window is
	// consistent when scoring fires. Advancing one step at a time ensures each
	// CUSUM call only sees one spike at a time (GetSeriesRange uses upTo=dataTime).
	for i, name := range metrics {
		ts := int64(baselineN + i)
		e.IngestMetric("qa-source", &metricObs{name: name, value: spikeVal, timestamp: ts})
	}

	// ---- Detection phase ------------------------------------------------------
	// Advance through each spike second so CUSUM fires exactly once per metric.
	// After each advance the anomaly is scored and [anomaly-event] is printed.
	for i := range metrics {
		e.Advance(int64(baselineN + i))
	}
	// Final flush: advance past the last spike so the 7th anomaly clears dedup.
	e.Advance(int64(baselineN + len(metrics)))

	t.Logf("total scored events: %d (expected %d)", len(collected), len(metrics))

	require.Len(t, collected, len(metrics),
		"expected one ScoredAnomalyEvent per metric spike; "+
			"got %d — check that CUSUM fired on each series", len(collected))

	// ---- Severity progression assertions -------------------------------------
	// Expected EWMA trajectory (alpha=0.30, score=0.9 per event, 7 signals):
	//   event 1: EWMA≈0.178 → LOW
	//   event 2: EWMA≈0.329 → LOW
	//   event 3: EWMA≈0.452 → MEDIUM  (low→medium)
	//   event 7: EWMA≈0.755 → HIGH    (medium→high)
	assert.Equal(t, observerdef.AnomalyEventSeverityLow, collected[0].Score.Severity,
		"event 1 (cpu spike): expected LOW — single signal, EWMA≈0.18")
	assert.Equal(t, observerdef.AnomalyEventSeverityLow, collected[1].Score.Severity,
		"event 2 (mem spike): expected LOW — two signals, EWMA≈0.33")
	assert.Equal(t, observerdef.AnomalyEventSeverityMedium, collected[2].Score.Severity,
		"event 3 (disk spike): expected MEDIUM — three signals push EWMA above 0.40")
	assert.Equal(t, observerdef.AnomalyEventSeverityHigh, collected[6].Score.Severity,
		"event 7 (db spike): expected HIGH — seven signals push EWMA above 0.75")

	// Severity-change transitions must be flagged.
	assert.True(t, collected[2].Score.SeverityChanged,
		"event 3 must flag low→medium transition")
	assert.True(t, collected[6].Score.SeverityChanged,
		"event 7 must flag medium→high transition")

	// EWMA must be monotonically increasing (every new spike raises the instant score).
	for i := 1; i < len(collected); i++ {
		assert.Greater(t, collected[i].Score.EWMA, collected[i-1].Score.EWMA,
			"event %d EWMA (%.3f) must be greater than event %d EWMA (%.3f)",
			i+1, collected[i].Score.EWMA, i, collected[i-1].Score.EWMA)
	}

	// All events should show rising trend (EWMA is always increasing).
	for i, evt := range collected {
		assert.Equal(t, observerdef.AnomalyEventTrendIncreased, evt.Score.Trend,
			"event %d: expected trend=increased (EWMA is rising)", i+1)
	}

	t.Logf("severity progression: LOW×%d → MEDIUM×%d → HIGH×%d",
		countSeverity(collected, observerdef.AnomalyEventSeverityLow),
		countSeverity(collected, observerdef.AnomalyEventSeverityMedium),
		countSeverity(collected, observerdef.AnomalyEventSeverityHigh),
	)
}

func countSeverity(events []observerdef.ScoredAnomalyEvent, sev observerdef.AnomalyEventSeverity) int {
	n := 0
	for _, e := range events {
		if e.Score.Severity == sev {
			n++
		}
	}
	return n
}
